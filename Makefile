# Makefile for roxie-golang - Advanced Cluster Security Deployment Tool

# Default target
.DEFAULT_GOAL := help

# Go configuration
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := gofmt
GOLINT := golangci-lint
BINARY_NAME := roxie
CMD_PATH := ./cmd/roxie
PKG_LIST := $(shell $(GOCMD) list ./... | grep -v /vendor/)

# Build output
BUILD_DIR := .
BINARY := $(BUILD_DIR)/$(BINARY_NAME)

# Version information
VERSION := 0.1
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)

# Help target
.PHONY: help
help: ## Show this help message
	@echo "🚀 roxie-golang - Advanced Cluster Security Deployment Tool"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Build targets
.PHONY: build
build: ## Build the roxie binary
	@echo "🔨 Building roxie..."
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD_PATH)
	@echo "✅ Build complete: $(BINARY)"

.PHONY: build-all
build-all: fmt vet build ## Format, vet, and build

.PHONY: install
install: ## Install roxie to $GOPATH/bin
	@echo "📦 Installing roxie..."
	$(GOCMD) install -ldflags "$(LDFLAGS)" $(CMD_PATH)

# Development targets
.PHONY: fmt
fmt: ## Format Go code with gofmt
	@echo "🎨 Formatting code..."
	$(GOFMT) -s -w .

.PHONY: vet
vet: ## Run go vet
	@echo "🔍 Running go vet..."
	$(GOVET) ./...

.PHONY: lint
lint: ## Run golangci-lint
	@echo "🔍 Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "⚠️  golangci-lint not installed. Install with:"; \
		echo "   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

.PHONY: check
check: fmt vet lint ## Run all code quality checks (fmt + vet + lint)

# Test targets
.PHONY: test
test: ## Run unit tests
	@echo "🧪 Running unit tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

.PHONY: test-short
test-short: ## Run unit tests (short mode)
	@echo "🧪 Running unit tests (short)..."
	$(GOTEST) -v -short -race ./...

.PHONY: test-coverage
test-coverage: test ## Run tests with coverage report
	@echo "📊 Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

.PHONY: test-e2e
test-e2e: build ## Run end-to-end tests (requires kubectl context and cluster access)
	@echo "🧪 Running E2E tests..."
	@if [ -z "$(shell kubectl config current-context 2>/dev/null)" ]; then \
		echo "❌ No kubectl context found. Please configure kubectl first."; \
		exit 1; \
	fi
	@if ! command -v skopeo >/dev/null 2>&1; then \
		echo "❌ skopeo not found. Please install skopeo for E2E tests."; \
		exit 1; \
	fi
	$(GOTEST) -v -tags=e2e -timeout=120m ./tests/e2e/...

.PHONY: test-all
test-all: test test-e2e ## Run all tests (unit + e2e)

# Benchmarks
.PHONY: bench
bench: ## Run benchmarks
	@echo "⚡ Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Clean up
.PHONY: clean
clean: ## Clean up build artifacts and caches
	@echo "🧹 Cleaning up..."
	@rm -f $(BINARY)
	@rm -f coverage.out coverage.html
	@$(GOCMD) clean -cache -testcache
	@find . -type f -name "*.test" -delete
	@echo "✅ Clean complete"

# Dependencies
.PHONY: deps
deps: ## Download and tidy dependencies
	@echo "📦 Downloading dependencies..."
	$(GOCMD) mod download
	$(GOCMD) mod tidy

.PHONY: deps-upgrade
deps-upgrade: ## Upgrade all dependencies
	@echo "⬆️  Upgrading dependencies..."
	$(GOCMD) get -u ./...
	$(GOCMD) mod tidy

# Development environment
.PHONY: dev-setup
dev-setup: ## Set up development environment
	@echo "🔧 Setting up development environment..."
	@echo "Installing development tools..."
	@$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "✅ Development environment ready"

# Run roxie help
.PHONY: run-help
run-help: build ## Show roxie command help
	@echo "🚀 roxie help:"
	@./$(BINARY_NAME) --help

.PHONY: run-deploy-help
run-deploy-help: build ## Show roxie deploy command help
	@./$(BINARY_NAME) deploy --help

# Validate
.PHONY: validate
validate: ## Validate go.mod and check for issues
	@echo "✅ Validating go.mod..."
	@$(GOCMD) mod verify
	@echo "✅ go.mod is valid"

# Full development workflow
.PHONY: all
all: clean deps check test build ## Run full development workflow

# Display project statistics
.PHONY: stats
stats: ## Display code statistics
	@echo "📊 Code statistics:"
	@echo ""
	@echo "Go files:"
	@find . -name "*.go" -not -path "./vendor/*" | wc -l
	@echo ""
	@echo "Lines of code:"
	@find . -name "*.go" -not -path "./vendor/*" -exec cat {} \; | wc -l
	@echo ""
	@echo "Test files:"
	@find . -name "*_test.go" -not -path "./vendor/*" | wc -l

# Docker targets (optional)
.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	docker build -t roxie:latest .

# Quick targets
.PHONY: quick
quick: fmt vet build ## Quick build (fmt + vet + build)
