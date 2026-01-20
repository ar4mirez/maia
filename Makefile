# MAIA - Memory AI Architecture
# Build and development commands

.PHONY: all build build-server build-cli build-mcp build-examples \
	dev dev-mcp test test-short test-cover test-pkg bench \
	lint fmt vet tidy deps generate clean clean-all \
	docker-build docker-run proto install-tools check \
	install uninstall run-example-basic run-example-proxy run-example-multi-agent \
	build-linux build-darwin build-windows build-all-platforms

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOVET := $(GOCMD) vet
GOFMT := gofmt

# Binary names
BINARY_NAME := maia
BINARY_CLI := maiactl
BINARY_MCP := maia-mcp

# Directories
BUILD_DIR := ./build
BIN_DIR := ./bin
CMD_DIR := ./cmd
EXAMPLES_DIR := ./examples

# Install location
PREFIX ?= /usr/local
INSTALL_DIR := $(PREFIX)/bin

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# Build info package path
PKG_VERSION := github.com/ar4mirez/maia/internal/version

# Linker flags for version injection
LDFLAGS := -ldflags "\
	-X $(PKG_VERSION).Version=$(VERSION) \
	-X $(PKG_VERSION).Commit=$(COMMIT) \
	-X $(PKG_VERSION).BuildTime=$(BUILD_TIME) \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildTime=$(BUILD_TIME)"

# Release build flags (smaller binary, no debug info)
LDFLAGS_RELEASE := -ldflags "\
	-s -w \
	-X $(PKG_VERSION).Version=$(VERSION) \
	-X $(PKG_VERSION).Commit=$(COMMIT) \
	-X $(PKG_VERSION).BuildTime=$(BUILD_TIME) \
	-X main.Version=$(VERSION) \
	-X main.Commit=$(COMMIT) \
	-X main.BuildTime=$(BUILD_TIME)"

# Platform detection
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Colors for terminal output
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

#=============================================================================
# Help
#=============================================================================

## help: Show this help message
help:
	@echo "$(GREEN)MAIA - Memory AI Architecture$(NC)"
	@echo ""
	@echo "$(BLUE)Usage:$(NC) make [target]"
	@echo ""
	@echo "$(YELLOW)Build Targets:$(NC)"
	@grep -E '^## (build|all)' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Development Targets:$(NC)"
	@grep -E '^## (dev|run|test|bench|lint|fmt|vet|tidy|deps|generate|check)' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Installation Targets:$(NC)"
	@grep -E '^## (install|uninstall)' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Docker Targets:$(NC)"
	@grep -E '^## docker' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Cleanup Targets:$(NC)"
	@grep -E '^## clean' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Cross-Platform Targets:$(NC)"
	@grep -E '^## build-(linux|darwin|windows|all-platforms)' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'

#=============================================================================
# Build Targets
#=============================================================================

## all: Build all binaries (server, CLI, MCP)
all: build

## build: Build all main binaries for current platform
build: build-server build-cli build-mcp
	@echo "$(GREEN)✓ All binaries built successfully in $(BUILD_DIR)$(NC)"

## build-server: Build the main MAIA server
build-server:
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)/maia

## build-cli: Build the CLI tool (maiactl)
build-cli:
	@echo "$(BLUE)Building $(BINARY_CLI)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_CLI) $(CMD_DIR)/maiactl

## build-mcp: Build the MCP server
build-mcp:
	@echo "$(BLUE)Building $(BINARY_MCP)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_MCP) $(CMD_DIR)/mcp-server

## build-examples: Build all example binaries
build-examples: build-example-basic build-example-proxy build-example-multi-agent
	@echo "$(GREEN)✓ All examples built successfully in $(BUILD_DIR)/examples$(NC)"

## build-example-basic: Build the basic-usage example
build-example-basic:
	@echo "$(BLUE)Building basic-usage example...$(NC)"
	@mkdir -p $(BUILD_DIR)/examples
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/examples/basic-usage $(EXAMPLES_DIR)/basic-usage

## build-example-proxy: Build the proxy-usage example
build-example-proxy:
	@echo "$(BLUE)Building proxy-usage example...$(NC)"
	@mkdir -p $(BUILD_DIR)/examples
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/examples/proxy-usage $(EXAMPLES_DIR)/proxy-usage

## build-example-multi-agent: Build the multi-agent example
build-example-multi-agent:
	@echo "$(BLUE)Building multi-agent example...$(NC)"
	@mkdir -p $(BUILD_DIR)/examples
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/examples/multi-agent $(EXAMPLES_DIR)/multi-agent

