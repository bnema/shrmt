.PHONY: build install test mock fmt lint

BINARY ?= shrmt
CMD_PATH ?= ./cmd/shrmt
BUILD_DIR ?= bin
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD_PATH)

install: build
	mkdir -p $(BINDIR)
	install -m 0755 $(BUILD_DIR)/$(BINARY) $(BINDIR)/$(BINARY)

test:
	go test ./...

mock:
	mockery

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...
