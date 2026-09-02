.PHONY: build test

build:
	go build -o caosi ./cmd/caosi

test:
	go test ./...
