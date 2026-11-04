# jman

`jman` is a command-line utility for managing the WordPress sites in [jman-api](README_API.md): sites hosted on SpinupWP and sites added manually. It runs remote `wp-cli` commands, manages plugins and WordPress core, and creates administrative users across multiple sites.

## Features

- **jman-api backend**: Site, server and plugin data comes from jman-api, so every user sees the same sites, including manually added ones, and the same update locks.
- **Remote WP-CLI**: Execute `wp-cli` commands on remote sites via SSH.
- **Bulk Operations**: Update, install or remove plugins and update WordPress core across multiple sites, as jman-api jobs that respect update locks.
- **Site Aliases**: Generate YAML-based alias files for SSH and WP-CLI, supporting both individual sites and server-based groups.
- **Works offline**: A local copy of the jman-api data keeps read-only commands working when jman-api can't be reached.

## Installation

### Option A: Automated install script (recommended)

For Linux and WSL, you can install the latest release directly via:

```bash
curl -fsSL https://raw.githubusercontent.com/JCO-Digital/jman/main/install.sh | sh
```

Or using `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/JCO-Digital/jman/main/install.sh | sh
```

The script:

1. Installs the latest `jman` binary to `~/.local/bin/jman`.
2. Checks for older `jman` installations elsewhere in your `PATH`, automatically removing them if user-writable or providing instructions if administrative permissions are required.
3. Automatically generates and installs shell completions for **Bash**, **Zsh**, and **Fish**.
4. Alerts you if `~/.local/bin` is not currently in your `PATH` and displays instructions for adding it.

### Option B: Manual binary download

1. Open the latest release page:
   https://github.com/JCO-Digital/jman/releases/latest
2. Download the `jman` executable for your architecture.
3. Move the binary into your `PATH` (e.g., `~/.local/bin/jman` or `/usr/local/bin/jman`):

```bash
chmod +x jman
mv jman ~/.local/bin/jman
```

### Option C: Build from source

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

- A user account in jman-api. Changing plugins and WordPress core needs the execute level.
- SSH access to the sites, for `wp`, `admin`, `mods`, `setup` and `plugin list --live`/`plugin info`.
- `wp-cli` available locally and on the target servers.
- Go (needed if building from source, v1.25 or later recommended)

## Configuration

`jman` uses a TOML configuration file located in your XDG config directory (typically `~/.config/jman/config.toml` on Linux and macOS). It must not be readable by other users (`chmod 600`).

