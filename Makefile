.PHONY: build build-client build-server clean format help naming-check dev test test-e2e test-e2e-all test-e2e-file-edit test-e2e-m5

ENV_FILE ?= .env.local
VERSION ?=
COMMIT ?=
BUILD_DATE ?=

export VERSION COMMIT BUILD_DATE

build: build-client build-server ## Build client and server release bundles

build-client: ## Build the aist client release bundle
	@./scripts/build-client.sh

build-server: ## Build the AIST server release bundle
	@./scripts/build-server.sh

dev: ## Run the server in dev mode (loads $(ENV_FILE) if present)
	@set -a; [ -f "$(ENV_FILE)" ] && . "$(ENV_FILE)"; set +a; \
	go run ./cmd/server serve --dev

test: ## Run all Go tests
	@go test ./...

test-e2e: test-e2e-file-edit test-e2e-m5 ## Run automated end-to-end tests

test-e2e-all: ## Run all Go tests and every automated E2E suite
	@./scripts/all-e2e.sh

test-e2e-file-edit: ## Run file_edit end-to-end integration tests
	@./scripts/file-edit-e2e.sh

test-e2e-m5: ## Run multi-target, SSH and confirmation E2E tests
	@./scripts/m5-e2e.sh

format: ## Format all Go source files
	@gofmt -w -s .
	@echo "✓ Formatted"

naming-check: ## Check for deprecated identifiers
	@./scripts/naming-check.sh

clean: ## Remove built binaries and the release directory
	@rm -f aist
	@find . -maxdepth 1 -type f -name 'aist-*' -delete
	@rm -rf dist
	@echo "✓ Cleaned"

help: ## Display this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'
