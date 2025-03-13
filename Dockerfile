FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

# Add support for cross-compilation
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum* ./

# Download dependencies (if go.sum exists)
RUN if [ -f go.sum ]; then go mod download; else go mod tidy; fi

# Copy source code
COPY *.go ./

# Build the application with proper cross-compilation flags
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o jsonrpc-proxy

# Use a smaller image for the final build
FROM alpine:3.17

RUN apk --no-cache add ca-certificates && \
    update-ca-certificates

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/jsonrpc-proxy .

# Create a directory for configuration
RUN mkdir -p /app/config

# Expose the default port
EXPOSE 8080

# Run the application
ENTRYPOINT ["/app/jsonrpc-proxy"]
CMD ["-config=/app/config/config.yaml", "-port=8080"]