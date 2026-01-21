# MAIA - Memory AI Architecture
# Build and development commands

.PHONY: all build build-server build-cli build-mcp build-migrate build-examples \
	dev dev-mcp test test-short test-cover test-pkg bench \
	lint fmt vet tidy deps generate clean clean-all \
	docker-build docker-run docker-compose-up docker-compose-down proto install-tools check \
	install uninstall run-example-basic run-example-proxy run-example-multi-agent \
	build-linux build-darwin build-windows build-all-platforms \
	helm-lint helm-template helm-package backup restore \
	k8s-crds-install k8s-crds-uninstall \
	operator-build operator-test operator-lint operator-manifests operator-generate \
	operator-docker-build operator-docker-push operator-deploy operator-undeploy \
	operator-install operator-uninstall operator-run operator-clean

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
BINARY_MIGRATE := maia-migrate

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
	@grep -E '^## (install|uninstall):' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Docker Targets:$(NC)"
	@grep -E '^## docker' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Helm Chart Targets:$(NC)"
	@grep -E '^## helm' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Kubernetes CRD Targets:$(NC)"
	@grep -E '^## k8s' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Operator Targets:$(NC)"
	@grep -E '^## operator' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Backup/Restore Targets:$(NC)"
	@grep -E '^## (backup|restore)' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Cleanup Targets:$(NC)"
	@grep -E '^## clean' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'
	@echo ""
	@echo "$(YELLOW)Cross-Platform Targets:$(NC)"
	@grep -E '^## build-(linux|darwin|windows|all-platforms)' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'

#=============================================================================
# Build Targets
#=============================================================================

## all: Build all binaries (server, CLI, MCP, migrate)
all: build

## build: Build all main binaries for current platform
build: build-server build-cli build-mcp build-migrate
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

## build-migrate: Build the migration tool
build-migrate:
	@echo "$(BLUE)Building $(BINARY_MIGRATE)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_MIGRATE) $(CMD_DIR)/migrate

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
	$(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/$(BINARY_MIGRATE) $(CMD_DIR)/migrate
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
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-amd64/$(BINARY_MIGRATE) $(CMD_DIR)/migrate
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/linux-arm64/$(BINARY_MIGRATE) $(CMD_DIR)/migrate
	@echo "$(GREEN)✓ Linux binaries built$(NC)"

## build-darwin: Build for macOS (amd64 and arm64)
build-darwin:
	@echo "$(BLUE)Building for macOS...$(NC)"
	@mkdir -p $(BUILD_DIR)/darwin-amd64 $(BUILD_DIR)/darwin-arm64
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_MIGRATE) $(CMD_DIR)/migrate
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_NAME) $(CMD_DIR)/maia
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_CLI) $(CMD_DIR)/maiactl
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_MCP) $(CMD_DIR)/mcp-server
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_MIGRATE) $(CMD_DIR)/migrate
	@echo "$(GREEN)✓ macOS binaries built$(NC)"

## build-windows: Build for Windows (amd64)
build-windows:
	@echo "$(BLUE)Building for Windows...$(NC)"
	@mkdir -p $(BUILD_DIR)/windows-amd64
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_NAME).exe $(CMD_DIR)/maia
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_CLI).exe $(CMD_DIR)/maiactl
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_MCP).exe $(CMD_DIR)/mcp-server
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS_RELEASE) -o $(BUILD_DIR)/windows-amd64/$(BINARY_MIGRATE).exe $(CMD_DIR)/migrate
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
	@install -m 755 $(BUILD_DIR)/$(BINARY_MIGRATE) $(INSTALL_DIR)/$(BINARY_MIGRATE)
	@echo "$(GREEN)✓ Installed:$(NC)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_NAME)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_CLI)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_MCP)"
	@echo "  - $(INSTALL_DIR)/$(BINARY_MIGRATE)"

## uninstall: Remove installed binaries from system
uninstall:
	@echo "$(BLUE)Removing binaries from $(INSTALL_DIR)...$(NC)"
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@rm -f $(INSTALL_DIR)/$(BINARY_CLI)
	@rm -f $(INSTALL_DIR)/$(BINARY_MCP)
	@rm -f $(INSTALL_DIR)/$(BINARY_MIGRATE)
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

## docker-compose-up: Start all services with docker-compose
docker-compose-up:
	@echo "$(BLUE)Starting MAIA with docker-compose...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)✓ MAIA is running at http://localhost:8080$(NC)"

