.PHONY: build test lint install clean help

BIN := bin/xhist

## build: Build the xhist binary
build:
	go build -o $(BIN) ./cmd/xhist

## test: Run all tests with race detector
test:
	go test -v -race -count=1 ./...

## lint: Run formatting check and vet
lint:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:" && gofmt -l . && exit 1)
	go vet ./...

## install: Install xhist to GOPATH/bin
install:
	go install ./cmd/xhist

## clean: Remove build artifacts
clean:
	rm -rf bin/

## help: Show available targets
help:
	@echo "Usage:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  make /' | sed 's/: /\t/'
