# MAIA - Memory AI Architecture
# Build and development commands

.PHONY: all build dev test lint fmt clean help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
BINARY_NAME=maia
BINARY_CLI=maiactl
BINARY_MCP=maia-mcp

# Build directories
BUILD_DIR=./build
CMD_DIR=./cmd

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Linker flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

## help: Show this help message
help:
	@echo "MAIA - Memory AI Architecture"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## all: Build all binaries
all: build

## build: Build all binaries for current platform
build: build-server build-cli build-mcp

## build-server: Build the main MAIA server
build-server:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)/maia

## build-cli: Build the CLI tool
build-cli:
	@echo "Building $(BINARY_CLI)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_CLI) $(CMD_DIR)/maiactl

## build-mcp: Build the MCP server
build-mcp:
	@echo "Building $(BINARY_MCP)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_MCP) $(CMD_DIR)/mcp-server

## dev: Run the server in development mode
dev:
	@echo "Starting MAIA in development mode..."
	$(GOCMD) run $(CMD_DIR)/maia

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

## test-short: Run tests without race detection (faster)
test-short:
	@echo "Running tests (short mode)..."
	$(GOTEST) -v -short ./...

## test-cover: Run tests with coverage report
test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## lint: Run linters
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

## tidy: Tidy go.mod
tidy:
	@echo "Tidying modules..."
	$(GOMOD) tidy

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

## generate: Run go generate
generate:
	@echo "Running go generate..."
	$(GOCMD) generate ./...

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@rm -rf ./data/test-*

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t maia:$(VERSION) -t maia:latest .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 9090:9090 -v maia-data:/data maia:latest

## proto: Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@if [ -f api/proto/maia.proto ]; then \
		protoc --go_out=. --go-grpc_out=. api/proto/maia.proto; \
	else \
		echo "No proto files found"; \
	fi

## install-tools: Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

## check: Run all checks (fmt, lint, test)
check: fmt lint test
	@echo "All checks passed!"

# Default target
.DEFAULT_GOAL := help
