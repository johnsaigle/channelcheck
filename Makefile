.PHONY: build test lint

build:
	go build -o bin/channelcheck ./cmd/channelcheck

test:
	go test -race ./...

lint:
	golangci-lint run
