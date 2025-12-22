BINARY_NAME := lsc
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-w -s -X main.version=$(VERSION)"

.PHONY: build build-host build-arm dist clean lint test fmt deps deploy deploy-test run

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

build-arm: build

build-host:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

dist: build

clean:
	rm -rf $(BUILD_DIR)

lint:
	golangci-lint run

test:
	go test -v ./...

fmt:
	go fmt ./...

deps:
	go mod download && go mod tidy

run:
	go run . status

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
