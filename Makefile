# Makefile for ssh-plex

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse --short HEAD)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Go build flags
LDFLAGS = -s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(COMMIT) -extldflags '-static'
BUILD_FLAGS = -ldflags "$(LDFLAGS)"

# Default target
.PHONY: all
all: build

# Build for current platform
.PHONY: build
build:
	@echo "Building ssh-plex for current platform..."
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o build/ssh-plex ./cmd/ssh-plex

# Build for all platforms
.PHONY: build-all
build-all: clean
	@echo "Building ssh-plex for all platforms..."
	@mkdir -p dist
	
	# Linux builds
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/ssh-plex-linux-amd64/ssh-plex ./cmd/ssh-plex
	
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/ssh-plex-linux-arm64/ssh-plex ./cmd/ssh-plex
	
	# macOS builds
	@echo "Building for macOS amd64..."
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/ssh-plex-darwin-amd64/ssh-plex ./cmd/ssh-plex
	
	@echo "Building for macOS arm64..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/ssh-plex-darwin-arm64/ssh-plex ./cmd/ssh-plex
	
	# Windows builds
	@echo "Building for Windows amd64..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/ssh-plex-windows-amd64/ssh-plex.exe ./cmd/ssh-plex
	
	@echo "Building for Windows arm64..."
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/ssh-plex-windows-arm64/ssh-plex.exe ./cmd/ssh-plex

# Create release packages
.PHONY: package
package: build-all
	@echo "Creating release packages..."
	@cd dist && \
	for dir in ssh-plex-*/; do \
		platform=$$(basename "$$dir"); \
		echo "Packaging $$platform..."; \
		if [[ "$$platform" == *"windows"* ]]; then \
			cd "$$dir" && zip -r "../$$platform.zip" . && cd ..; \
		else \
			tar -czf "$$platform.tar.gz" -C "$$dir" .; \
		fi; \
	done
	@echo "Packages created:"
	@ls -la dist/*.zip dist/*.tar.gz 2>/dev/null || echo "No packages found"

# Test the build
.PHONY: test
test:
	@echo "Running tests..."
	@if find . -name "*_test.go" -not -path "./.git/*" | grep -q .; then \
		go test -v -race ./...; \
	else \
		echo "No test files found"; \
	fi

# Lint the code
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run --timeout=5m

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf build/ dist/

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Run the application (for testing)
.PHONY: run
run: build
	@echo "Running ssh-plex..."
	./build/ssh-plex version

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build      - Build for current platform"
	@echo "  build-all  - Build for all platforms"
	@echo "  package    - Create release packages"
	@echo "  test       - Run tests"
	@echo "  lint       - Run linter"
	@echo "  clean      - Clean build artifacts"
	@echo "  deps       - Install dependencies"
	@echo "  run        - Build and run the application"
	@echo "  help       - Show this help message"