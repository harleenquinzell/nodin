ROOT    := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
BIN     := $(ROOT)nodin
VERSION := $(shell git -C $(ROOT) describe --tags --always --dirty 2>/dev/null || echo "dev")

export PATH := /usr/local/go/bin:$(HOME)/go/bin:$(PATH)

.PHONY: all build test test-integration test-e2e lint clean install hooks help

all: build

build:
	cd $(ROOT) && go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/nodin

test:
	cd $(ROOT) && go test ./...

test-integration:
	@cd $(ROOT) && bash -c 'set -a; [ -f .env ] && source .env; set +a; \
		go test -tags integration -timeout 300s -v ./internal/sync/ ./internal/notion/'

test-e2e:
	@cd $(ROOT) && bash -c 'set -a; [ -f .env ] && source .env; set +a; \
		go test -tags e2e -timeout 600s -v ./e2e/'

lint:
	@cd $(ROOT) && unformatted=$$(gofmt -l .); \
		if [ -n "$$unformatted" ]; then \
			echo "Files not formatted (run gofmt -w .):"; \
			echo "$$unformatted"; \
			exit 1; \
		fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		cd $(ROOT) && golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, skipping (install from https://golangci-lint.run/welcome/install/)"; \
	fi

clean:
	rm -f $(BIN)

hooks:
	git config core.hooksPath hooks

install:
	cd $(ROOT) && go install -ldflags "-X main.version=$(VERSION)" ./cmd/nodin

help:
	@echo "build            build the nodin binary"
	@echo "test             run unit tests (no network required)"
	@echo "test-integration run integration tests (sources .env for credentials)"
	@echo "test-e2e         run end-to-end tests (sources .env for credentials)"
	@echo "lint             run gofmt check + golangci-lint"
	@echo "install          go install ./cmd/nodin"
	@echo "hooks            configure git to use the hooks/ directory"
	@echo "clean            remove the built binary"
