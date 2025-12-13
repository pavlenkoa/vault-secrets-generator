.PHONY: build test lint clean docker docker-push install help

# Variables
BINARY_NAME := vsg
MODULE := github.com/pavlenkoa/vault-secrets-generator
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X $(MODULE)/internal/command.Version=$(VERSION) \
	-X $(MODULE)/internal/command.Commit=$(COMMIT) \
	-X $(MODULE)/internal/command.BuildDate=$(BUILD_DATE)

# Docker
DOCKER_REGISTRY ?= ghcr.io
DOCKER_IMAGE ?= $(DOCKER_REGISTRY)/pavlenkoa/vault-secrets-generator
DOCKER_TAG ?= $(VERSION)

# Go
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) .

## build-all: Build for all platforms
build-all:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o dist/$(BINARY_NAME)-windows-amd64.exe .

## install: Install the binary to $GOPATH/bin
install:
	CGO_ENABLED=0 go install -ldflags="$(LDFLAGS)" .

## test: Run tests
test:
	go test -v -race -cover ./...

## test-short: Run tests without verbose output
test-short:
	go test -race ./...

## test-coverage: Run tests with coverage report
test-coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

## lint: Run linters
lint:
	golangci-lint run

## lint-fix: Run linters and fix issues
lint-fix:
	golangci-lint run --fix

## fmt: Format code
fmt:
	go fmt ./...
	gofmt -s -w .

## vet: Run go vet
vet:
	go vet ./...

## tidy: Tidy go modules
tidy:
	go mod tidy

## clean: Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
	rm -f coverage.out coverage.html

## docker: Build Docker image
docker:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest

## docker-push: Push Docker image
docker-push: docker
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest

## release-snapshot: Build a snapshot release with goreleaser
release-snapshot:
	goreleaser release --snapshot --clean

## release: Build a release with goreleaser (requires GITHUB_TOKEN)
release:
	goreleaser release --clean