## docker-compose-up-monitoring: Start all services including monitoring stack
docker-compose-up-monitoring:
	@echo "$(BLUE)Starting MAIA with monitoring stack...$(NC)"
	docker-compose --profile monitoring up -d
	@echo "$(GREEN)✓ MAIA: http://localhost:8080$(NC)"
	@echo "$(GREEN)✓ Prometheus: http://localhost:9091$(NC)"
	@echo "$(GREEN)✓ Grafana: http://localhost:3000$(NC)"

## docker-compose-down: Stop all docker-compose services
docker-compose-down:
	@echo "$(BLUE)Stopping docker-compose services...$(NC)"
	docker-compose --profile monitoring down
	@echo "$(GREEN)✓ Services stopped$(NC)"

## docker-compose-logs: View docker-compose logs
docker-compose-logs:
	docker-compose logs -f

#=============================================================================
# Helm Chart Targets
#=============================================================================

## helm-lint: Lint the Helm chart
helm-lint:
	@echo "$(BLUE)Linting Helm chart...$(NC)"
	helm lint deployments/helm/maia
	@echo "$(GREEN)✓ Helm chart is valid$(NC)"

## helm-template: Render Helm chart templates locally
helm-template:
	@echo "$(BLUE)Rendering Helm templates...$(NC)"
	helm template maia deployments/helm/maia

## helm-package: Package Helm chart for distribution
helm-package:
	@echo "$(BLUE)Packaging Helm chart...$(NC)"
	@mkdir -p $(BUILD_DIR)/helm
	helm package deployments/helm/maia -d $(BUILD_DIR)/helm
	@echo "$(GREEN)✓ Helm chart packaged to $(BUILD_DIR)/helm$(NC)"

## helm-install: Install MAIA using Helm (requires running Kubernetes cluster)
helm-install:
	@echo "$(BLUE)Installing MAIA via Helm...$(NC)"
	helm install maia deployments/helm/maia
	@echo "$(GREEN)✓ MAIA installed$(NC)"

## helm-upgrade: Upgrade MAIA Helm release
helm-upgrade:
	@echo "$(BLUE)Upgrading MAIA Helm release...$(NC)"
	helm upgrade maia deployments/helm/maia
	@echo "$(GREEN)✓ MAIA upgraded$(NC)"

## helm-uninstall: Uninstall MAIA Helm release
helm-uninstall:
	@echo "$(BLUE)Uninstalling MAIA Helm release...$(NC)"
	helm uninstall maia
	@echo "$(GREEN)✓ MAIA uninstalled$(NC)"

#=============================================================================
# Kubernetes CRD Targets
#=============================================================================

## k8s-crds-install: Install MAIA CRDs to Kubernetes cluster
k8s-crds-install:
	@echo "$(BLUE)Installing MAIA CRDs...$(NC)"
	kubectl apply -k deployments/kubernetes/crds
	@echo "$(GREEN)✓ CRDs installed$(NC)"

## k8s-crds-uninstall: Remove MAIA CRDs from Kubernetes cluster
k8s-crds-uninstall:
	@echo "$(BLUE)Removing MAIA CRDs...$(NC)"
	kubectl delete -k deployments/kubernetes/crds
	@echo "$(GREEN)✓ CRDs removed$(NC)"

## k8s-examples: Apply example Kubernetes resources
k8s-examples:
	@echo "$(BLUE)Applying example resources...$(NC)"
	kubectl apply -f deployments/kubernetes/examples/
	@echo "$(GREEN)✓ Examples applied$(NC)"

#=============================================================================
# Backup and Restore Targets
#=============================================================================

## backup: Create a backup of MAIA data
backup:
	@echo "$(BLUE)Creating backup...$(NC)"
	./scripts/backup.sh --data-dir ./data --output-dir ./backups --compress
	@echo "$(GREEN)✓ Backup complete$(NC)"

## backup-encrypted: Create an encrypted backup (requires GPG_RECIPIENT)
backup-encrypted:
	@if [ -z "$(GPG_RECIPIENT)" ]; then \
		echo "$(YELLOW)GPG_RECIPIENT not set. Usage: GPG_RECIPIENT=admin@example.com make backup-encrypted$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Creating encrypted backup...$(NC)"
	./scripts/backup.sh --data-dir ./data --output-dir ./backups --compress --encrypt
	@echo "$(GREEN)✓ Encrypted backup complete$(NC)"

