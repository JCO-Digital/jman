.PHONY: build prepare install clean test format

# Go build flags
LDFLAGS := -s -w -X main.Version=$(shell git describe --tags --always --dirty || echo "dev")

build: clean prepare bin/jman

bin/jman:
	go build -ldflags="$(LDFLAGS)" -o bin/jman ./cmd/jman

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
