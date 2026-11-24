BIN     := urlchecker
PKG     := github.com/extimsu/urlchecker
VERSION ?= $(shell git describe --tags --match 'v*' --dirty 2>/dev/null | sed 's/^v//' || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X $(PKG)/version.Version=$(VERSION) -X $(PKG)/version.GitCommit=$(COMMIT) -X $(PKG)/version.BuildDate=$(DATE)
IMAGE   ?= ghcr.io/extimsu/urlchecker

.PHONY: build test lint vuln image snapshot clean help
.DEFAULT_GOAL := help

build: ## Build bin/urlchecker
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BIN) .

test: ## Run the tests with the race detector
	go test -race -cover ./...

lint: ## Run golangci-lint
	golangci-lint run

vuln: ## Check dependencies and the toolchain for known vulnerabilities
	govulncheck ./...

image: ## Build a local image, IMAGE:VERSION
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t $(IMAGE):$(VERSION) .

snapshot: ## Build every release artifact locally, without publishing
	goreleaser release --snapshot --clean

clean: ## Remove build output
	rm -rf bin dist

help: ## Show this help
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'
