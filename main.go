// Package main implements a JSON-RPC HTTP proxy that routes requests based on a YAML configuration.
//
// This proxy server reads a configuration file specifying routing rules for JSON-RPC methods.
// Each incoming request is inspected to extract the method name, which is then used to determine
// the appropriate destination URL. If no specific route is found for a method, the request is
// forwarded to a default URL.
//
// # Configuration
//
// The proxy uses a YAML configuration file with the following structure:
//
//	# Default destination for methods without specific routes
//	default_url: "https://mainnet.infura.io/v3/your-project-id"
//
//	# Method-specific routing
//	routes:
//	  - method: "eth_chainId"
//	    url: "https://polygon-rpc.com"
//
//	  - method: "eth_blockNumber"
//	    url: "https://rpc.ankr.com/eth"
//
// # Usage
//
// Run the proxy with the following command:
//
//	jsonrpc-proxy -config=config.yaml -port=8080
//
// Example request:
//
//	curl -X POST -H "Content-Type: application/json" \
//	     --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
//	     http://localhost:8080
//
// The proxy will route this request to https://polygon-rpc.com based on the example configuration.
//
// # Options
//
//   -config: Path to the YAML configuration file (default: "config.yaml")
//   -port:   Port to run the proxy server on (default: 8080)
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Configuration structs

// Route defines a single method-to-URL mapping for JSON-RPC method routing.
// Each Route specifies which JSON-RPC method should be forwarded to a particular URL.
type Route struct {
	Method string `yaml:"method"` // The JSON-RPC method name (e.g., "eth_chainId")
	URL    string `yaml:"url"`    // The destination URL for this method
}

// Config holds the complete proxy configuration loaded from the YAML file.
// It contains the default fallback URL and a list of method-specific routes.
type Config struct {
	DefaultURL string  `yaml:"default_url"` // URL for methods without specific routes
	Routes     []Route `yaml:"routes"`      // List of method-specific routes
}

// JSONRPCRequest represents the structure of a JSON-RPC 2.0 request.
// This struct is used to parse incoming requests to extract the method name.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"` // JSON-RPC version (should be "2.0")
	Method  string      `json:"method"`  // The method to invoke
	Params  interface{} `json:"params"`  // Method parameters
	ID      interface{} `json:"id"`      // Request identifier
}

// Global variables
var config Config                 // Holds the loaded configuration
var methodToURL map[string]string // Maps method names to destination URLs

// main is the entry point of the application.
// It loads the configuration, sets up the HTTP server, and starts listening for requests.
// It also supports overriding configuration via environment variables.
func main() {
	// Parse command line flags
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	port := flag.Int("port", 8080, "Port to run the proxy server on")
	flag.Parse()

	// Allow overriding via environment variables (for Docker/container usage)
	if envConfig := os.Getenv("CONFIG_PATH"); envConfig != "" {
		*configFile = envConfig
	}

	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		} else {
			log.Printf("Warning: Invalid PORT environment variable: %s", envPort)
		}
	}

	// Load configuration
	if err := loadConfig(*configFile); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create method to URL mapping for faster lookups
	buildMethodURLMap()

	// Set up HTTP server
	http.HandleFunc("/", handleProxy)
	http.HandleFunc("/health", handleHealth)
	serverAddr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting ANUS proxy server on %s", serverAddr)
	log.Printf("Default URL: %s", config.DefaultURL)
	log.Printf("Loaded %d method-specific routes", len(config.Routes))

	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// loadConfig reads and parses the YAML configuration file.
// It validates that the required fields are present and properly formatted.
//
// Parameters:
//   - filename: The path to the YAML configuration file
//
// Returns:
//   - error: An error if the configuration cannot be loaded or is invalid
func loadConfig(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return fmt.Errorf("error unmarshaling YAML: %w", err)
	}

	// Validate configuration
	if config.DefaultURL == "" {
		return fmt.Errorf("default_url is required in configuration")
	}

	return nil
}

// buildMethodURLMap creates a lookup map from method names to their destination URLs.
// This improves performance by allowing O(1) lookups instead of iterating through routes.
func buildMethodURLMap() {
	methodToURL = make(map[string]string)
	for _, route := range config.Routes {
		methodToURL[route.Method] = route.URL
	}
}

// handleProxy processes incoming HTTP requests, extracts the JSON-RPC method,
// determines the appropriate destination URL, and forwards the request.
// It then relays the response back to the original client.
// Supports both single requests and batch requests (arrays of requests).
//
// Parameters:
//   - w: The HTTP response writer
//   - r: The incoming HTTP request
func handleProxy(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Determine if this is a batch request (array) or single request
	isBatchRequest := false
	var rawMessage json.RawMessage
	if err := json.Unmarshal(body, &rawMessage); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Check if the body starts with '[' to identify a batch request
	trimmed := bytes.TrimSpace(rawMessage)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		isBatchRequest = true
	}

	if isBatchRequest {
		// Handle batch request
		handleBatchRequest(w, body)
	} else {
		// Handle single request
		handleSingleRequest(w, body)
	}
}

