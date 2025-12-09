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
VERSION := 0.1
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildDate=$(BUILD_DATE)

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
	@if ! command -v podman >/dev/null 2>&1; then \
		echo "❌ podman not found. Please install podman for E2E tests."; \
		exit 1; \
	fi
	$(GOTEST) -v -tags=e2e -timeout=120m -parallel=1 ./tests/e2e/...

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
DOCKER_IMAGE := roxie
DOCKER_TAG := latest
DOCKER_VERSION_TAG := $(VERSION)-$(GIT_COMMIT)
DOCKER_FULL_IMAGE := $(DOCKER_IMAGE):$(DOCKER_TAG)
DOCKER_VERSION_IMAGE := $(DOCKER_IMAGE):$(DOCKER_VERSION_TAG)
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
		-t $(DOCKER_FULL_IMAGE) \
		-t $(DOCKER_VERSION_IMAGE) \
		-f Dockerfile .
	@echo "✅ Built container images:"
	@echo "   - $(DOCKER_FULL_IMAGE)"
	@echo "   - $(DOCKER_VERSION_IMAGE)"

.PHONY: docker-build-multiarch
docker-build-multiarch: ## Build multi-architecture images (amd64, arm64) using buildx
	@echo "🏗️  Building multi-architecture roxie container images..."
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "❌ Docker is required for multi-arch builds (buildx)"; \
		exit 1; \
	fi
	@if ! docker buildx version >/dev/null 2>&1; then \
		echo "❌ Docker buildx is required for multi-arch builds"; \
		echo "Install: docker buildx install"; \
		exit 1; \
	fi
	@echo "Creating/using buildx builder..."
	@docker buildx create --name roxie-builder --use 2>/dev/null || docker buildx use roxie-builder
	@echo "Building for platforms: $(PLATFORMS)"
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_FULL_IMAGE) \
		-t $(DOCKER_VERSION_IMAGE) \
		--load \
		-f Dockerfile .
	@echo "✅ Built multi-arch images:"
	@echo "   - $(DOCKER_FULL_IMAGE)"
	@echo "   - $(DOCKER_VERSION_IMAGE)"

.PHONY: docker-build-push-multiarch
docker-build-push-multiarch: ## Build and push multi-arch images to registry (requires DOCKER_REGISTRY)
	@echo "🚀 Building and pushing multi-architecture images..."
	@if [ -z "$(DOCKER_REGISTRY)" ]; then \
		echo "❌ DOCKER_REGISTRY is required. Example: make docker-build-push-multiarch DOCKER_REGISTRY=ghcr.io/myorg"; \
		exit 1; \
	fi
	@if ! docker buildx version >/dev/null 2>&1; then \
		echo "❌ Docker buildx is required for multi-arch builds"; \
		exit 1; \
	fi
	@docker buildx create --name roxie-builder --use 2>/dev/null || docker buildx use roxie-builder
	docker buildx build \
		--platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_VERSION_TAG) \
		-t $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(VERSION) \
		--push \
		-f Dockerfile .
	@echo "✅ Pushed multi-arch images:"
	@echo "   - $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)"
	@echo "   - $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_VERSION_TAG)"
	@echo "   - $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(VERSION)"

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
		-t $(DOCKER_IMAGE):$(DOCKER_TAG)-arm64 \
		-t $(DOCKER_IMAGE):$(DOCKER_VERSION_TAG)-arm64 \
		-f Dockerfile .
	@echo "✅ Built arm64 images:"
	@echo "   - $(DOCKER_IMAGE):$(DOCKER_TAG)-arm64"
	@echo "   - $(DOCKER_IMAGE):$(DOCKER_VERSION_TAG)-arm64"

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
		-t $(DOCKER_IMAGE):$(DOCKER_TAG)-amd64 \
		-t $(DOCKER_IMAGE):$(DOCKER_VERSION_TAG)-amd64 \
		-f Dockerfile .
	@echo "✅ Built amd64 images:"
	@echo "   - $(DOCKER_IMAGE):$(DOCKER_TAG)-amd64"
	@echo "   - $(DOCKER_IMAGE):$(DOCKER_VERSION_TAG)-amd64"

.PHONY: docker-test-podman
docker-test-podman: ## Test podman functionality inside the roxie container
	@echo "🧪 Testing podman inside roxie container..."
	@echo ""
	@echo "1. Testing podman pull (operator bundle)..."
	@$(CONTAINER_RUNTIME) run --rm \
		--entrypoint podman \
		$(DOCKER_FULL_IMAGE) \
		pull quay.io/rhacs-eng/stackrox-operator-bundle:v4.4.3
	@echo ""
	@echo "2. Testing podman inspect..."
	@$(CONTAINER_RUNTIME) run --rm \
		--entrypoint podman \
		$(DOCKER_FULL_IMAGE) \
		inspect quay.io/rhacs-eng/stackrox-operator-bundle:v4.4.3 > /dev/null
	@echo "✓ Podman can pull and inspect images successfully"
	@echo ""
	@echo "3. Cleaning up test image..."
	@$(CONTAINER_RUNTIME) run --rm \
		--entrypoint podman \
		$(DOCKER_FULL_IMAGE) \
		rmi quay.io/rhacs-eng/stackrox-operator-bundle:v4.4.3
	@echo "✓ Podman test complete"

.PHONY: docker-clean
docker-clean: ## Remove roxie Docker images
	@echo "🧹 Cleaning up roxie container images..."
	@if [ -z "$(CONTAINER_RUNTIME)" ]; then \
		echo "❌ No container runtime found. Please install docker or podman."; \
		exit 1; \
	fi
	$(CONTAINER_RUNTIME) rmi $(DOCKER_FULL_IMAGE) 2>/dev/null || true
	@echo "✅ Cleanup complete"

# Quick targets
.PHONY: quick
quick: fmt vet build ## Quick build (fmt + vet + build)
