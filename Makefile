.PHONY: build test lint fmt vet run clean

BINARY := sitedex
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/Xara-AI/sitedex/internal/version.Version=$(VERSION) \
	-X github.com/Xara-AI/sitedex/internal/version.Commit=$(COMMIT) \
	-X github.com/Xara-AI/sitedex/internal/version.Date=$(DATE)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/sitedex

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -l -w .

vet:
	go vet ./...

run: build
	./bin/$(BINARY) $(ARGS)

clean:
	rm -rf bin/
