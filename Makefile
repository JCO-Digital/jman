.PHONY: build build-pkg dev dev-api prepare install clean test format completions dev-ui build-ui install-ui clean-ui

# Go build flags
LDFLAGS := -s -w -X github.com/JCO-Digital/jman/internal/config.AppVersion=$(shell git describe --tags --always --dirty || echo "dev")

build: clean prepare bin/jman bin/jman-api bin/jman-monitor bin/jman-agent completions

# Build without the built-in update command (for use with external package managers)
build-pkg: clean prepare
	go build -tags noupdate -ldflags="$(LDFLAGS)" -o bin/jman ./cmd/jman

bin/jman:
	go build -ldflags="$(LDFLAGS)" -o bin/jman ./cmd/jman

bin/jman.exe:
	GOOS=windows go build -ldflags="$(LDFLAGS)" -o bin/jman.exe ./cmd/jman

bin/jman-api:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-api ./cmd/jman-api

bin/jman-monitor:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-monitor ./cmd/jman-monitor

bin/jman-agent:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-agent ./cmd/jman-agent

dev: clean prepare
	go build -o bin/jman ./cmd/jman
	go build -o bin/jman-api ./cmd/jman-api
	go build -o bin/jman-monitor ./cmd/jman-monitor
	go build -o bin/jman-agent ./cmd/jman-agent

dev-api:
	air

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

completions: bin/jman
	@mkdir -p completions
	JMAN_TOKENSPINUP=placeholder bin/jman completion bash > completions/jman.bash
	JMAN_TOKENSPINUP=placeholder bin/jman completion zsh > completions/jman.zsh
	JMAN_TOKENSPINUP=placeholder bin/jman completion fish > completions/jman.fish

dev-ui:
	$(MAKE) -C web dev

build-ui:
	$(MAKE) -C web build

install-ui:
	$(MAKE) -C web install

clean-ui:
	$(MAKE) -C web clean
