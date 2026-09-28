.PHONY: run build vet test seed-reset bible-registry

GO ?= go

run:
	$(GO) run ./cmd/server

build:
	$(GO) build -o bin/iconfess ./cmd/server

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

seed-reset:
	rm -f data/iconfess.db data/iconfess.db-wal data/iconfess.db-shm
	rm -rf data/media
	@echo "Database + media reset. Next 'make run' will re-seed."

# Regenerate the reviewed translation registry the web app renders as
# provenance cards on the translations page (see internal/bible/registry.go).
bible-registry:
	$(GO) run ./cmd/bible-registry ../web/lib/versions.json
