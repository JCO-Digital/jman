# Project Exploration Findings: jman

`jman` is a Go-based suite of tools designed for managing WordPress installations hosted on SpinupWP. It includes a primary CLI, a monitoring service, and a REST API.

## Project Structure

- `cmd/`: Entry points for various binaries.
  - `jman/`: The main CLI tool, a client of `jman-api`.
  - `jman-api/`: A REST API server for accessing cached data, in-process uptime monitoring, and incident management.
  - `jman-agent/`: An agent running on managed servers for disk and config reporting.
  - `gen-keys/` & `sign-binaries/`: Internal tools for release signing and security.
- `internal/`: Core logic organized by domain.
  - `commands/`: Cobra command definitions for the CLI.
  - `db/`: Persistence layer using SQLite (via `modernc.org/sqlite`).
  - `wpcli/`: Wrappers for remote `wp-cli` execution over SSH.
  - `vuln/`: Logic for scanning and reporting plugin vulnerabilities.
  - `api/`: Implementation of the REST API.
  - `monitor/`: Implementation of the uptime monitoring service.
- `docs/`: Supplemental documentation.

## Key Technologies

- **Language**: Go (v1.25 recommended).
- **CLI Framework**: `spf13/cobra` and `spf13/viper`.
- **Database**: SQLite (CGO-free driver via `modernc.org/sqlite`).
- **Authentication**: JWT for the API, Ed25519 signatures for binary updates.
- **Integrations**: SpinupWP API, Slack (for notifications).

## Functional Areas

### 1. Centralized Management (`jman`)

- Gets server/site/plugin data from `jman-api` (`internal/client`, `internal/mirror`) and keeps a local JSON copy; logs in through the browser (`jman login`). It must not link the database code (`cmd/jman/deps_test.go`).
- Executes commands across multiple sites (bulk operations); plugin and core changes run as `jman-api` jobs.
- Manages plugins, WordPress core updates, and admin users.
- Generates WP-CLI and SSH alias files.

### 2. Monitoring & Vulnerability Tracking

- `jman-api` checks site availability, manages incidents, and sends Slack/PagerDuty alerts.
- `jman vuln` scans cached plugin data against vulnerability databases.
- Supports CVSS threshold filtering and Slack reporting.

### 3. API & Connectivity (`jman-api`)

- Serves cached site/server/plugin data via JSON.
- Implements JWT-based auth with TOTP support.
- Provides endpoints for managing monitoring ignore lists.

## Build & Configuration

### Build System

- **Makefile**: Provides targets for building all binaries (`build`), testing (`test`), and generating shell completions. Releases are distributed only through GitHub, and `jman update` installs them.
- **LDFlags**: Used during build to inject the application version into `internal/config.AppVersion`.

### Configuration

- **Main Config**: `jman` and `jman-api` share a TOML file layout (typically `~/.config/jman/config.toml`) managed via `viper`. The CLI uses `apiUrl` and the cache timing keys; `jman-api` uses the SpinupWP/Slack tokens and `slackChannel`, `slackMonitorChannel`, `slackTasksChannel`, `webUrl`, `minCliVersion` and `latestCliVersion`.
- **API Users**: `jman-api` uses a separate `users.toml` for authentication, storing bcrypt-hashed passwords and TOTP secrets.
- **XDG Support**: Follows XDG Base Directory Specification for configuration and data storage.

## Security Features

- **Binary Signing**: Official releases are signed with `minisig`. The `update` command verifies these signatures before installation.
- **API Security**: Rate limiting on login, bcrypt password hashing, and mandatory JWT secret configuration.
