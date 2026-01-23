# Sherlock Makefile
# SSH/Database/Cache connection manager

# Variables
BINARY_NAME := sherlock
MODULE := github.com/warm3snow/sherlock
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GO_VERSION := $(shell go version | cut -d ' ' -f 3)

# Build flags
LDFLAGS := -ldflags "-s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildTime=$(BUILD_TIME) \
	-X main.Commit=$(COMMIT)"

# Directories
BUILD_DIR := build
CMD_DIR := cmd/sherlock

# Install path
PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin

# Go settings
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for current platform with debug info
.PHONY: build-debug
build-debug:
	@echo "Building $(BINARY_NAME) with debug info..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Debug build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Install to system
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to $(BINDIR)..."
	@install -d $(BINDIR)
	@install -m 755 $(BUILD_DIR)/$(BINARY_NAME) $(BINDIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(BINDIR)/$(BINARY_NAME)"

# Uninstall from system
.PHONY: uninstall
uninstall:
	@echo "Removing $(BINARY_NAME) from $(BINDIR)..."
	@rm -f $(BINDIR)/$(BINARY_NAME)
	@echo "Uninstalled $(BINARY_NAME)"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(BUILD_DIR)
	go test -coverprofile=$(BUILD_DIR)/coverage.out ./...
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "Coverage report: $(BUILD_DIR)/coverage.html"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@go clean
	@echo "Clean complete"

# Run go mod tidy
.PHONY: tidy
tidy:
	@echo "Tidying go modules..."
	go mod tidy

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	go mod download

# Run linter (requires golangci-lint)
.PHONY: lint
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Vet code
.PHONY: vet
vet:
	@echo "Vetting code..."
	go vet ./...

# Build for all platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows

# Build for Linux
.PHONY: build-linux
build-linux:
	@echo "Building for Linux (amd64)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	@echo "Building for Linux (arm64)..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)

# Build for macOS
.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS (amd64)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	@echo "Building for macOS (arm64)..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)

# Build for Windows
.PHONY: build-windows
build-windows:
	@echo "Building for Windows (amd64)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)

# Run the application
.PHONY: run
run:
	@go run ./$(CMD_DIR)

# Show version info
.PHONY: version
version:
	@echo "Binary: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "OS/Arch: $(GOOS)/$(GOARCH)"

# Generate and show help
.PHONY: help
help:
	@echo "Sherlock - SSH/Database/Cache Connection Manager"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@echo "  build         Build the binary (default)"
	@echo "  build-debug   Build with debug info"
	@echo "  build-all     Build for all platforms"
	@echo "  build-linux   Build for Linux (amd64, arm64)"
	@echo "  build-darwin  Build for macOS (amd64, arm64)"
	@echo "  build-windows Build for Windows (amd64)"
	@echo ""
	@echo "Install targets:"
	@echo "  install       Install to $(BINDIR)"
	@echo "  uninstall     Remove from $(BINDIR)"
	@echo ""
	@echo "Development targets:"
	@echo "  run           Run the application"
	@echo "  test          Run tests"
	@echo "  test-coverage Run tests with coverage"
	@echo "  lint          Run linter"
	@echo "  fmt           Format code"
	@echo "  vet           Vet code"
	@echo ""
	@echo "Maintenance targets:"
	@echo "  clean         Clean build artifacts"
	@echo "  tidy          Tidy go modules"
	@echo "  deps          Download dependencies"
	@echo "  version       Show version info"
	@echo ""
	@echo "Variables:"
	@echo "  PREFIX        Install prefix (default: /usr/local)"
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make install            # Build and install to /usr/local/bin"
	@echo "  make install PREFIX=~   # Install to ~/bin"
	@echo "  make test               # Run all tests"