## build-release: Build release binaries (optimized, smaller)
build-release:
	@echo "$(BLUE)Building release binaries...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)/maia
	$(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/$(BINARY_CLI) $(CMD_DIR)/maiactl
	$(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	@echo "$(GREEN)✓ Release binaries built successfully$(NC)"

#=============================================================================
# Cross-Platform Build Targets
#=============================================================================

## build-linux: Build for Linux (amd64 and arm64)
build-linux:
	@echo "$(BLUE)Building for Linux...$(NC)"
	@mkdir -p $(BUILD_DIR)/linux-amd64 $(BUILD_DIR)/linux-arm64
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-amd64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-amd64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-amd64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	@echo "$(GREEN)✓ Linux binaries built$(NC)"

## build-darwin: Build for macOS (amd64 and arm64)
build-darwin:
	@echo "$(BLUE)Building for macOS...$(NC)"
	@mkdir -p $(BUILD_DIR)/darwin-amd64 $(BUILD_DIR)/darwin-arm64
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	@echo "$(GREEN)✓ macOS binaries built$(NC)"

## build-windows: Build for Windows (amd64)
build-windows:
	@echo "$(BLUE)Building for Windows...$(NC)"
	@mkdir -p $(BUILD_DIR)/windows-amd64
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_NAME).exe $(CMD_DIR)/maia
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_CLI).exe $(CMD_DIR)/maiactl
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_MCP).exe $(CMD_DIR)/mcp-server
	@echo "$(GREEN)✓ Windows binaries built$(NC)"

