SHELL := /bin/sh
.DEFAULT_GOAL := help

GO ?= go
BINARY ?= git-update
BUILD_DIR ?= bin
TOOLS_DIR ?= .bin
BIN_PATH := $(BUILD_DIR)/$(BINARY)

GOLANGCI_LINT_VERSION ?= v2.13.2
GOLANGCI_LINT ?= $(TOOLS_DIR)/golangci-lint

GO_BIN := $(shell $(GO) env GOBIN 2>/dev/null)
ifeq ($(strip $(GO_BIN)),)
INSTALL_DIR ?= $(shell $(GO) env GOPATH 2>/dev/null)/bin
else
INSTALL_DIR ?= $(GO_BIN)
endif
INSTALL_PATH := $(INSTALL_DIR)/$(BINARY)

.PHONY: help doctor deps tools fmt fmt-check vet test coverage lint build install uninstall run ci clean distclean

help: ## Show available commands
	@printf "git-update development commands\n\n"
	@awk 'BEGIN {FS = ":.*## "; printf "Usage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Check required local tools and versions
	@command -v $(GO) >/dev/null 2>&1 || { echo "ERROR: Go is not installed or not in PATH"; exit 1; }
	@command -v git >/dev/null 2>&1 || { echo "ERROR: Git is not installed or not in PATH"; exit 1; }
	@echo "Go:  $$($(GO) version)"
	@echo "Git: $$(git --version)"
	@echo "Install directory: $(INSTALL_DIR)"

deps: doctor ## Download and normalize Go dependencies
	$(GO) mod tidy

tools: doctor ## Install development tools locally under .bin
	@mkdir -p $(TOOLS_DIR)
	@if command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1 && "$(GOLANGCI_LINT)" version 2>/dev/null | grep -q "$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))"; then \
		exit 0; \
	fi; \
	if [ "$(GOLANGCI_LINT)" != "$(TOOLS_DIR)/golangci-lint" ]; then \
		echo "ERROR: $(GOLANGCI_LINT) is missing or is not version $(GOLANGCI_LINT_VERSION)"; \
		exit 1; \
	fi; \
	echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
	GOBIN="$(abspath $(TOOLS_DIR))" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

fmt: ## Format Go source files
	gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')

fmt-check: ## Verify Go source formatting
	@test -z "$$(gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'))" || { \
		echo "ERROR: Go files need formatting. Run: make fmt"; \
		gofmt -l $$(find . -type f -name '*.go' -not -path './vendor/*'); \
		exit 1; \
	}

vet: deps ## Run go vet
	$(GO) vet ./...

test: deps ## Run tests with race detector and coverage
	$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...

coverage: test ## Print test coverage details
	$(GO) tool cover -func=coverage.out

lint: deps tools ## Run golangci-lint
	$(GOLANGCI_LINT) run --timeout=5m ./...

build: deps ## Build the git-update binary
	@mkdir -p $(BUILD_DIR)
	$(GO) build -trimpath -o $(BIN_PATH) .
	@echo "Built: $(BIN_PATH)"

install: build ## Install git-update into GOBIN/GOPATH/bin
	@mkdir -p "$(INSTALL_DIR)"
	install -m 0755 "$(BIN_PATH)" "$(INSTALL_PATH)"
	@echo "Installed: $(INSTALL_PATH)"
	@echo "You can now run: git update <folder>"

uninstall: ## Remove the installed git-update binary
	rm -f "$(INSTALL_PATH)"
	@echo "Removed: $(INSTALL_PATH)"

run: deps ## Run from source (pass ARGS='...' for arguments)
	$(GO) run . $(ARGS)

ci: fmt-check vet test lint build ## Run all validation checks

clean: ## Remove build and coverage output
	rm -rf $(BUILD_DIR) coverage.out

distclean: clean ## Remove build output and downloaded development tools
	rm -rf $(TOOLS_DIR)
