.PHONY: help build test test-race cover lint fmt vet tidy install clean check ci release-dry

BIN_DIR ?= bin
BIN     := $(BIN_DIR)/flotilla
PKG     := ./...

GO       ?= go
GOFMT    ?= gofmt
GOLINT   ?= golangci-lint

VERSION  := $(shell cat VERSION)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

help: ## Show this help
	@awk 'BEGIN{FS=":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the flotilla binary into bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/flotilla

test: ## Run unit tests
	$(GO) test $(PKG)

test-race: ## Run tests with the race detector
	$(GO) test -race $(PKG)

cover: ## Run tests with HTML coverage report
	$(GO) test -coverprofile=coverage.out $(PKG)
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in your browser."

lint: ## Run golangci-lint (must be installed: https://golangci-lint.run)
	$(GOLINT) run

fmt: ## Format Go sources in place
	$(GOFMT) -w -s .

vet: ## Run go vet
	$(GO) vet $(PKG)

tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

install: ## Install flotilla into $GOPATH/bin
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/flotilla

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist coverage.out coverage.html

check: fmt vet test ## Local pre-commit checks (formats sources)
	@echo "Local checks passed."

ci: vet test-race ## CI suite (no formatter writes)
	@echo "CI checks passed."

release-dry: ## Local snapshot release via GoReleaser, no publish
	goreleaser release --snapshot --clean
