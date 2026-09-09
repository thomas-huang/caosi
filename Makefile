.PHONY: build test cover dist live

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X github.com/thomas-huang/caosi/internal/app.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o caosi ./cmd/caosi

test:
	go test ./...

live:
	go test -tags live -timeout 30m -v ./internal/livetest

cover:
	go test -coverprofile=cover.out ./...
	go tool cover -func=cover.out

dist:
	bash scripts/build_release.sh "$(VERSION)" dist
