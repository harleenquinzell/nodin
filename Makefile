ROOT := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
BIN  := $(ROOT)nodin

.PHONY: all build test test-integration lint clean install help

all: build

build:
	cd $(ROOT) && go build -o $(BIN) ./cmd/nodin

test:
	cd $(ROOT) && go test ./...

test-integration:
	@cd $(ROOT) && bash -c 'set -a; [ -f .env ] && source .env; set +a; \
		go test -tags integration -timeout 60s -v ./internal/sync/ ./internal/notion/'

lint:
	cd $(ROOT) && go vet ./...

clean:
	rm -f $(BIN)

install:
	cd $(ROOT) && go install ./cmd/nodin

help:
	@echo "build            build the nodin binary"
	@echo "test             run unit tests (no network required)"
	@echo "test-integration run integration tests (sources .env for credentials)"
	@echo "lint             run go vet"
	@echo "install          go install ./cmd/nodin"
	@echo "clean            remove the built binary"
