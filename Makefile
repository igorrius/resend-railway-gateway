BINARY := resend-railway-gateway
PKG := ./...

.PHONY: all build test bench lint run clean

all: build

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/$(BINARY) ./cmd/gateway

test:
	go test -race -coverprofile=coverage.out -covermode=atomic $(PKG)

bench:
	go test -bench=. -benchmem $(PKG)

lint:
	golangci-lint run || true

run:
	go run ./cmd/gateway

clean:
	rm -rf bin coverage.out
