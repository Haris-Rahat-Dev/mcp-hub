# Makefile for mcp-hub
# Provides all common development commands

# Binary names
BINARIES := mcp-hub mcp-hub-bridge mcp-hub-fixture
# Platform-specific binary names (for Windows)
BINARIES_WIN := $(BINARIES:=.exe)

# Build output directory
BUILD_DIR := .
# Frontend directory
FRONTEND_DIR := frontend

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet
GOMOD := $(GOCMD) mod
GOTIDY := $(GOMOD) tidy
GOCLEAN := $(GOCMD) clean

# Linting
LINTER := golangci-lint
LINTER_VERSION := v2.11.4
LINTER_MODULE := github.com/golangci/golangci-lint/v2/cmd/golangci-lint

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Default target
.PHONY: help
help: ## Show this help message
	@echo "mcp-hub Development Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | grep -v "help" | sort | awk 'BEGIN {FS = ":.*## "}; {printf "  $(BLUE)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "Run 'make help' for more information."

## Build Commands

build: ## Build all binaries (mcp-hub, mcp-hub-bridge, mcp-hub-fixture)
build: build/cli build/bridge build/fixture

build/cli: ## Build mcp-hub CLI daemon
	@echo "$(GREEN)Building mcp-hub...$(NC)"
	$(GOBUILD) -o $(BUILD_DIR)/mcp-hub ./cmd/mcp-hub

build/bridge: ## Build mcp-hub-bridge binary
	@echo "$(GREEN)Building mcp-hub-bridge...$(NC)"
	$(GOBUILD) -o $(BUILD_DIR)/mcp-hub-bridge ./cmd/mcp-hub-bridge

build/fixture: ## Build mcp-hub-fixture test server
	@echo "$(GREEN)Building mcp-hub-fixture...$(NC)"
	$(GOBUILD) -o $(BUILD_DIR)/mcp-hub-fixture ./cmd/mcp-hub-fixture

build/gui: ## Build GUI binary (requires Wails)
	@echo "$(GREEN)Building mcp-hub-gui...$(NC)"
	cd $(FRONTEND_DIR) && npm run build
	$(GOBUILD) -o $(BUILD_DIR)/mcp-hub-gui .

build/all: build ## Build all binaries including GUI

## Development Commands

dev: ## Run mcp-hub in development mode (Wails dev server)
	@echo "$(GREEN)Starting mcp-hub in development mode...$(NC)"
	cd $(FRONTEND_DIR) && npm run dev & \
	$(GOCMD) run ./cmd/mcp-hub start

dev/cli: ## Run mcp-hub CLI in development mode (hot reload via -ldflags)
	@echo "$(GREEN)Starting mcp-hub CLI in development mode...$(NC)"
	$(GOCMD) run ./cmd/mcp-hub start

## Testing Commands

test: ## Run all tests
	@echo "$(GREEN)Running tests...$(NC)"
	$(GOTEST) -v -race -tags exclude_wails ./...

test/unit: ## Run unit tests only (no integration tests)
	@echo "$(GREEN)Running unit tests...$(NC)"
	$(GOTEST) -v -race -short -tags exclude_wails ./...

test/cover: ## Run tests with coverage report
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	$(GOTEST) -v -race -tags exclude_wails -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

test/bench: ## Run benchmarks
	@echo "$(GREEN)Running benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem ./...

test/watch: ## Run tests with watch mode (requires gotestsum)
	@echo "$(YELLOW)Running tests in watch mode...$(NC)"
	which gotestsum > /dev/null || (echo "$(RED)gotestsum not installed. Run: go install gotest.tools/gotestsum@latest$(NC)" && exit 1)
	gotestsum --watch

## Code Quality Commands

fmt: ## Format all Go code
	@echo "$(GREEN)Formatting Go code...$(NC)"
	$(GOFMT) ./...

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GOVET) -tags exclude_wails ./...

tidy: ## Run go mod tidy
	@echo "$(GREEN)Running go mod tidy...$(NC)"
	$(GOTIDY)

