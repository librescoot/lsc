.PHONY: build build-arm build-amd64 clean test lint run

# Binary name
BINARY=lsc

# Build directory
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target: build for ARM
build: build-arm

# Build for ARM (target platform)
build-arm:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) .
	@echo "Built $(BUILD_DIR)/$(BINARY) for ARM"

# Build for AMD64 (development/testing)
build-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-amd64 .
	@echo "Built $(BUILD_DIR)/$(BINARY)-amd64 for AMD64"

# Build for native platform
build-native:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-native .
	@echo "Built $(BUILD_DIR)/$(BINARY)-native for native platform"

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

# Deploy to Deep Blue (requires deep-blue ssh alias)
deploy:
	@echo "Building for ARM..."
	@make build-arm
	@echo "Copying to Deep Blue..."
	@scp $(BUILD_DIR)/$(BINARY) deep-blue:/data/$(BINARY)-$$(date +%s)
	@echo "Deployed to /data/$(BINARY)-<timestamp>"
	@echo "To install: ssh deep-blue 'cp /data/$(BINARY)-* /usr/local/bin/$(BINARY)'"

# Quick deploy and test
deploy-test: deploy
	@echo "Testing basic status command..."
	@ssh deep-blue "/data/$(BINARY)-* status"
