.PHONY: build test lint clean run docker-build docker-build-multiarch

# Binary name
BINARY=bridge
BUILD_DIR=./cmd/bridge

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build flags
LDFLAGS=-ldflags="-s -w"

# Default target
all: lint test build

# Build the binary
build:
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY) $(BUILD_DIR)

# Run tests with coverage
test:
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...

# Run tests with coverage report
test-coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -f coverage.out coverage.html

# Run the server (requires DASHBOARD_URL)
run: build
	./$(BINARY)

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Docker build for local architecture
docker-build:
	docker build -t ghcr.io/cbenitezpy-ueno/retrodash-server:latest .

# Docker multi-arch build (AMD64 + ARM64)
docker-build-multiarch:
	docker buildx create --name multiarch --driver docker-container --use 2>/dev/null || true
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t ghcr.io/cbenitezpy-ueno/retrodash-server:latest \
		--push .

# Docker run locally
docker-run:
	docker run --rm \
		--shm-size=256m \
		-p 8080:8080 \
		-e DASHBOARD_URL=$(DASHBOARD_URL) \
		ghcr.io/cbenitezpy-ueno/retrodash-server:latest

# Help
help:
	@echo "Available targets:"
	@echo "  build              - Build the binary"
	@echo "  test               - Run tests with coverage"
	@echo "  test-coverage      - Run tests and generate HTML coverage report"
	@echo "  lint               - Run golangci-lint"
	@echo "  clean              - Remove build artifacts"
	@echo "  run                - Build and run the server"
	@echo "  deps               - Download and tidy dependencies"
	@echo "  docker-build       - Build Docker image for local arch"
	@echo "  docker-build-multiarch - Build Docker image for AMD64 and ARM64"
	@echo "  docker-run         - Run Docker container (requires DASHBOARD_URL)"
