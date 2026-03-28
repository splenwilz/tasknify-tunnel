.PHONY: build build-server build-client test lint vet run-server run-client clean

VERSION ?= dev

build: build-server build-client

build-server:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/devtunnel-server ./cmd/devtunnel-server

build-client:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/devtunnel ./cmd/devtunnel

test:
	go test ./... -v -race

lint: vet
	@which golangci-lint > /dev/null 2>&1 || echo "golangci-lint not installed, skipping"
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run || true

vet:
	go vet ./...

run-server:
	go run -ldflags="-X main.version=$(VERSION)" ./cmd/devtunnel-server

run-client:
	go run -ldflags="-X main.version=$(VERSION)" ./cmd/devtunnel http 3000 testapp --server ws://localhost:8001

clean:
	rm -rf bin/
