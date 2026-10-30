# Makefile for awesomegen.
#
# Every target only needs the Go toolchain. Tools that are not part of the
# standard distribution are executed through `go run` with a pinned version,
# so nothing has to be installed on the machine.
#
#   make help      list the available targets
#   make build     build the CLI into bin/
#   make test      run the unit tests
#   make lint      run golangci-lint

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

GO ?= go

BINARY      := awesomegen
CMD         := ./cmd/awesomegen
BIN_DIR     := bin
DIST_DIR    := dist
COVERAGE    := coverage.out
SCHEMA_FILE := config-schema.json

# The version is stamped into the binary. Override it with `make build VERSION=v1.2.3`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# The commit and the build date are stamped into the binary as well, so that
# "awesomegen version" reports where the running binary came from.
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Pinned tool versions. Bump them together with the corresponding configuration.
GOLANGCI_LINT_VERSION ?= v2.15.0
GORELEASER_VERSION    ?= v2.19.0
GOVULNCHECK_VERSION   ?= v1.9.0

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the CLI into bin/
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" \
		-o $(BIN_DIR)/$(BINARY) $(CMD)

.PHONY: install
install: ## Install the CLI into the Go bin directory
	$(GO) install $(CMD)

.PHONY: test
test: ## Run the unit tests
	$(GO) test ./...

.PHONY: test-race
test-race: ## Run the unit tests with the race detector (needs cgo and a C compiler)
	CGO_ENABLED=1 $(GO) test -race ./...

.PHONY: cover
cover: ## Run the tests and write a coverage profile
	$(GO) test -coverprofile=$(COVERAGE) ./...
	$(GO) tool cover -func=$(COVERAGE)

.PHONY: lint
lint: ## Run golangci-lint
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

.PHONY: fmt
fmt: ## Format the source code
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail when the source code is not gofmt'ed
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "the following files are not gofmt'ed:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: schema
schema: ## Regenerate config-schema.json from the Go types
	$(GO) generate ./...

.PHONY: schema-check
schema-check: ## Fail when config-schema.json is out of date
	@$(GO) generate ./...
	@if ! git diff --quiet -- $(SCHEMA_FILE); then \
		echo "$(SCHEMA_FILE) is out of date: run 'make schema' and commit the result" >&2; \
		git --no-pager diff -- $(SCHEMA_FILE); \
		exit 1; \
	fi

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

.PHONY: vuln
vuln: ## Run govulncheck against the dependency graph
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

.PHONY: release
release: ## Prepare the release locally (the workflow signs and publishes it)
	# The SBOM, the signatures and the container image are skipped: they
	# need syft, cosign with an OIDC token and a Docker daemon, none of
	# which a local machine has. The release workflow does all three when
	# the tag is pushed, and it reuses the draft that this target creates.
	$(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION) release --clean \
		--skip=sbom --skip=sign --skip=docker

.PHONY: snapshot
snapshot: ## Build a snapshot release with GoReleaser (nothing is published)
	# The SBOM, the signatures and the container image are skipped: they
	# need syft, cosign with an OIDC token and a Docker daemon, none of
	# which a local rehearsal has. The artefacts of the snapshot are not
	# published, so none of the three would be of any use.
	$(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION) release --snapshot --clean \
		--skip=sbom --skip=sign --skip=docker

.PHONY: clean
clean: ## Remove the build output
	rm -rf $(BIN_DIR) $(DIST_DIR) $(COVERAGE)
