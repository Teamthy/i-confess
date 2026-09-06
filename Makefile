# I CONFESS — developer entry points.
#
# The Go module lives in server/, not at the repository root, which is the
# single most common way a new contributor's first command fails. Every target
# here handles that, so `make test` works from a fresh clone with no setup.
#
# Tests require a live PostgreSQL. TEST_DATABASE_URL below is the default for a
# local container; override it on the command line for anything else. There is
# deliberately no fallback that skips the tests — see internal/db/dbtest.

SHELL := /bin/bash
GO    ?= go

export TEST_DATABASE_URL ?= host=127.0.0.1 port=5432 user=iconfess password=iconfess dbname=postgres sslmode=disable

SERVER  := server
LINT    := golangci-lint

.PHONY: help build test race vet lint lint-fix fmt fmt-check tidy verify clean

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Compile every package
	cd $(SERVER) && $(GO) build ./...

test: ## Run the suite against PostgreSQL
	cd $(SERVER) && $(GO) test ./... -count=1

race: ## Run the suite with the race detector
	cd $(SERVER) && $(GO) test -race ./... -count=1

vet: ## Run go vet
	cd $(SERVER) && $(GO) vet ./...

lint: ## Run golangci-lint
	cd $(SERVER) && $(LINT) run ./...

lint-fix: ## Run golangci-lint and apply fixes
	cd $(SERVER) && $(LINT) run --fix ./...

fmt: ## Format the Go source
	cd $(SERVER) && gofmt -w .

fmt-check: ## Fail if anything is unformatted
	@cd $(SERVER) && files=$$(gofmt -l .); \
	  if [ -n "$$files" ]; then echo "not gofmt-formatted:"; echo "$$files"; exit 1; fi

tidy: ## Tidy go.mod and go.sum
	cd $(SERVER) && $(GO) mod tidy

# The full local gate. This is what CI runs, so passing it here means the
# pull request will not fail there.
verify: fmt-check build vet lint test ## Run everything CI runs
	@echo "verify: all checks passed"

clean: ## Remove build artifacts
	cd $(SERVER) && $(GO) clean ./...
