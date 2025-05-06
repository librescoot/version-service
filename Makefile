BINARY_NAME=version-service
GOOS=linux

.PHONY: build build-arm dist clean

build:
	CGO_ENABLED=0 go build -o $(BINARY_NAME) ./cmd/version-service

build-arm:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=arm GOARM=7 go build -o $(BINARY_NAME)-arm ./cmd/version-service

dist:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o $(BINARY_NAME)-arm-dist ./cmd/version-service

clean:
	rm -f $(BINARY_NAME)
