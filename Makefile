# Container runtime (podman by default, override with CONTAINER_RUNTIME=docker)
CONTAINER_RUNTIME ?= podman

.PHONY: help
help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: test
test: ## Run unit tests (excludes integration and e2e tests)
	@echo "Running unit tests..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: test-integration
test-integration: ## Run integration tests (requires live OpenShift cluster with KubeVirt)
	@echo "Running integration tests..."
	go test -v -race -tags=integration ./...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (requires live OpenShift cluster with KubeVirt)
	@echo "Running e2e tests..."
	go test -v -race -tags=e2e ./tests/e2e/...

.PHONY: test-all
test-all: test test-integration test-e2e ## Run all tests (unit, integration, and e2e)

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

.PHONY: fmt
fmt: ## Format Go code
	@echo "Formatting code..."
	go fmt ./...

.PHONY: build
build: ## Build the virtwork binary
	@echo "Building virtwork..."
	go build -o bin/virtwork ./cmd/virtwork

.PHONY: clean
clean: ## Remove build artifacts and test coverage files
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out

.PHONY: ci
ci: vet test build ## Run CI validation locally (vet, test, build)

.PHONY: install-tools
install-tools: ## Install development tools (golangci-lint)
	@echo "Installing development tools..."
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: container-build
container-build: ## Build container image locally (uses podman by default, override with CONTAINER_RUNTIME=docker)
	@echo "Building container image with $(CONTAINER_RUNTIME)..."
	$(CONTAINER_RUNTIME) build -t virtwork:local -f Dockerfile .

.PHONY: verify
verify: fmt vet lint test ## Run all verification checks (fmt, vet, lint, test)

.DEFAULT_GOAL := help