## build-all-platforms: Build for all supported platforms
build-all-platforms: build-linux build-darwin build-windows
	@echo "$(GREEN)✓ All platform binaries built successfully$(NC)"
	@echo ""
	@echo "Binaries available in:"
	@ls -la $(BUILD_DIR)/*/

#=============================================================================
# Development Targets
#=============================================================================

## dev: Run the MAIA server in development mode
dev:
	@echo "$(BLUE)Starting MAIA in development mode...$(NC)"
	$(GOCMD) run $(CMD_DIR)/maia

## dev-mcp: Run the MCP server in development mode
dev-mcp:
	@echo "$(BLUE)Starting MCP server in development mode...$(NC)"
	$(GOCMD) run $(CMD_DIR)/mcp-server

## run-example-basic: Run the basic-usage example
run-example-basic:
	@echo "$(BLUE)Running basic-usage example...$(NC)"
	$(GOCMD) run $(EXAMPLES_DIR)/basic-usage

## run-example-proxy: Run the proxy-usage example
run-example-proxy:
	@echo "$(BLUE)Running proxy-usage example...$(NC)"
	$(GOCMD) run $(EXAMPLES_DIR)/proxy-usage

## run-example-multi-agent: Run the multi-agent example
run-example-multi-agent:
	@echo "$(BLUE)Running multi-agent example...$(NC)"
	$(GOCMD) run $(EXAMPLES_DIR)/multi-agent

#=============================================================================
# Testing Targets
#=============================================================================

## test: Run all tests with race detection and coverage
test:
	@echo "$(BLUE)Running tests...$(NC)"
	$(GOTEST) -v -race -cover ./...

## test-short: Run tests without race detection (faster)
test-short:
	@echo "$(BLUE)Running tests (short mode)...$(NC)"
	$(GOTEST) -v -short ./...

## test-cover: Run tests with detailed coverage report
test-cover:
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ Coverage report: coverage.html$(NC)"
	@$(GOCMD) tool cover -func=coverage.out | tail -1

## test-pkg: Run tests for a specific package (usage: make test-pkg PKG=./internal/inference)
test-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "$(YELLOW)Usage: make test-pkg PKG=./internal/inference$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Running tests for $(PKG)...$(NC)"
	$(GOTEST) -v -race -cover $(PKG)

## bench: Run benchmarks
bench:
	@echo "$(BLUE)Running benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem -run=^$$ ./...

## bench-pkg: Run benchmarks for a specific package (usage: make bench-pkg PKG=./internal/inference)
bench-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "$(YELLOW)Usage: make bench-pkg PKG=./internal/inference$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Running benchmarks for $(PKG)...$(NC)"
	$(GOTEST) -bench=. -benchmem -run=^$$ $(PKG)

#=============================================================================
# Code Quality Targets
#=============================================================================

## lint: Run golangci-lint
lint:
	@echo "$(BLUE)Running linters...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not installed. Install with: make install-tools$(NC)"; \
		exit 1; \
	fi

## fmt: Format code with gofmt and goimports
fmt:
	@echo "$(BLUE)Formatting code...$(NC)"
	$(GOFMT) -s -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi
	@echo "$(GREEN)✓ Code formatted$(NC)"

## vet: Run go vet
vet:
	@echo "$(BLUE)Running go vet...$(NC)"
	$(GOVET) ./...

## tidy: Tidy go.mod and go.sum
tidy:
	@echo "$(BLUE)Tidying modules...$(NC)"
	$(GOMOD) tidy
	@echo "$(GREEN)✓ Modules tidied$(NC)"

## deps: Download dependencies
deps:
	@echo "$(BLUE)Downloading dependencies...$(NC)"
	$(GOMOD) download
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

## generate: Run go generate
generate:
	@echo "$(BLUE)Running go generate...$(NC)"
	$(GOCMD) generate ./...
	@echo "$(GREEN)✓ Code generation complete$(NC)"

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "$(GREEN)✓ All checks passed!$(NC)"

## pre-commit: Run pre-commit checks (fast)
pre-commit: fmt vet lint test-short
	@echo "$(GREEN)✓ Pre-commit checks passed!$(NC)"

#=============================================================================
# Installation Targets
#=============================================================================

## install: Install binaries to system (default: /usr/local/bin)
install: build
	@echo "$(BLUE)Installing binaries to $(INSTALL_DIR)...$(NC)"
	@mkdir -p $(INSTALL_DIR)
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@install -m 755 $(BUILD_DIR)/$(BINARY_CLI) $(INSTALL_DIR)/$(BINARY_CLI)
	@install -m 755 $(BUILD_DIR)/$(BINARY_MCP) $(INSTALL_DIR)/$(BINARY_MCP)
	@echo "$(GREEN)✓ Installed:$(NC)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_CLI)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_MCP)"

## uninstall: Remove installed binaries from system
uninstall:
	@echo "$(BLUE)Removing binaries from $(INSTALL_DIR)...$(NC)"
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@rm -f $(INSTALL_DIR)/$(BINARY_CLI)
	@rm -f $(INSTALL_DIR)/$(BINARY_MCP)
	@echo "$(GREEN)✓ Uninstalled$(NC)"

#=============================================================================
# Docker Targets
#=============================================================================

## docker-build: Build Docker image
docker-build:
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build -t maia:$(VERSION) -t maia:latest .
	@echo "$(GREEN)✓ Docker image built: maia:$(VERSION)$(NC)"

## docker-run: Run Docker container
docker-run:
	@echo "$(BLUE)Running Docker container...$(NC)"
	docker run -p 8080:8080 -p 9090:9090 -v maia-data:/data maia:latest

## docker-push: Push Docker image to registry (requires DOCKER_REGISTRY env var)
docker-push:
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "$(YELLOW)DOCKER_REGISTRY not set. Usage: DOCKER_REGISTRY=ghcr.io/username make docker-push$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Pushing Docker image to $(DOCKER_REGISTRY)...$(NC)"
	docker tag maia:$(VERSION) $(DOCKER_REGISTRY)/maia:$(VERSION)
	docker tag maia:latest $(DOCKER_REGISTRY)/maia:latest
	docker push $(DOCKER_REGISTRY)/maia:$(VERSION)
	docker push $(DOCKER_REGISTRY)/maia:latest
	@echo "$(GREEN)✓ Docker image pushed$(NC)"

#=============================================================================
# Cleanup Targets
#=============================================================================

## clean: Clean build artifacts
clean:
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	@rm -rf $(BUILD_DIR)
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out coverage.html
	@rm -rf ./data/test-*
	@echo "$(GREEN)✓ Clean complete$(NC)"

## clean-all: Clean everything including root binaries and cache
clean-all: clean
	@echo "$(BLUE)Cleaning root binaries and cache...$(NC)"
	@rm -f ./maia ./maiactl ./maia-mcp ./mcp-server
	@rm -f ./basic-usage ./proxy-usage ./multi-agent
	@$(GOCMD) clean -cache -testcache
	@echo "$(GREEN)✓ Full clean complete$(NC)"

#=============================================================================
# Protocol Buffers
#=============================================================================

## proto: Generate protobuf code
proto:
	@echo "$(BLUE)Generating protobuf code...$(NC)"
	@if [ -f api/proto/maia.proto ]; then \
		protoc --go_out=. --go-grpc_out=. api/proto/maia.proto; \
		echo "$(GREEN)✓ Protobuf code generated$(NC)"; \
	else \
		echo "$(YELLOW)No proto files found$(NC)"; \
	fi

#=============================================================================
# Tool Installation
#=============================================================================

## install-tools: Install all development tools
install-tools:
	@echo "$(BLUE)Installing development tools...$(NC)"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "$(GREEN)✓ Development tools installed$(NC)"

#=============================================================================
# Version Info
#=============================================================================

## version: Show version information
version:
	@echo "$(GREEN)MAIA Version Information$(NC)"
	@echo "  Version:    $(VERSION)"
	@echo "  Commit:     $(COMMIT)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Go Version: $(shell go version | cut -d' ' -f3)"
	@echo "  OS/Arch:    $(GOOS)/$(GOARCH)"

# Default target
.DEFAULT_GOAL := help
