package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// mockHTTPServer creates a test HTTP server that returns predefined responses
func mockHTTPServer(t *testing.T, expectedMethod, response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read and parse the JSON-RPC request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}
		defer r.Body.Close()

		var req JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("Failed to parse JSON-RPC request: %v", err)
		}

		// Verify the method is what we expect
		if req.Method != expectedMethod {
			t.Errorf("Expected method %s, got %s", expectedMethod, req.Method)
		}

		// Return the predefined response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
}

// setupTestConfig creates a temporary configuration file for testing
func setupTestConfig(t *testing.T, defaultURL string, routes []Route) string {
	// Create a test configuration
	testConfig := Config{
		DefaultURL: defaultURL,
		Routes:     routes,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	// Write to a temporary file
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := tmpfile.Write(data); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	return tmpfile.Name()
}

// cleanupTestConfig removes the temporary configuration file
func cleanupTestConfig(t *testing.T, configPath string) {
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("Failed to remove temp file: %v", err)
	}
}

// TestLoadConfig tests the configuration loading functionality
func TestLoadConfig(t *testing.T) {
	// Setup
	configPath := setupTestConfig(t, "http://default-url.com", []Route{
		{Method: "test_method", URL: "http://test-url.com"},
	})
	defer cleanupTestConfig(t, configPath)

	// Test
	err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify
	if config.DefaultURL != "http://default-url.com" {
		t.Errorf("Expected default URL to be %s, got %s", "http://default-url.com", config.DefaultURL)
	}

	if len(config.Routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(config.Routes))
	}

	if config.Routes[0].Method != "test_method" {
		t.Errorf("Expected route method to be %s, got %s", "test_method", config.Routes[0].Method)
	}

	if config.Routes[0].URL != "http://test-url.com" {
		t.Errorf("Expected route URL to be %s, got %s", "http://test-url.com", config.Routes[0].URL)
	}
}

// TestBuildMethodURLMap tests the method to URL mapping functionality
func TestBuildMethodURLMap(t *testing.T) {
	// Setup
	config = Config{
		DefaultURL: "http://default-url.com",
		Routes: []Route{
			{Method: "method1", URL: "http://url1.com"},
			{Method: "method2", URL: "http://url2.com"},
		},
	}

	// Test
	buildMethodURLMap()

	// Verify
	if len(methodToURL) != 2 {
		t.Errorf("Expected map to have 2 entries, got %d", len(methodToURL))
	}

	if url, ok := methodToURL["method1"]; !ok || url != "http://url1.com" {
		t.Errorf("Expected method1 to map to http://url1.com, got %s", url)
	}

	if url, ok := methodToURL["method2"]; !ok || url != "http://url2.com" {
		t.Errorf("Expected method2 to map to http://url2.com, got %s", url)
	}
}

// TestHealthEndpoint tests the health check endpoint
func TestHealthEndpoint(t *testing.T) {
	// Setup
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Test
	handleHealth(w, req)

	// Verify
	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	expectedBody := `{"status":"ok"}`
	if string(body) != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, string(body))
	}
}

// TestHandleProxy tests the main proxy functionality
func TestHandleProxy(t *testing.T) {
	// Setup mock servers
	server1 := mockHTTPServer(t, "method1", `{"jsonrpc":"2.0","result":"response1","id":1}`)
	defer server1.Close()

	server2 := mockHTTPServer(t, "method2", `{"jsonrpc":"2.0","result":"response2","id":2}`)
	defer server2.Close()

	defaultServer := mockHTTPServer(t, "unknown_method", `{"jsonrpc":"2.0","result":"default","id":3}`)
	defer defaultServer.Close()

	// Setup configuration
	config = Config{
		DefaultURL: defaultServer.URL,
		Routes: []Route{
			{Method: "method1", URL: server1.URL},
			{Method: "method2", URL: server2.URL},
		},
	}
	buildMethodURLMap()

	// Test cases
	testCases := []struct {
		name           string
		method         string
		expectedResult string
	}{
		{"Route to server1", "method1", "response1"},
		{"Route to server2", "method2", "response2"},
		{"Route to default", "unknown_method", "default"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test request
			reqBody := JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  tc.method,
				Params:  []interface{}{},
				ID:      1,
			}
			reqBytes, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/", bytes.NewReader(reqBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Test
			handleProxy(w, req)

			// Verify
			resp := w.Result()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
			}

			var result struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if result.Result != tc.expectedResult {
				t.Errorf("Expected result %s, got %s", tc.expectedResult, result.Result)
			}
		})
	}
}

// TestNonPostRequest tests handling of non-POST requests
func TestNonPostRequest(t *testing.T) {
	// Setup
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Test
	handleProxy(w, req)

	// Verify
	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
}

// TestInvalidJSONRequest tests handling of invalid JSON requests
func TestInvalidJSONRequest(t *testing.T) {
	// Setup
	req := httptest.NewRequest("POST", "/", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Test
	handleProxy(w, req)

	// Verify
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
}

// TestForwardRequest tests the request forwarding functionality
func TestForwardRequest(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type header to be application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept header to be application/json, got %s", r.Header.Get("Accept"))
		}

		// Return a response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","result":"test_response","id":1}`))
	}))
	defer server.Close()

	// Test
	resp, err := forwardRequest(server.URL, []byte(`{"jsonrpc":"2.0","method":"test_method","params":[],"id":1}`))
	if err != nil {
		t.Fatalf("Failed to forward request: %v", err)
	}
	defer resp.Body.Close()

	// Verify
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result.Result != "test_response" {
		t.Errorf("Expected result test_response, got %s", result.Result)
	}
}
