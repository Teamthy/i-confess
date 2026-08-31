.PHONY: run build vet test seed-reset

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
