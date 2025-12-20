.PHONY: build build-arm build-amd64 build-native build-host clean test lint run deps fmt dist deploy deploy-test

# Binary name
BINARY_NAME=lsc

# Build directory
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Version handling
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build flags
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

# Default target: build for ARM
build: build-arm

# Build for ARM (target platform)
build-arm:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME) for ARM"

# Build for AMD64 (development/testing)
build-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-amd64 .
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)-amd64 for AMD64"

# Build for native platform
build-native:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-native .
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)-native for native platform"

# Build for host platform (alias for build-native)
build-host: build-native

# Clean build artifacts
clean:
	@$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "Cleaned build artifacts"

# Run tests
test:
	@$(GOTEST) -v ./...

# Run linter
lint:
	@golangci-lint run

# Run locally (requires Redis)
run:
	@$(GOCMD) run . status

# Install dependencies
deps:
	@$(GOMOD) download
	@$(GOMOD) tidy

# Format code
fmt:
	@go fmt ./...

# Build distribution binary (stripped ARM build)
dist:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GOBUILD) -ldflags "-s -w -X main.version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME) for ARM (distribution)"

# Deploy to Deep Blue (requires deep-blue ssh alias)
deploy:
	@echo "Building for ARM..."
	@make build-arm
	@echo "Copying to Deep Blue..."
	@scp $(BUILD_DIR)/$(BINARY_NAME) deep-blue:/data/$(BINARY_NAME)-$$(date +%s)
	@echo "Deployed to /data/$(BINARY_NAME)-<timestamp>"
	@echo "To install: ssh deep-blue 'cp /data/$(BINARY_NAME)-* /usr/local/bin/$(BINARY_NAME)'"

# Quick deploy and test
deploy-test: deploy
	@echo "Testing basic status command..."
	@ssh deep-blue "/data/$(BINARY_NAME)-* status"
