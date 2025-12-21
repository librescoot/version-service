BINARY_NAME := version-service
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-w -s -X main.version=$(VERSION)"

.PHONY: build build-host build-arm dist clean lint test fmt deps

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/version-service

build-arm: build

build-host:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/version-service

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
