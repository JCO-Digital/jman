.PHONY: build dev prepare install clean test format

# Go build flags
LDFLAGS := -s -w -X github.com/JCO-Digital/jman/internal/config.AppVersion=$(shell git describe --tags --always --dirty || echo "dev")

build: clean prepare bin/jman bin/jman-api bin/jman-monitor

bin/jman:
	go build -ldflags="$(LDFLAGS)" -o bin/jman ./cmd/jman

bin/jman.exe:
	GOOS=windows go build -ldflags="$(LDFLAGS)" -o bin/jman.exe ./cmd/jman

bin/jman-api:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-api ./cmd/jman-api

bin/jman-monitor:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-monitor ./cmd/jman-monitor

dev: clean prepare
	go build -o bin/jman ./cmd/jman
	go build -o bin/jman-api ./cmd/jman-api
	go build -o bin/jman-monitor ./cmd/jman-monitor

prepare:
	go mod download
	go mod tidy

install: build
	cp bin/jman ~/.local/bin/

clean:
	rm -rf bin
	go clean

test:
	go test ./...

format:
	go fmt ./...
