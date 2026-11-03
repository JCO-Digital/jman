.PHONY: build dev dev-api prepare install clean test format completions dev-ui build-ui install-ui clean-ui \
	local-dev local-dev-reset local-dev-agent local-dev-agent-down local-dev-agent-infect local-dev-agent-clean

# Go build flags
LDFLAGS := -s -w -X github.com/JCO-Digital/jman/internal/config.AppVersion=$(shell git describe --tags --always --dirty || echo "dev")

# air from PATH, else where `go install` puts it (GOBIN, or GOPATH/bin)
AIR ?= $(or $(shell command -v air 2>/dev/null),$(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)/air)

build: clean prepare bin/jman bin/jman-api bin/jman-agent completions

bin/jman:
	go build -ldflags="$(LDFLAGS)" -o bin/jman ./cmd/jman

bin/jman.exe:
	GOOS=windows go build -ldflags="$(LDFLAGS)" -o bin/jman.exe ./cmd/jman

bin/jman-api:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-api ./cmd/jman-api

bin/jman-agent:
	go build -ldflags="$(LDFLAGS)" -o bin/jman-agent ./cmd/jman-agent

dev: clean prepare
	go build -o bin/jman ./cmd/jman
	go build -o bin/jman-api ./cmd/jman-api
	go build -o bin/jman-agent ./cmd/jman-agent

dev-api:
	@test -x "$(AIR)" || { echo "air not found; install it with: go install github.com/air-verse/air@latest"; exit 1; }
	$(AIR)

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

# Whole stack locally (jman-api with hot reload, web UI, logged-in CLI),
# isolated in .local-dev/. See README.md, "Local development".
LOCAL_DEV := scripts/local-dev/local-dev.sh

local-dev:
	@$(LOCAL_DEV) up

local-dev-reset:
	@$(LOCAL_DEV) reset

# Docker containers acting as managed servers, each running jman-agent
# against the local jman-api (needs make local-dev running).
local-dev-agent:
	@$(LOCAL_DEV) agent-up

local-dev-agent-down:
	@$(LOCAL_DEV) agent-down

# Plant or remove inert files that trigger the agent's scan rules.
local-dev-agent-infect:
	@$(LOCAL_DEV) infect

local-dev-agent-clean:
	@$(LOCAL_DEV) clean
