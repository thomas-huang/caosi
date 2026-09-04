.PHONY: build test cover

build:
	go build -o caosi ./cmd/caosi

test:
	go test ./...

cover:
	go test -coverprofile=cover.out ./...
	go tool cover -func=cover.out
