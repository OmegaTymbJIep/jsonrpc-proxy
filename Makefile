.PHONY: build test docker run clean all cover help

# Variables
BINARY_NAME=jsonrpc-proxy
DOCKER_IMAGE=jsonrpc-proxy
GOFILES=$(wildcard *.go)

# Default target
all: clean build test

# Help target
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  test        - Run tests"
	@echo "  cover       - Run tests with coverage"
	@echo "  docker      - Build Docker image"
	@echo "  run         - Run the proxy locally"
	@echo "  clean       - Remove build artifacts"
	@echo "  all         - Clean, build, and test"
	@echo "  help        - Show this help message"

# Build the binary
build: $(GOFILES)
	go build -o $(BINARY_NAME) .

# Run tests
test:
	go test -v ./...

# Run tests with coverage
cover:
	go test -cover ./...
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"

# Build Docker image
docker:
	docker build -t $(DOCKER_IMAGE) .

# Run the proxy locally
run: build
	./$(BINARY_NAME) -config=config.yaml

# Clean up
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out
	rm -f coverage.html