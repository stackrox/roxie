# Makefile for roxie - Advanced Cluster Security Deployment Tool

# Default target
.DEFAULT_GOAL := help

# Python paths to check/format (entire repo)
PYTHON_PATHS := .

# Help target
.PHONY: help
help: ## Show this help message
	@echo "🚀 roxie - Advanced Cluster Security Deployment Tool"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# Development targets
.PHONY: check
check: lint typecheck ## Run all code quality checks (ruff + mypy)

.PHONY: lint
lint: ## Run ruff linter and check code style
	@echo "🔍 Running ruff linter..."
	ruff check $(PYTHON_PATHS)

.PHONY: format
format: ## Format code with ruff
	@echo "🎨 Formatting code with ruff..."
	ruff format $(PYTHON_PATHS)

.PHONY: fmt
fmt: format ## Alias for format

.PHONY: fix
fix: ## Auto-fix linting issues with ruff
	@echo "🔧 Auto-fixing code issues..."
	ruff check --fix $(PYTHON_PATHS)

.PHONY: typecheck
typecheck: ## Run mypy type checker
	@echo "🔍 Running mypy type checker..."
	mypy $(PYTHON_PATHS) --ignore-missing-imports

# Test target
.PHONY: test
test: ## Run tests with pytest
	@echo "🧪 Running tests with pytest..."
	pytest -q

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (requires kubectl context and cluster access)
	@echo "🧪 Running e2e tests..."
	pytest -m e2e -s -rs

# Clean up generated files
.PHONY: clean
clean: ## Clean up generated files and caches
	@echo "🧹 Cleaning up..."
	find . -type f -name "*.pyc" -delete
	find . -type d -name "__pycache__" -delete
	find . -type d -name ".mypy_cache" -delete
	find . -type d -name ".ruff_cache" -delete
	rm -rf dist/ build/ *.egg-info/

# Development environment setup
.PHONY: dev-setup
dev-setup: ## Set up development environment (requires Nix)
	@echo "🔧 Setting up development environment..."
	@if command -v nix >/dev/null 2>&1; then \
		echo "✅ Nix found - use 'nix develop' or './shell.sh' to enter dev environment"; \
	else \
		echo "❌ Nix not found. Install Nix first:"; \
		echo "   curl --proto '=https' --tlsv1.2 -sSf -L https://install.determinate.systems/nix | sh -s -- install"; \
	fi

# Run roxie help
.PHONY: run-help
run-help: ## Show roxie command help
	@echo "🚀 roxie help:"
	./roxie deploy --help

# Validate configuration files
.PHONY: validate
validate: ## Validate configuration files
	@echo "✅ Validating configuration files..."
	@if command -v nix >/dev/null 2>&1; then \
		nix --extra-experimental-features 'nix-command flakes' flake check; \
	else \
		echo "⚠️  Nix not available - skipping flake validation"; \
	fi
	@python -c "import tomllib; tomllib.load(open('pyproject.toml', 'rb'))" && echo "✅ pyproject.toml is valid"

# Full development workflow
.PHONY: all
all: check validate ## Run all checks and validations

# Display project code statistics
.PHONY: stats
stats:
	tokei
