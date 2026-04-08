# Makefile for roxie - Advanced Cluster Security Deployment Tool

# Default target
.DEFAULT_GOAL := build

# Go configuration
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOVET := $(GOCMD) vet
GOFMT := gofmt
GOLINT := golangci-lint
BINARY_NAME := roxie
PKG_LIST := $(shell $(GOCMD) list ./... | grep -v /vendor/)

# Build output
BUILD_DIR := .
BINARY := $(BUILD_DIR)/$(BINARY_NAME)

# Version information
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
# Convention is that the git tags are of the form
#      v<major>.<minor>.<patch>-<build-number>-<commit-hash>[-dirty]
#   or v<major>.<minor>.<patch>
#
# We use sed to drop the initial 'v' in case the whole tag matches any of the above patterns.
# Hence, the resulting version string will simply be
#
#     <major>.<minor>.<patch> or <major>.<minor>.<patch>-<build-number>-<commit-hash>[-dirty]
#
# This will also become the tag of the docker images.
VERSION := $(shell git describe --tags --always --dirty | sed -E 's/^v([0-9]+\.[0-9]+\.[0-9]+-[0-9]+-[a-z0-9]+(-dirty)?$$)/\1/')
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: get-build-date
get-build-date:
	@echo $(BUILD_DATE)

.PHONY: get-commit-hash
get-commit-hash:
	@echo $(GIT_COMMIT)

.PHONY: version
version:
	@echo $(VERSION)

# Build targets
.PHONY: build
build: ## Build the roxie binary
	@echo "🔨 Building roxie..."
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd
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
	$(GOTEST) -v -tags=e2e -timeout=120m -parallel=1 ./tests/e2e/...

.PHONY: test-integration
test-integration: build ## Run integration tests (requires kubectl context and cluster access)
	@echo "🧪 Running integration tests..."
	@if [ -z "$(shell kubectl config current-context 2>/dev/null)" ]; then \
		echo "❌ No kubectl context found. Please configure kubectl first."; \
		exit 1; \
	fi
	$(GOTEST) -v -tags=integration -run=_Integration$$ -timeout=120m -parallel=1 ./...

.PHONY: test-all
test-all: test test-integration test-e2e ## Run all tests (unit + integration + e2e)

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
# Validate
.PHONY: validate
validate: ## Validate go.mod and check for issues
	@echo "✅ Validating go.mod..."
	@$(GOCMD) mod verify
	@echo "✅ go.mod is valid"

# Full development workflow
.PHONY: all
all: clean deps check test build ## Run full development workflow

# Docker/Container targets
IMAGE_DEFAULT_REGISTRY := localhost
IMAGE_REGISTRY := $(shell if [ -z "$(IMAGE_REGISTRY)" ]; then echo $(IMAGE_DEFAULT_REGISTRY); else echo $(IMAGE_REGISTRY); fi)
IMAGE_NAME := roxie
IMAGE_LATEST_TAG := $(IMAGE_REGISTRY)/$(IMAGE_NAME):latest
IMAGE_VERSION_TAG := $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(VERSION)
CONTAINER_RUNTIME ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# Multi-architecture support
PLATFORMS ?= linux/amd64,linux/arm64
BUILD_PLATFORM := $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

.PHONY: docker-build
docker-build: ## Build roxie Docker image for current platform
	@echo "🐳 Building roxie container image for current platform..."
	@if [ -z "$(CONTAINER_RUNTIME)" ]; then \
		echo "❌ No container runtime found. Please install docker or podman."; \
		exit 1; \
	fi
	$(CONTAINER_RUNTIME) build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_LATEST_TAG) \
		-t $(IMAGE_VERSION_TAG) \
		-f Dockerfile .
	@echo "✅ Built container images:"
	@echo "   - $(IMAGE_LATEST_TAG)"
	@echo "   - $(IMAGE_VERSION_TAG)"


.PHONY: docker-build-arm64
docker-build-arm64: ## Build roxie Docker image for arm64
	@echo "🐳 Building roxie container image for arm64..."
	@if [ -z "$(CONTAINER_RUNTIME)" ]; then \
		echo "❌ No container runtime found. Please install docker or podman."; \
		exit 1; \
	fi
	$(CONTAINER_RUNTIME) build \
		--platform linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_LATEST_TAG)-arm64 \
		-t $(IMAGE_VERSION_TAG)-arm64 \
		-f Dockerfile .
	@echo "✅ Built arm64 images:"
	@echo "   - $(IMAGE_LATEST_TAG)-arm64"
	@echo "   - $(IMAGE_VERSION_TAG)-arm64"

.PHONY: docker-build-amd64
docker-build-amd64: ## Build roxie Docker image for amd64
	@echo "🐳 Building roxie container image for amd64..."
	@if [ -z "$(CONTAINER_RUNTIME)" ]; then \
		echo "❌ No container runtime found. Please install docker or podman."; \
		exit 1; \
	fi
	$(CONTAINER_RUNTIME) build \
		--platform linux/amd64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE_LATEST_TAG)-amd64 \
		-t $(IMAGE_VERSION_TAG)-amd64 \
		-f Dockerfile .
	@echo "✅ Built amd64 images:"
	@echo "   - $(IMAGE_LATEST_TAG)-amd64"
	@echo "   - $(IMAGE_VERSION_TAG)-amd64"

# Quick targets
.PHONY: quick
quick: fmt vet build ## Quick build (fmt + vet + build)