The only required setting is the jman-api URL, which `jman login` saves for you (see [Logging in](#logging-in)). Everything else is optional:

```toml
apiUrl = "https://jman.example.com" # set by `jman login <url>`

# How the local copy of the jman-api data is kept fresh (Go durations)
cacheTTL = "2m"        # how old it can get before a command checks jman-api
apiTimeout = "3s"      # how long to wait for that check
offlineBackoff = "15m" # after a failed check, how long to work offline before trying again
staleWarning = "168h"  # warn when working from data older than this

# Thresholds for `jman vuln sites`
cvssThreshold = 7.0 # sites with a vulnerability of at least this CVSS
vulnThreshold = 7.0 # sites with at least this many vulnerabilities

# Port knocking for SSH connections (optional)
# knockdPorts = "7000,8000:udp,9000" # Port sequence before SSH/remote connections
# knockdTimeout = 60 # Timeout in seconds to assume knocked port remains open (defaults to 60)

# Plugin aliases for shorthand installs
[pluginAliases]
jquest = "https://github.com/JCO-Digital/jquest-plugin/releases/latest/download/jquest.zip"

# Longer WP-CLI timeouts for slow SSH hosts (optional, minutes; see
# docs/slow-ssh-hosts.md)
[[wpcliHostTimeouts]]
host = "*.ssh.wpengine.net"
minutes = 5
```

The SpinupWP and Slack tokens are jman-api settings; the CLI no longer uses them.

## Usage

```bash
jman <command> [target] [args...]
```

### Logging in

Log in once per device. `jman login` opens the jman web UI in your browser, where you approve the login; the token for this device is stored in the system keyring (or in a file only you can read if there is no keyring):

```bash
jman login https://jman.example.com   # the first time
jman login                            # later, e.g. after the token expired
```

A token expires after 90 days without use. Your tokens are listed, and can be revoked, under Settings → CLI Tokens in the web UI. `jman logout` revokes this device's token.

### Local copy of the data

Commands read sites, servers and plugins from a local copy of the jman-api data, which is checked for changes when it is older than `cacheTTL` (2 minutes). When jman-api can't be reached, commands keep working from the local copy and say so. Vulnerability reports and changes to plugins and core always need jman-api.

To pick up a site that was just created in SpinupWP, refresh explicitly:

```bash
jman refresh        # with the execute level, jman-api also refreshes from SpinupWP first
jman --fresh search mysite   # or check jman-api before any one command
```

### Available Commands

| Command  | Description                                                           |
| :------- | :-------------------------------------------------------------------- |
| `login`  | Log in to jman-api through the browser (`logout` to log out).         |
| `refresh` | Download the latest data from jman-api.                              |
| `wp`     | Run a `wp-cli` command on a target site.                              |
| `core`   | Manage WordPress core (check, update, version) on target sites.       |
| `search` | Search for sites or plugins matching a query.                         |
| `admin`  | Create a new administrator user on target sites.                      |
| `plugin` | Plugin actions (list, install, update, remove, info) on target sites. |
| `mods`   | Set `DISALLOW_FILE_MODS` to true on target sites.                     |
| `alias`  | Create SSH/WP-CLI alias files for all sites or a filtered collection. |
| `vuln`   | Show plugin and core vulnerabilities found by jman-api.               |
| `ignore` | Manage unified ignore list for monitoring and vulnerabilities.        |
| `update` | Check for and install updates for `jman` or its sidecar binaries.     |

When a target matches one site, read-only commands (`plugin list`, `plugin info`, `core version`, `core check`) run on it straight away, and commands that change sites ask you to confirm. When it matches several sites, you choose which ones. `--yes` (`-y`) skips all of this and runs on every matching site, which is useful in scripts:

```bash
jman -y plugin update prod-server akismet
```

### Examples

**Run WP-CLI on a site:**

```bash
jman wp mysite.com plugin list --status=active
```

**Install a plugin on a site:**

```bash
jman plugin install mysite.com akismet
```

**Show plugin details (name, version, status, author, available update):**

```bash
jman plugin info mysite.com akismet   # one plugin
jman plugin info mysite.com           # every plugin on the site
```

**Manage WordPress users:**

```bash
# List a site's users (subscribers and WooCommerce customers are only counted)
jman user list mysite.com

# Find a user on every site by login, email or name
jman user find someone@example.com

# Create an administrator; the generated password is shown once
jman user add mysite.com myusername user@example.com
jman user add mysite.com editor1 editor@example.com --role editor --password '…'

# Delete matching users, picking from a list; users with posts need --reassign
jman user delete mainwp
jman user delete olduser --reassign admin@example.com
```

**Manage WordPress core:**

```bash
# Check for core updates
jman core check mysite.com

# Update core to the latest minor version
jman core update mysite.com

# Update to a new major version (and confirm it on update-locked sites)
jman core update mysite.com --major --allow-major
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

# List backport rules (vulnerabilities fixed in releases the database doesn't list)
jman vuln backports
```

The `vuln` command shows what jman-api found when checking every site's plugins and WordPress core against known vulnerability databases:

- Plugin name and affected versions
- Vulnerability details and CVSS scores
- List of sites running vulnerable plugin versions

jman-api sends new vulnerabilities and a daily per-site summary to Slack itself.

When a vendor backports a fix to an older branch (e.g. WooCommerce 11.1.2 fixing a vulnerability the database lists as fixed in 11.2.0), add a backport rule with "Mark patched…" on the affected version in the web UI. Sites on a matching version are then not reported as affected; `jman vuln list` shows them under "Patched (backport)" while other sites are still affected.

## Sidecar Utilities

In addition to the main `jman` CLI, this repository includes sidecar utilities:

- **[jman-api](README_API.md)**: A lightweight REST API to serve cached data, run in-process uptime monitoring, and track incidents.
- **[jman-agent](README_AGENT.md)**: A self-updating agent that runs on managed servers to report per-site disk usage and WordPress config flags.
- **[jman-ui](web/README.md)**: A Vue 3 + Vite web frontend that consumes `jman-api` for dashboards, site/asset management, incidents, and monitoring views.

## Development

- **Run locally without compiling:** `go run ./cmd/jman <command>`
- **Test:** `make test`
- **Format:** `make format`
- **Run the API with hot reload:** `make dev-api`
- **Run the web UI dev server:** `make dev-ui` (see [web/README.md](web/README.md) for frontend-specific details)

### Local development

`make local-dev` starts the whole stack with one command: `jman-api` with hot reload (air), the web UI (Vite) and a `jman` CLI that is already logged in. It creates a throwaway user `admin` with a random password and TOTP, and prints the URLs and credentials. Ctrl-C stops everything. It needs Go, pnpm, curl, jq, openssl, OpenSSH and coreutils' `base32`. If air isn't installed, a pinned version runs through `go run`. jman-api runs wp-cli locally for its SSH work: an installed `wp` is used, else (with PHP) a pinned wp-cli is downloaded into `.local-dev/`.

- **Isolated:** all state (config, `users.toml`, databases, CLI token, SSH key) is in `./.local-dev/`, never in `~/.config/jman`, `~/.ssh` or the OS keyring. Inherited `JMAN_*` variables are ignored. jman-api's `ssh`, `scp` and `ssh-keygen` go through wrappers with their own config, key and `known_hosts`, and it gets no SSH agent, so it can only log in to the local containers.
- **No side effects:** there are no SpinupWP, Slack or PagerDuty tokens, and uptime monitoring is off. The refresh runs every 5 minutes: it reads plugins and core over SSH, fetches plugin info and vulnerability data from wordpress.org and the WPVulnerability API, and syncs the exposures. The seeded fake servers and sites use reserved names (`*.example.test`, `*.invalid`, `192.0.2.x`) that never resolve, so `Cannot connect over SSH` warnings about them in its log are expected.
- **Login:** user `admin`, the printed password, and the code from `.local-dev/totp` (the secret is in `.local-dev/admin-totp-secret`, for an authenticator app). With TOTP the user has the admin level, so update jobs, AI runs and user admin work.
- **CLI:** run `.local-dev/jman <command>`, e.g. `.local-dev/jman search example` or `.local-dev/jman ai run vuln-rank`.
- **AI gateway:** canned answers (`aiFake`) by default. `LOCAL_DEV_ANTHROPIC_API_KEY=sk-ant-... make local-dev` uses the Claude API instead; the key goes to jman-api through its environment only. Local runs send public vulnerability data and counts of the made-up sites.
- **Your settings:** `.local-dev.toml` in the repo root (gitignored, or another file named in `LOCAL_DEV_API_CONFIG`) is appended to the generated jman-api config on every start, e.g. `[aiAgents.vuln-rank]` with `model = "claude-haiku-5-5"`. Add keys and tables there, but don't repeat the generated top-level keys: TOML refuses a key set twice.
- **Ports:** 8080 (API), 5173 (UI) and 2221–2223 (the containers' SSH, on 127.0.0.1); override them with `LOCAL_DEV_API_PORT`, `LOCAL_DEV_UI_PORT` and `LOCAL_DEV_SSH_PORT` (the first of the three). Open the UI at `localhost`, not `127.0.0.1`, or its session cookie won't reach the API. `LOCAL_DEV_STATE` and `LOCAL_DEV_PROJECT` move the state directory and the Compose project, to run a second stack next to the first.
- **Reset:** `make local-dev-reset` deletes `.local-dev/` and the containers' volumes.

#### Server containers

With `make local-dev` running, `make local-dev-agent` (needs Docker with the buildx plugin) starts three containers that look like SpinupWP servers:

- `dev-agent-1` (alpha, production; beta, staging) and `dev-agent-2` (gamma, production) run `jman-agent` built from this checkout, reporting every minute.
- `dev-ssh-1` (delta, production) has no agent, so jman-api reads it over SSH only, as it does agentless sites.
- WordPress sites at `/sites/<domain>/files`, each owned by its own user, nginx access logs, PHP-FPM and a shared MariaDB.
- SSH as the site user (`ssh -F .local-dev/ssh/config -p 2221 alpha@127.0.0.1`), with the key in `.local-dev/ssh`. jman-api uses it for plugin and core updates, live update checks, test connections and the agentless refresh.
- Old plugin releases with published vulnerabilities, so jman finds real exposures: Duplicator 1.3.26 (in CISA KEV), Contact Form 7 5.3.1 (CVSS 10), WP Statistics, LiteSpeed Cache, Really Simple SSL, Forminator and Wordfence. gamma runs WordPress 6.8.2 for core vulnerabilities. The list is `site_plugins` in `scripts/local-dev/agent/entrypoint.sh`. The containers' web servers aren't published on the host. A plugin is only installed when it is missing, so updates made through jman stay; `make local-dev-reset` brings the old versions back.

It adds the servers and sites to jman-api and creates their agent tokens with `jman-api agent-token`. Run it again after changing the agent code to rebuild it. The exposures appear with the next refresh, within 5 minutes of the sites' first report. The sites, databases and scan baselines persist until `make local-dev-agent-down` plus `make local-dev-reset`. Installing WordPress and the plugins, the checksums and the vulnerability data come from wordpress.org and the WPVulnerability API. The agent reaches jman-api through `host.docker.internal`, because jman-api listens on all interfaces. A host firewall can block that: the containers' bridge is named after the Compose project (`jman-local-dev`), so with ufw run `sudo ufw allow in on jman-local-dev to any port 8080 proto tcp`. `make local-dev-agent` checks the connection and prints this command when it's blocked.

- `make local-dev-agent-infect` plants inert files on `alpha.local-dev.test` that match the scan rules: PHP in uploads, an `eval(base64_decode(…))` of an `echo`, an `auto_prepend_file` `.htaccess`, a `.sql` dump, and a comment appended to a core file and a plugin file. `make local-dev-agent-clean` removes them again. Findings appear, and later clear, within a minute or two.
- `docker compose -p jman-local-dev exec dev-agent-1 jman-agent --check-wp` (or `--check-scan`) checks a server by hand. `docker compose -p jman-local-dev logs -f` follows the logs.

## License

GPL-3.0-only
