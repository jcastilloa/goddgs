.DEFAULT_GOAL := help

GO ?= go
GO_FILES := $(shell find . -type f -name '*.go' -not -path './vendor/*')

.PHONY: help fmt fmt-check vet test test-race integration cover tidy verify

help:
	@echo "Targets:"
	@echo "  make fmt          Format Go files"
	@echo "  make fmt-check    Check Go formatting"
	@echo "  make vet          Run go vet"
	@echo "  make test         Run unit and contract tests"
	@echo "  make test-race    Run tests with race detector"
	@echo "  make integration  Run opt-in live-engine tests"
	@echo "  make cover        Produce coverage.out"
	@echo "  make tidy         Run go mod tidy"
	@echo "  make verify       Formatting, vet, unit tests, race tests"

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@test -z "$$(gofmt -l $(GO_FILES))" || (gofmt -l $(GO_FILES); exit 1)

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

integration:
	$(GO) test -tags=integration ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

tidy:
	$(GO) mod tidy

verify: fmt-check vet test test-race
