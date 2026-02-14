.PHONY: build test lint clean docker-build help

# Build variables
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d %H:%M:%S')
LDFLAGS = -ldflags "-X github.com/Ajpantuso/etcd-snapshot-driver/pkg/util.Version=$(VERSION) -X 'github.com/Ajpantuso/etcd-snapshot-driver/pkg/util.BuildTime=$(BUILD_TIME)' -X github.com/Ajpantuso/etcd-snapshot-driver/pkg/util.GitCommit=$(COMMIT)"

# Build output directory
BIN_DIR := bin
BINARY_NAME := etcd-snapshot-driver

# Default Go flags
GO := go
GOFLAGS := -v
GOMOD := on

help:
	@echo "etcd-snapshot-driver build targets:"
	@echo "  build          - Build the driver binary"
	@echo "  test           - Run unit tests"
	@echo "  lint           - Run linter (requires golangci-lint)"
	@echo "  docker-build   - Build Docker image"
	@echo "  clean          - Remove build artifacts"
	@echo ""
	@echo "Environment variables:"
	@echo "  VERSION        - Version string (default: dev)"
	@echo "  COMMIT         - Git commit hash (auto-detected)"
	@echo "  BUILD_TIME     - Build timestamp (auto-generated)"

build: ## Build the driver binary
	$(GO) mod download
	$(GO) mod tidy
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) \
		-o $(BIN_DIR)/$(BINARY_NAME) ./cmd/etcd-snapshot-driver

test: ## Run unit tests
	$(GO) test -v -race -cover ./...

test-coverage: ## Run tests with coverage report
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	$(GO) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

integration-test: ## Run integration tests (requires Kind cluster)
	$(GO) test -v -tags=integration ./test/integration/...

lint: ## Run linter
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

fmt: ## Format code
	$(GO) fmt ./...
	goimports -w .

docker-build: build ## Build Docker image
	docker build -t etcd-snapshot-driver:latest -f build/Dockerfile .

docker-build-with-tag: build ## Build Docker image with version tag
	docker build -t etcd-snapshot-driver:$(VERSION) -f build/Dockerfile .

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	rm -f coverage.txt coverage.html
	$(GO) clean -testcache

.PHONY: all
all: clean lint test build
