# jman

`jman` is a command-line utility designed to manage WordPress sites hosted on SpinupWP. It provides a streamlined way to fetch site data, run remote `wp-cli` commands, manage plugins, and create administrative users across multiple sites.

## Features

- **SpinupWP Integration**: Fetch and cache site/server data directly from the SpinupWP API.
- **Remote WP-CLI**: Execute `wp-cli` commands on remote sites via SSH.
- **Bulk Operations**: Perform actions like disabling file modifications or installing plugins across multiple sites.
- **Site Aliases**: Generate YAML-based alias files for SSH and WP-CLI, supporting both individual sites and server-based groups.
- **Local Caching**: Optimized performance by caching site and server metadata locally.

## Installation

### Option A: Download a prebuilt binary (recommended)

1. Open the latest release page:
   https://github.com/JCO-Digital/jman/releases/latest
2. Download the executable for your OS/architecture.
3. Extract it.
4. Move the `jman` binary to a directory in your `PATH` (for example `~/.local/bin` or `/usr/local/bin` if you want it system wide).

Example (Linux/macOS):

```bash
chmod +x jman
mv jman ~/.local/bin/jman
```

### Option B: Build from source

If you prefer building locally, clone the repository and build with the project’s standard build process, then place the resulting `jman` binary in your `PATH`.

1. Clone the repository:

   ```bash
   git clone https://github.com/JCO-Digital/jman.git
   cd jman
   ```

2. Build the project using `make`:

   ```bash
   make build
   ```

3. (Optional) Install the binary to `~/.local/bin/`:
   ```bash
   make install
   ```

If you don't use `make install`, the compiled binary will be generated at `./bin/jman`.

## Security & Updates

To ensure the integrity of the tool, all official `jman` releases and sidecar binaries are signed using Ed25519 signatures.

When you run `jman update`, the tool automatically:

1. Checks for a new version and its corresponding signature file (`.minisig`).
2. Downloads the new binary to a temporary location.
3. Verifies the signature against a hardcoded public key.
4. Only replaces the current binary if the signature is valid.

This prevents unauthorized or tampered binaries from being installed via the update mechanism.

### Prerequisites

- SSH access configured for your SpinupWP servers.
- `wp-cli` available locally and on the target servers.
- Go (needed if building from source, v1.25 or later recommended)

## Configuration

`jman` uses a TOML configuration file located in your XDG config directory (typically `~/.config/jman/config.toml` on Linux and macOS).

Create the file and add your credentials:

```toml
tokenSpinup = "your_spinupwp_api_token"

# Slack notifications for vulnerabilities
slackToken = "xoxb-your-slack-bot-token" # (optional)
slackChannel = "#alerts" # (optional, defaults to #testing)

# Vulnerability scanning thresholds
cvssThreshold = 7.0 # (optional, alerts for vulnerabilities with CVSS >= this value)
vulnThreshold = 7.0 # (optional, alerts for sites with total vulnerabilities >= this value)

# Required only for `jman agent token` (create/list/revoke) — these talk to
# jman-api over HTTP, since agent tokens live in jman-api's own database.
# You'll be prompted for your jman-api password (and TOTP, if configured);
# it's never stored in this file.
apiURL = "https://jman-api.example.com" # (optional)
apiUsername = "admin" # (optional, must be an admin-level jman-api user)

# Plugin aliases for shorthand installs
[pluginAliases]
jquest = "https://github.com/JCO-Digital/jquest-plugin/releases/latest/download/jquest.zip"
```

## Usage

```bash
jman <command> [target] [args...]
```

### Local Caching

To speed up operations, `jman` caches site and server data from SpinupWP. If you add new sites or servers, you should run the `fetch` command to update your local cache:

```bash
jman fetch
```

### Available Commands

| Command  | Description                                                           |
| :------- | :-------------------------------------------------------------------- |
| `fetch`  | Fetch latest data from SpinupWP and update local cache.               |
| `wp`     | Run a `wp-cli` command on a target site.                              |
| `core`   | Manage WordPress core (check, update, version) on target sites.       |
| `search` | Search for sites or plugins matching a query.                         |
| `admin`  | Create a new administrator user on target sites.                      |
| `plugin` | Plugin actions (list, install, update, remove, info) on target sites. |
| `mods`   | Set `DISALLOW_FILE_MODS` to true on target sites.                     |
| `alias`  | Create SSH/WP-CLI alias files for all sites or a filtered collection. |
| `vuln`   | Scan for plugin vulnerabilities across all sites.                     |
| `ignore` | Manage unified ignore list for monitoring and vulnerabilities.        |
| `update` | Check for and install updates for `jman` or its sidecar binaries.     |

### Examples

**Run WP-CLI on a site:**

```bash
jman wp mysite.com plugin list --status=active
```

**Install a plugin on a site:**

```bash
jman plugin mysite.com install akismet
```

**Create an admin user:**

```bash
jman admin mysite.com myusername user@example.com
```

**Manage WordPress core:**

```bash
# Check for core updates
jman core check mysite.com

# Update core to latest version
jman core update mysite.com
```

**Generate WP-CLI aliases for a server group:**

```bash
# This outputs a YAML structure compatible with WP-CLI's alias configuration
jman alias my-server-name > ~/.wp-cli/alias.yml
```

**Scan for plugin vulnerabilities:**

```bash
# Scan all plugins and display vulnerabilities (vulnerability-centric)
jman vuln list

# Scan all plugins and display vulnerabilities (site-centric)
jman vuln sites

# Filter by CVSS score (only show vulnerabilities with score >= 7.0)
jman vuln list --cvss 7.0

# Send vulnerability reports to Slack (requires Slack configuration)
jman vuln list --slack
```

The `vuln` command checks all cached plugins against known vulnerability databases and reports:

- Plugin name and affected versions
- Vulnerability details and CVSS scores
- List of sites running vulnerable plugin versions

When using the `--slack` flag, the command tracks sent messages to avoid duplicates and only resends for high-severity vulnerabilities (based on configured CVSS threshold).

## Sidecar Utilities

In addition to the main `jman` CLI, this repository includes sidecar utilities:

- **[jman-api](README_API.md)**: A lightweight REST API to serve cached data.
- **[jman-monitor](README_MONITOR.md)**: An automated uptime monitoring and Slack alerting tool.
- **[jman-agent](README_AGENT.md)**: A self-updating agent that runs on managed servers to report per-site disk usage and WordPress config flags.

## Development

- **Run locally without compiling:** `go run ./cmd/jman <command>`
- **Test:** `make test`
- **Format:** `make format`

## License

GPL-3.0-only
