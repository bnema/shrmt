.PHONY: build test mock fmt lint

build:
	go build ./...

test:
	go test ./...

mock:
	mockery

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...