lint: ## Run linter (installs if needed)
	@echo "$(GREEN)Running linter...$(NC)"
	@if ! which $(LINTER) > /dev/null || ! $(LINTER) version | grep -q "$(LINTER_VERSION)"; then \
		echo "$(YELLOW)Installing golangci-lint $(LINTER_VERSION)...$(NC)"; \
		$(MAKE) install/linter; \
	fi
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; \
	$(LINTER) run ./...

lint/fix: ## Run linter with auto-fix
	@echo "$(GREEN)Running linter with auto-fix...$(NC)"
	@if ! which $(LINTER) > /dev/null || ! $(LINTER) version | grep -q "$(LINTER_VERSION)"; then \
		echo "$(YELLOW)Installing golangci-lint $(LINTER_VERSION)...$(NC)"; \
		$(MAKE) install/linter; \
	fi
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; \
	$(LINTER) run --fix ./...

lint/ci: ## Run linter in CI mode (for GitHub Actions)
	@echo "$(GREEN)Running linter in CI mode...$(NC)"
	@if ! which $(LINTER) > /dev/null; then \
		echo "$(RED)golangci-lint not found. Run: make install/linter$(NC)" && exit 1; \
	fi
	@export PATH="$$(go env GOPATH)/bin:$$PATH"; \
	$(LINTER) run --timeout 5m --color never ./...

## Clean Commands

clean: ## Clean all build artifacts
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)/mcp-hub $(BUILD_DIR)/mcp-hub.exe
	rm -rf $(BUILD_DIR)/mcp-hub-bridge $(BUILD_DIR)/mcp-hub-bridge.exe
	rm -rf $(BUILD_DIR)/mcp-hub-fixture $(BUILD_DIR)/mcp-hub-fixture.exe
	rm -rf $(BUILD_DIR)/mcp-hub-gui $(BUILD_DIR)/mcp-hub-gui.exe
	rm -f coverage.out coverage.html
	rm -rf $(FRONTEND_DIR)/dist

distclean: clean ## Clean everything including downloaded dependencies
	@echo "$(RED)Removing all dependencies...$(NC)"
	$(GOCMD) clean -mod
	rm -rf $(FRONTEND_DIR)/node_modules

## Installation Commands

install/linter: ## Install golangci-lint
	@echo "$(GREEN)Installing golangci-lint $(LINTER_VERSION)...$(NC)"
	$(GOCMD) install $(LINTER_MODULE)@$(LINTER_VERSION)
	@echo "$(GREEN)golangci-lint installed to $$(go env GOPATH)/bin$(NC)"

install: install/linter ## Install development dependencies

## Release Commands (requires goreleaser)

release/snapshot: ## Build snapshot release
	@echo "$(GREEN)Building snapshot release...$(NC)"
	goreleaser build --snapshot --clean

release: ## Build official release (requires git tag)
	@echo "$(GREEN)Building official release...$(NC)"
	goreleaser release --clean

## Docker Commands

docker/build: ## Build Docker image
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t mcp-hub:latest .

docker/run: ## Run mcp-hub in Docker
	@echo "$(GREEN)Running mcp-hub in Docker...$(NC)"
	docker run -p 4117:4117 -v ~/.config/mcp-hub:/home/mcp-hub/.config/mcp-hub mcp-hub:latest

## CI Commands (for GitHub Actions)

ci: lint/ci test ## Run full CI pipeline

## Misc Commands

version: ## Show version info
	@$(GOCMD) version
	@if which $(LINTER) > /dev/null; then $(LINTER) version; fi

check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

# Phony targets that are not file names
.PHONY: help build build/cli build/bridge build/fixture build/gui build/all \
	dev dev/cli test test/unit test/cover test/bench test/watch \
	fmt vet tidy lint lint/fix lint/ci \
	clean distclean install/linter install \
	release/snapshot release \
	docker/build docker/run \
	ci version check
