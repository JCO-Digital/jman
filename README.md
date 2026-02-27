# jman

`jman` is a command-line utility designed to manage WordPress sites hosted on SpinupWP, with additional support for MainWP integration. It provides a streamlined way to fetch site data, run remote `wp-cli` commands, manage plugins, and create administrative users across multiple sites.

_Note: `jman` was recently rewritten in Go for improved performance, concurrent operations, and to provide a statically linked binary._

## Features

- **SpinupWP Integration**: Fetch and list site/server data directly from the SpinupWP API.
- **Remote WP-CLI**: Execute `wp-cli` commands on remote sites via SSH.
- **MainWP Support**: Automated MainWP Child plugin installation and site management.
- **Bulk Operations**: Perform actions like disabling file modifications or installing plugins across multiple sites.
- **Site Aliases**: Generate YAML-based alias files for SSH and WP-CLI, supporting both individual sites and server-based groups.
- **Local Caching**: Optimized performance by caching site and server metadata locally.

## Installation

You can install `jman` directly via the Go toolchain:

```bash
go install github.com/JCO-Digital/jman/cmd/jman@latest
```

### Prerequisites

- Go (v1.21 or later recommended)
- SSH access configured for your SpinupWP servers.
- `wp-cli` available on the target servers.

### Building from source

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
ignoreSites = ["example.com"] # (optional, list of domains to ignore during vulnerability scans)
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

| Command    | Description                                                                         |
| :--------- | :---------------------------------------------------------------------------------- |
| `fetch`    | Fetch latest data from SpinupWP and update local cache.                             |
| `list`     | List cached data from SpinupWP (`servers`, `sites`, or `all`).                      |
| `wp`       | Run a `wp-cli` command on a target site.                                            |
| `search`   | Search for a specific term across sites.                                            |
| `admin`    | Create a new administrator user on target sites.                                    |
| `plugin`   | Install a plugin on target sites. Supports WordPress.org slugs or custom repo URLs. |
| `mods`     | Set `DISALLOW_FILE_MODS` to true on target sites.                                   |
| `alias`    | Create SSH/WP-CLI alias files for all sites or a filtered collection.               |
| `inactive` | List sites that don't have an active MainWP Child connection.                       |
| `mainwp`   | Install and configure MainWP on sites.                                              |
| `vuln`     | Scan for plugin vulnerabilities across all sites.                                   |

### Examples

**Run WP-CLI on a site:**

```bash
jman wp mysite.com plugin list --status=active
```

**Install a plugin on a site:**

```bash
jman plugin mysite.com akismet
```

**Create an admin user:**

```bash
jman admin mysite.com myusername user@example.com
```

**Generate WP-CLI aliases for a server group:**

```bash
# This outputs a YAML structure compatible with WP-CLI's alias configuration
jman alias my-server-name > ~/.wp-cli/alias.yml
```

**Scan for plugin vulnerabilities:**

```bash
# Scan all plugins and display vulnerabilities
jman vuln

# Filter by CVSS score (only show vulnerabilities with score >= 7.0)
jman vuln cvss 7.0

# Send vulnerability reports to Slack (requires Slack configuration)
jman vuln slack
```

The `vuln` command checks all cached plugins against known vulnerability databases and reports:

- Plugin name and affected versions
- Vulnerability details and CVSS scores
- List of sites running vulnerable plugin versions

When using the `slack` target, the command tracks sent messages to avoid duplicates and only resends for high-severity vulnerabilities (based on configured CVSS threshold).

## Sidecar Utilities

In addition to the main `jman` CLI, this repository includes sidecar utilities:

- **[jman-api](README_API.md)**: A lightweight REST API to serve cached data.
- **[jman-monitor](README_MONITOR.md)**: An automated uptime monitoring and Slack alerting tool.

## Development

- **Run locally without compiling:** `go run ./cmd/jman <command>`
- **Test:** `make test`
- **Format:** `make format`

## License

GPL-3.0-only