// handleSingleRequest processes a single JSON-RPC request.
// It extracts the method, determines the target URL, and forwards the request.
//
// Parameters:
//   - w: The HTTP response writer
//   - body: The raw request body bytes
func handleSingleRequest(w http.ResponseWriter, body []byte) {
	// Parse the JSON-RPC request
	var rpcRequest JSONRPCRequest
	if err := json.Unmarshal(body, &rpcRequest); err != nil {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	// Determine target URL based on the method
	targetURL, exists := methodToURL[rpcRequest.Method]
	if !exists {
		targetURL = config.DefaultURL
	}

	log.Printf("Proxying method '%s' to %s", rpcRequest.Method, targetURL)

	// Forward the request to the target URL
	resp, err := forwardRequest(targetURL, body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	// Set response status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error copying response: %v", err)
	}
}

// handleBatchRequest processes a batch of JSON-RPC requests.
// It parses each request in the batch, routes them to appropriate targets,
// and combines the responses.
//
// Parameters:
//   - w: The HTTP response writer
//   - body: The raw request body bytes containing an array of requests
func handleBatchRequest(w http.ResponseWriter, body []byte) {
	// Parse the batch of requests
	var batchRequests []JSONRPCRequest
	if err := json.Unmarshal(body, &batchRequests); err != nil {
		http.Error(w, "Invalid JSON-RPC batch request", http.StatusBadRequest)
		return
	}

	// Group requests by target URL for efficiency
	requestsByURL := make(map[string][]json.RawMessage)
	methodByID := make(map[interface{}]string) // To log methods by ID

	// First pass: unmarshall to get method and ID for grouping
	for _, req := range batchRequests {
		// Determine target URL based on the method
		targetURL, exists := methodToURL[req.Method]
		if !exists {
			targetURL = config.DefaultURL
		}

		// Convert the request back to raw JSON
		rawRequest, err := json.Marshal(req)
		if err != nil {
			log.Printf("Error marshaling request: %v", err)
			continue
		}

		requestsByURL[targetURL] = append(requestsByURL[targetURL], rawRequest)

		// Store method by ID for logging
		methodByID[req.ID] = req.Method

		log.Printf("Batch request: method '%s' (ID: %v) to %s", req.Method, req.ID, targetURL)
	}

	// Process each group of requests to their target URL
	allResponses := make([]json.RawMessage, 0)

	for targetURL, requests := range requestsByURL {
		// Create a JSON array for this batch of requests
		batchJSON, err := json.Marshal(requests)
		if err != nil {
			log.Printf("Error creating batch request: %v", err)
			continue
		}

		// Unwrap the batch to get array of raw requests
		var rawBatch []json.RawMessage
		if err := json.Unmarshal(batchJSON, &rawBatch); err != nil {
			log.Printf("Error unwrapping batch: %v", err)
			continue
		}

		// Convert each json.RawMessage to []byte for joining
		byteBatch := make([][]byte, len(rawBatch))
		for i, raw := range rawBatch {
			byteBatch[i] = raw
		}

		// Create a proper JSON array for the batch
		batchBody := []byte("[" + string(bytes.Join(byteBatch, []byte(","))) + "]")

		// Forward this batch to the target URL
		resp, err := forwardRequest(targetURL, batchBody)
		if err != nil {
			log.Printf("Error forwarding batch to %s: %v", targetURL, err)
			continue
		}

		// Read the response body
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading response: %v", err)
			continue
		}

		// Parse the response to get the array of results
		var responses []json.RawMessage
		if err := json.Unmarshal(respBody, &responses); err != nil {
			log.Printf("Error parsing batch response: %v", err)
			continue
		}

		// Add these responses to the combined result
		allResponses = append(allResponses, responses...)
	}

	// Send the combined batch response
	w.Header().Set("Content-Type", "application/json")
	if len(allResponses) == 0 {
		// If no responses (all failed), return an empty array
		w.Write([]byte("[]"))
		return
	}

	// Marshal the final combined response
	responseBody, err := json.Marshal(allResponses)
	if err != nil {
		http.Error(w, "Error creating response", http.StatusInternalServerError)
		return
	}

	w.Write(responseBody)
}

// handleHealth responds to health check requests with a 200 OK status.
// This endpoint is used by Docker for container health checks.
//
// Parameters:
//   - w: The HTTP response writer
//   - r: The incoming HTTP request
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// forwardRequest sends the JSON-RPC request to the target URL and returns the response.
// It sets appropriate headers for JSON-RPC communication.
//
// Parameters:
//   - targetURL: The destination URL to forward the request to
//   - body: The raw request body bytes
//
// Returns:
//   - *http.Response: The response from the target server
//   - error: An error if the request fails
func forwardRequest(targetURL string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set common headers for JSON-RPC
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Send the request
	client := &http.Client{}
	return client.Do(req)
}
