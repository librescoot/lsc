BINARY_NAME := lsc
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-w -s -X main.version=$(VERSION)"

.PHONY: build build-host build-arm dist clean lint test fmt deps deploy deploy-test deploy-lsd run run-lsd run-lsd-remote

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BUILD_DIR)/lsc .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BUILD_DIR)/lsd ./cmd/lsd

build-arm: build

build-host:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/lsc .
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/lsd ./cmd/lsd

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

# Run lsd locally against a Redis reachable from this machine.
run-lsd:
	go run ./cmd/lsd -addr 127.0.0.1:8090

# Run lsd on this machine against Deep Blue's Valkey over wireguard. Status,
# settings, the live stream and vehicle commands act on the scooter; the
# Services and Files pages and the Cloud service states are this machine's,
# since those go through the local systemctl and filesystem.
run-lsd-remote:
	go run ./cmd/lsd -addr 127.0.0.1:8090 -redis-addr 10.7.0.4:6379 -data /tmp

# Deploy to Deep Blue (requires deep-blue ssh alias)
deploy:
	@echo "Building for ARM..."
	@make build-arm
	@echo "Copying to Deep Blue..."
	@scp $(BUILD_DIR)/lsc deep-blue:/data/lsc-$$(date +%s)
	@echo "Deployed to /data/lsc-<timestamp>"
	@echo "To install: ssh deep-blue 'cp /data/lsc-* /usr/local/bin/lsc'"

# Quick deploy and test
deploy-test: deploy
	@echo "Testing basic status command..."
	@ssh deep-blue "/data/lsc-* status"

# Deploy lsd to Deep Blue's /data partition and run it from there, bound to
# all interfaces so it is reachable over wireguard (10.7.0.4) as well as usb0.
# pkill -x matches the process name only: a -f pattern would also match the
# remote shell running this very command and kill the deploy.
deploy-lsd:
	@echo "Building lsd for ARM..."
	@make build-arm
	@echo "Copying to Deep Blue..."
	@ssh deep-blue 'mkdir -p /data/lsd'
	@scp $(BUILD_DIR)/lsd deep-blue:/data/lsd/lsd-new
	@ssh deep-blue 'pkill -x lsd; sleep 1; mv /data/lsd/lsd-new /data/lsd/lsd && chmod +x /data/lsd/lsd; nohup /data/lsd/lsd -addr :8090 >>/data/lsd/lsd.log 2>&1 & sleep 1; tail -3 /data/lsd/lsd.log; echo; echo "lsd is up on http://192.168.7.1:8090 and http://10.7.0.4:8090"'