## restore: Restore from a backup file (usage: make restore BACKUP=backups/maia_backup_xxx.tar.gz)
restore:
	@if [ -z "$(BACKUP)" ]; then \
		echo "$(YELLOW)Usage: make restore BACKUP=backups/maia_backup_xxx.tar.gz$(NC)"; \
		exit 1; \
	fi
	@echo "$(BLUE)Restoring from $(BACKUP)...$(NC)"
	./scripts/restore.sh --data-dir ./data $(BACKUP)
	@echo "$(GREEN)✓ Restore complete$(NC)"

## backup-list: List available backups
backup-list:
	@echo "$(BLUE)Available backups:$(NC)"
	@ls -lh ./backups/*.tar* 2>/dev/null || echo "  No backups found in ./backups/"

#=============================================================================
# Kubernetes Operator Targets
#=============================================================================

# Operator variables
OPERATOR_DIR := ./operator
OPERATOR_IMG ?= ghcr.io/ar4mirez/maia-operator:$(VERSION)

## operator-build: Build the Kubernetes operator binary
operator-build:
	@echo "$(BLUE)Building MAIA operator...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) build
	@echo "$(GREEN)✓ Operator built successfully$(NC)"

## operator-test: Run operator tests
operator-test:
	@echo "$(BLUE)Running operator tests...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) test
	@echo "$(GREEN)✓ Operator tests passed$(NC)"

## operator-lint: Run linter on operator code
operator-lint:
	@echo "$(BLUE)Linting operator code...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) lint
	@echo "$(GREEN)✓ Operator linting passed$(NC)"

## operator-manifests: Generate operator CRD manifests
operator-manifests:
	@echo "$(BLUE)Generating operator manifests...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) manifests
	@echo "$(GREEN)✓ Operator manifests generated$(NC)"

## operator-generate: Generate operator DeepCopy code
operator-generate:
	@echo "$(BLUE)Generating operator code...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) generate
	@echo "$(GREEN)✓ Operator code generated$(NC)"

## operator-docker-build: Build operator Docker image
operator-docker-build:
	@echo "$(BLUE)Building operator Docker image...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) docker-build IMG=$(OPERATOR_IMG)
	@echo "$(GREEN)✓ Operator image built: $(OPERATOR_IMG)$(NC)"

## operator-docker-push: Push operator Docker image to registry
operator-docker-push:
	@echo "$(BLUE)Pushing operator Docker image...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) docker-push IMG=$(OPERATOR_IMG)
	@echo "$(GREEN)✓ Operator image pushed$(NC)"

## operator-docker-buildx: Build and push multi-arch operator image
operator-docker-buildx:
	@echo "$(BLUE)Building multi-arch operator Docker image...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) docker-buildx IMG=$(OPERATOR_IMG)
	@echo "$(GREEN)✓ Multi-arch operator image pushed$(NC)"

## operator-install: Install operator CRDs to cluster
operator-install:
	@echo "$(BLUE)Installing operator CRDs...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) install
	@echo "$(GREEN)✓ Operator CRDs installed$(NC)"

## operator-uninstall: Uninstall operator CRDs from cluster
operator-uninstall:
	@echo "$(BLUE)Uninstalling operator CRDs...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) uninstall
	@echo "$(GREEN)✓ Operator CRDs uninstalled$(NC)"

## operator-deploy: Deploy operator to cluster
operator-deploy:
	@echo "$(BLUE)Deploying operator...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) deploy
	@echo "$(GREEN)✓ Operator deployed$(NC)"

## operator-undeploy: Undeploy operator from cluster
operator-undeploy:
	@echo "$(BLUE)Undeploying operator...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) undeploy
	@echo "$(GREEN)✓ Operator undeployed$(NC)"

## operator-run: Run operator locally (outside cluster)
operator-run:
	@echo "$(BLUE)Running operator locally...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) run

## operator-clean: Clean operator build artifacts
operator-clean:
	@echo "$(BLUE)Cleaning operator artifacts...$(NC)"
	cd $(OPERATOR_DIR) && $(MAKE) clean
	@echo "$(GREEN)✓ Operator artifacts cleaned$(NC)"

## operator-all: Build and test operator
operator-all: operator-manifests operator-generate operator-build operator-test
	@echo "$(GREEN)✓ Operator build and test complete$(NC)"

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

## clean-all: Clean everything including root binaries, cache, and operator
clean-all: clean operator-clean
	@echo "$(BLUE)Cleaning root binaries and cache...$(NC)"
	@rm -f ./maia ./maiactl ./maia-mcp ./mcp-server ./migrate ./maia-migrate
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
