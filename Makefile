BINARY_NAME=version-service
GOOS=linux
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build build-host build-amd64 build-arm dist clean lint test fmt deps

build:
	CGO_ENABLED=0 go build -ldflags "-w -s -X main.version=$(VERSION)" -o $(BINARY_NAME) ./cmd/version-service

build-host:
	CGO_ENABLED=0 go build -ldflags "-w -s -X main.version=$(VERSION)" -o $(BINARY_NAME) ./cmd/version-service

build-amd64:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=amd64 go build -ldflags "-w -s -X main.version=$(VERSION)" -o $(BINARY_NAME)-amd64 ./cmd/version-service

build-arm:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=arm GOARM=7 go build -ldflags "-w -s -X main.version=$(VERSION)" -o $(BINARY_NAME)-arm ./cmd/version-service

dist:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=arm GOARM=7 go build -ldflags "-w -s -X main.version=$(VERSION)" -o $(BINARY_NAME)-arm-dist ./cmd/version-service

lint:
	golangci-lint run

test:
	go test -v ./...

fmt:
	go fmt ./...

deps:
	go mod download && go mod tidy

clean:
	rm -f $(BINARY_NAME)
