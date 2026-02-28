BINARY_NAME := pipeline-retry
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: build install clean test lint run-list run-retry

## build: Build the CLI binary
build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) .

## install: Install the binary to $GOPATH/bin
install:
	go install $(LDFLAGS) .

## clean: Remove build artifacts
clean:
	rm -rf bin/
	go clean

## test: Run all tests
test:
	go test ./... -v -race

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## tidy: Tidy go modules
tidy:
	go mod tidy

## run-list: Run the list command
run-list:
	go run . list -n $(NS)

## run-retry: Run the retry command
run-retry:
	go run . retry -n $(NS)

## help: Show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'
