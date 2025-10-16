# ssh-plex Makefile
# Build system for static binaries across multiple platforms

# Build configuration
BINARY_NAME := ssh-plex
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go build flags for static binaries
CGO_ENABLED := 0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(COMMIT) -extldflags '-static'"

# Target platforms
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

# Build directories
BUILD_DIR := build
DIST_DIR := dist

.PHONY: all build clean test lint cross-compile release help

# Default target
all: build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME) for current platform..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ssh-plex

# Build for all platforms
cross-compile: clean
	@echo "Cross-compiling $(BINARY_NAME) for all platforms..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d'/' -f1); \
		arch=$$(echo $$platform | cut -d'/' -f2); \
		output_name=$(BINARY_NAME); \
		if [ $$os = "windows" ]; then output_name=$(BINARY_NAME).exe; fi; \
		echo "Building for $$os/$$arch..."; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) \
			-o $(DIST_DIR)/$(BINARY_NAME)-$$os-$$arch/$$output_name ./cmd/ssh-plex; \
		if [ $$? -ne 0 ]; then \
			echo "Failed to build for $$os/$$arch"; \
			exit 1; \
		fi; \
	done

# Create release packages
release: cross-compile
	@echo "Creating release packages..."
	@cd $(DIST_DIR) && for dir in */; do \
		platform=$$(basename "$$dir"); \
		echo "Packaging $$platform..."; \
		if echo "$$platform" | grep -q "windows"; then \
			zip -r "$(BINARY_NAME)-$$platform.zip" "$$dir"; \
		else \
			tar -czf "$(BINARY_NAME)-$$platform.tar.gz" "$$dir"; \
		fi; \
	done
	@echo "Release packages created in $(DIST_DIR)/"

# Run tests (if any test files exist)
test:
	@echo "Running tests..."
	@if find . -name "*_test.go" -not -path "./.git/*" | grep -q .; then \
		go test -v ./...; \
	else \
		echo "No test files found"; \
	fi

# Run tests with coverage (if any test files exist)
test-coverage:
	@echo "Running tests with coverage..."
	@if find . -name "*_test.go" -not -path "./.git/*" | grep -q .; then \
		go test -v -coverprofile=coverage.out ./...; \
		go tool cover -html=coverage.out -o coverage.html; \
	else \
		echo "No test files found"; \
	fi

# Lint code
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found, running go vet instead"; \
		go vet ./...; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(DIST_DIR) coverage.out coverage.html

# Install binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	go install $(LDFLAGS) ./cmd/ssh-plex

# Development build with race detection
dev:
	@echo "Building development version with race detection..."
	@mkdir -p $(BUILD_DIR)
	go build -race -o $(BUILD_DIR)/$(BINARY_NAME)-dev ./cmd/ssh-plex

# Verify static binary (Linux only)
verify-static: build
	@echo "Verifying static binary..."
	@if [ "$$(uname)" = "Linux" ]; then \
		if ldd $(BUILD_DIR)/$(BINARY_NAME) 2>&1 | grep -q "not a dynamic executable"; then \
			echo "✓ Binary is statically linked"; \
		else \
			echo "✗ Binary has dynamic dependencies:"; \
			ldd $(BUILD_DIR)/$(BINARY_NAME); \
			exit 1; \
		fi; \
	else \
		echo "Static verification only available on Linux"; \
	fi

# Show help
help:
	@echo "Available targets:"
	@echo "  build          - Build for current platform"
	@echo "  cross-compile  - Build for all supported platforms"
	@echo "  release        - Create release packages"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  lint           - Run linter"
	@echo "  clean          - Clean build artifacts"
	@echo "  install        - Install binary to GOPATH/bin"
	@echo "  dev            - Build development version with race detection"
	@echo "  verify-static  - Verify binary is statically linked (Linux only)"
	@echo "  help           - Show this help"