# Plan: Rewriting `jman` in Go

This document outlines the strategy for rewriting the `jman` CLI tool from TypeScript (Bun) to Go. Go is an excellent choice for CLI utilities due to its compilation to single statically linked binaries, fantastic concurrency model, and robust standard library.

## 1. Dependency Mapping

The current TypeScript implementation relies on several npm packages. Here are their Go equivalents:

| Feature / TS Package                        | Go Equivalent                                                          | Justification                                                                                                               |
| :------------------------------------------ | :--------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------- |
| **CLI Framework** (Custom `cmdParse`)       | `github.com/spf13/cobra`                                               | The industry standard for Go CLIs. Provides routing, subcommands, and flags out of the box.                                 |
| **TOML Parsing** (`smol-toml`)              | `github.com/pelletier/go-toml/v2`                                      | Fast, standard-compliant TOML parsing.                                                                                      |
| **Schema Validation** (`zod`)               | Go Structs + `encoding/json`                                           | Go's strong typing mostly removes the need for `zod`. Complex validations can use `github.com/go-playground/validator/v10`. |
| **XDG Directories** (`xdg-basedir`)         | `github.com/adrg/xdg`                                                  | Cross-platform XDG Base Directory specification support.                                                                    |
| **Interactive Prompts** (`@topcli/prompts`) | `github.com/manifoldco/promptui` or `github.com/AlecAivazis/survey/v2` | Rich, interactive terminal prompts for searching/selecting.                                                                 |
| **Slack API** (`@slack/web-api`)            | `github.com/slack-go/slack`                                            | Full-featured Slack API client for Go.                                                                                      |
| **Subprocesses** (`child_process.exec`)     | `os/exec` (Standard Library)                                           | Native OS process execution for running `wp-cli`.                                                                           |

## 2. Target Project Structure

We will adopt a standard Go project layout:

```text
jman/
├── cmd/
│   └── jman/
│       └── main.go         # Entry point, initializes Cobra root command
├── internal/
│   ├── config/             # Config management, xdg paths, TOML parsing
│   ├── cache/              # JSON file caching logic (read/write, 12h TTL)
│   ├── api/
│   │   ├── spinupwp/       # SpinupWP REST client
│   │   ├── mainwp/         # MainWP REST client
│   │   └── wpvuln/         # WPVulnerability client
│   ├── wpcli/              # SSH command execution wrapper (os/exec)
│   ├── commands/           # Cobra CLI subcommands implementations
│   ├── models/             # Go structs representing Site, Server, Plugin, etc.
│   └── slack/              # Slack notification client
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 3. Migration Steps

### Phase 1: Foundation & Configuration

1. **Initialize Module:** `go mod init github.com/JCO-Digital/jman`
2. **Define Models:** Translate Zod schemas (`Site`, `Server`, `Plugin`, `jConfig`) into Go structs with JSON and TOML tags.
3. **Configuration & Paths:** Implement `internal/config` using `adrg/xdg` to resolve `~/.config/jman/config.toml`, `~/.cache/jman`, and `~/.local/share/jman`.
4. **CLI Setup:** Initialize Cobra in `cmd/jman/main.go` and scaffold empty commands.

### Phase 2: API Integration & Caching

1. **API Clients:** Build the SpinupWP client (`internal/api/spinupwp`). Ensure pagination logic (the `next` cursor) is implemented seamlessly.
2. **Caching System:** Implement `internal/cache` to read/write JSON arrays. Replicate the 12-hour expiration check using `os.Stat(path).ModTime()`.
3. **Commands:** Implement `jman fetch` and `jman list` to populate and verify local caches.

### Phase 3: WP-CLI Execution

1. **Subprocess Wrapper:** Implement `internal/wpcli.RunWP()`. Use `os/exec.Command("wp", args...)`. Ensure `stderr` and `stdout` are captured properly, mimicking the TS promise wrapper.
2. **Base Commands:** Implement `jman wp`.
3. **Plugin & Admin Tools:** Implement `jman plugin`, `jman admin`, and `jman mods`.
4. **Concurrency:** **[Major Improvement]** In the TS version, operations running across multiple sites (like `jman mods`) often run sequentially. In Go, we can use `goroutines` and `sync.WaitGroup` to execute SSH commands across dozens of sites in parallel, drastically reducing execution time.

### Phase 4: MainWP, Aliases, & Searching

1. **Search UI:** Implement `jman search` using `promptui` for an interactive filtering experience.
2. **Aliases:** Implement `jman alias` to generate YAML files by dumping Go structs configured for WP-CLI aliases.
3. **MainWP Integration:** Implement the MainWP API POST requests and `wp-cli` checks for `mainwp-child` to power `jman mainwp` and `jman inactive`.

### Phase 5: Vulnerability Scanning & Notifications

1. **Vuln Client:** Implement the WPVulnerability API.
2. **Scanning Engine:** Implement `jman vuln`. Use goroutines to fetch vulnerability data for multiple plugins concurrently.
3. **Slack Integration:** Implement `internal/slack` to format and send vulnerability alerts, respecting the CVSS threshold configuration. Ensure the deduplication/tracking logic (currently using JSON files in `runtimeData.dataDir`) is ported over accurately.

## 4. Build & Distribution

- Replace the `bun build` pipeline with Go tooling.
- The `Makefile` or GoReleaser can be used to easily cross-compile static binaries for multiple architectures (e.g., Linux, macOS ARM64).

```bash
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=x.y.z" -o bin/jman cmd/jman/main.go
```
