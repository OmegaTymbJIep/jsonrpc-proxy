# JSON-RPC HTTP Proxy

A simple, configurable HTTP proxy for JSON-RPC requests written in Go. The proxy routes requests to different backend services based on the JSON-RPC method name.

## Features

- Route JSON-RPC requests to different backends based on method name
- YAML-based configuration
- Fallback to default URL for undefined methods
- Transparent proxy that preserves headers and status codes

## Installation

### Prerequisites

- Go 1.16 or higher (for building from source)
- Docker (for containerized deployment)

### Building from source

```bash
# Clone the repository
git clone https://github.com/yourusername/jsonrpc-proxy.git
cd jsonrpc-proxy

# Install dependencies
go mod download

# Build the binary
go build -o jsonrpc-proxy

# Run the proxy
./jsonrpc-proxy -config=config.yaml -port=8080
```

### Using Docker

```bash
# Build the Docker image
docker build -t jsonrpc-proxy .

# Run the container
docker run -p 8080:8080 -v $(pwd)/config.yaml:/app/config/config.yaml jsonrpc-proxy
```

### Using Docker Compose

```bash
# Start the service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the service
docker-compose down
```

## Configuration

Create a `config.yaml` file with the following structure:

```yaml
# Default destination URL for any methods not explicitly defined
default_url: "https://mainnet.infura.io/v3/your-project-id"

# Method-specific routing
routes:
  - method: "eth_chainId"
    url: "https://polygon-rpc.com"
  
  - method: "eth_blockNumber"
    url: "https://rpc.ankr.com/eth"
  
  - method: "net_version"
    url: "https://cloudflare-eth.com"
```

## Usage

### Command-line options

- `-config`: Path to the YAML configuration file (default: `config.yaml`)
- `-port`: The port to run the proxy server on (default: 8080)

### Docker Environment Variables

When using Docker, you can override the command-line options with environment variables:

```bash
docker run -p 8080:8080 \
  -e CONFIG_PATH=/app/config/custom-config.yaml \
  -e PORT=9000 \
  -v $(pwd)/custom-config.yaml:/app/config/custom-config.yaml \
  jsonrpc-proxy
```

For Docker Compose, add them to your docker-compose.yml file:

```yaml
services:
  jsonrpc-proxy:
    # ...
    environment:
      - CONFIG_PATH=/app/config/custom-config.yaml
      - PORT=9000
```

### Running the proxy

```bash
./jsonrpc-proxy -config=my-config.yaml -port=9000
```

### Example requests

Use curl to test the proxy:

```bash
# This will be routed to the Polygon RPC endpoint
curl -X POST -H "Content-Type: application/json" \
     --data '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}' \
     http://localhost:8080

# This will be routed to the Ankr RPC endpoint
curl -X POST -H "Content-Type: application/json" \
     --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
     http://localhost:8080

# This will be routed to the default URL
curl -X POST -H "Content-Type: application/json" \
     --data '{"jsonrpc":"2.0","method":"unknown_method","params":[],"id":1}' \
     http://localhost:8080
```

## Advanced Configuration Examples

### Load balancing between multiple endpoints

```yaml
default_url: "https://mainnet.infura.io/v3/your-project-id"

routes:
  # Send all eth_call methods to a read-optimized node
  - method: "eth_call"
    url: "https://read-optimized-node.example.com"
  
  # Send all transaction-related methods to a write-optimized node
  - method: "eth_sendRawTransaction"
    url: "https://write-optimized-node.example.com"
  
  - method: "eth_sendTransaction"
    url: "https://write-optimized-node.example.com"
```

### Multi-chain configuration

```yaml
default_url: "https://ethereum.example.com"

routes:
  # Ethereum chain ID
  - method: "eth_chainId"
    url: "https://ethereum.example.com"
  
  # Polygon chain ID
  - method: "polygon_chainId"
    url: "https://polygon.example.com"
  
  # Arbitrum chain ID
  - method: "arbitrum_chainId"
    url: "https://arbitrum.example.com"
```

## Error handling

The proxy will return appropriate HTTP status codes when errors occur:

- `405 Method Not Allowed`: When a non-POST request is received
- `400 Bad Request`: When the request body cannot be read or is not valid JSON-RPC
- `500 Internal Server Error`: When the proxy fails to forward the request

## Health Check

The proxy provides a `/health` endpoint that returns a 200 OK response with a JSON payload:

```json
{"status":"ok"}
```

This endpoint is used by the Docker container's health check to verify that the service is operational. You can use it in your monitoring setup to ensure the proxy is running correctly:

```bash
curl http://localhost:8080/health
```

## Kubernetes Deployment

You can deploy the JSON-RPC proxy to Kubernetes using the following example manifests:

### ConfigMap for configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jsonrpc-proxy-config
data:
  config.yaml: |
    default_url: "https://mainnet.infura.io/v3/your-project-id"
    routes:
      - method: "eth_chainId"
        url: "https://polygon-rpc.com"
      - method: "eth_blockNumber"
        url: "https://rpc.ankr.com/eth"
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jsonrpc-proxy
  labels:
    app: jsonrpc-proxy
spec:
  replicas: 2
  selector:
    matchLabels:
      app: jsonrpc-proxy
  template:
    metadata:
      labels:
        app: jsonrpc-proxy
    spec:
      containers:
      - name: jsonrpc-proxy
        image: your-registry/jsonrpc-proxy:latest
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config-volume
          mountPath: /app/config
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: config-volume
        configMap:
          name: jsonrpc-proxy-config
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: jsonrpc-proxy
spec:
  selector:
    app: jsonrpc-proxy
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
```

## License

MIT License