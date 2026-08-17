# jman-agent

`jman-agent` is a lightweight sidecar service that runs directly on each server managed by `jman`. It collects data that can't be pulled from SpinupWP or over SSH — per-site disk usage, whether a site is running WordPress Multisite, and whether `DISALLOW_FILE_MODS` is set — and pushes it to `jman-api` on a schedule.

Unlike `jman-api` and `jman-monitor`, `jman-agent` does **not** need `jman` installed alongside it. It runs standalone, on the managed server itself, with only a small config file and its own binary.

## How it Works

On each collection cycle, `jman-agent`:

1. Calls `GET /api/agent/manifest` on `jman-api` (authenticated with its own per-server token) to find out which sites live on this server and where.
2. Collects, locally and without SSH:
   - **Disk usage** per site, via `du -sb` (falling back to a manual directory walk if `du` isn't available).
   - **WordPress flags** per site, by reading `wp-config.php` directly for the `MULTISITE` and `DISALLOW_FILE_MODS` constants.
3. Sends everything back in one batched `POST /api/agent/report` request.

It also checks for and installs its own updates on a schedule — see [Self-Updating](#self-updating) below.

## Installation

### Option A: Download a prebuilt binary (recommended)

1. Open the latest release page: https://github.com/JCO-Digital/jman/releases/latest
2. Download `jman-agent` for the server's OS/architecture, along with `jman-agent.minisig`.
3. Move it into place and make it executable:

   ```bash
   sudo mv jman-agent /usr/local/bin/jman-agent
   sudo chmod +x /usr/local/bin/jman-agent
   ```

### Option B: Build from source

```bash
git clone https://github.com/JCO-Digital/jman.git
cd jman
make build
# The binary will be available at ./bin/jman-agent
```

## Generating a Server Token

`jman-agent` authenticates to `jman-api` with a per-server token, not a human login. Generate one from a machine that already has `jman` set up against the same `jman-api` database:

```bash
jman agent token create <server-id-or-name> --description "optional note"
```

This prints the plaintext token (`<id>.<secret>`) **once** — it cannot be retrieved again, only revoked and replaced. Copy it into the agent's config file (below).

Manage existing tokens with:

```bash
jman agent token list
jman agent token revoke <id>
```

Tokens can also be created and revoked from the jman-ui admin Settings page.

## Configuration

Create a config file on the managed server at `/etc/jman-agent/config.toml` (or `$XDG_CONFIG_HOME/jman-agent/config.toml` if not running as root):

```toml
apiUrl = "https://jman.example.com/api"
token = "3.q1w2e3r4t5y6..."   # from `jman agent token create`

# Optional — defaults shown
reportIntervalMinutes = 15
selfUpdateEnabled = true
selfUpdateCheckIntervalHours = 24
```

The file must not be readable by group or others (it contains the server's API token) — `jman-agent` refuses to start otherwise:

```bash
sudo chmod 600 /etc/jman-agent/config.toml
```

| Key                            | Default | Description                                                             |
| :------------------------------ | :------ | :----------------------------------------------------------------------- |
| `apiUrl`                         | -       | Base URL of `jman-api`, e.g. `https://jman.example.com/api`.              |
| `token`                          | -       | This server's agent token, from `jman agent token create`.               |
| `reportIntervalMinutes`          | `15`    | How often to collect and push data.                                      |
| `selfUpdateEnabled`              | `true`  | Whether the agent checks for and installs its own updates.               |
| `selfUpdateCheckIntervalHours`   | `24`    | How often to check for a new version.                                    |

`apiUrl` and `token` can also be set via the `JMAN_AGENT_API_URL` and `JMAN_AGENT_TOKEN` environment variables, which take precedence over the file — useful for container/secret-manager based deployments.

## Usage

### Service Mode (Recommended)

```bash
jman-agent --service
```

Runs continuously: an initial collection on startup, then one on every `reportIntervalMinutes` tick, plus a periodic self-update check.

### One-off Mode

Run a single collection-and-report cycle and exit — useful for testing, or driving the agent from an external cron instead of the built-in scheduler:

```bash
jman-agent --once
```

### Flags

- `-s`, `--service`: Run as a continuous background service.
- `--once`: Run a single collection cycle and exit.
- `--config <path>`: Path to `config.toml` (default: `/etc/jman-agent/config.toml`, or `$XDG_CONFIG_HOME/jman-agent/config.toml` if not root).
- `-v`, `--verbose`: Enable verbose output.
- `-d`, `--debug`: Enable debug output.

## Running as a Service (systemd)

A systemd unit file is provided in the repository. To install it:

1. Copy the binary to `/usr/local/bin/jman-agent`.
2. Create and secure the config file at `/etc/jman-agent/config.toml` (see [Configuration](#configuration)).
3. Copy `jman-agent.service` to `/etc/systemd/system/`.
4. Reload and start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable jman-agent
   sudo systemctl start jman-agent
   ```

The service runs as `root` by default (see the comments in `jman-agent.service`) since the agent needs read access to every site's files to measure disk usage and check `wp-config.php`, and write access to its own binary to self-update.

## Self-Updating

`jman-agent` checks GitHub Releases for a newer version on its own schedule (`selfUpdateCheckIntervalHours`, jittered by a few minutes so a fleet of agents doesn't all check at once) — it does not depend on `jman` or `jman update` being present on the server at all.

When an update is found, the same Ed25519 signature verification used by `jman`/`jman-api`/`jman-monitor` applies: the new binary and its `.minisig` are downloaded, the signature is checked against the hardcoded public key, and only a valid, signed binary is installed. The agent then **re-executes itself in place** (replacing its own running process image, same PID) to pick up the update immediately — it does not rely on `systemctl restart` or the service manager noticing the file changed, so this works correctly under systemd, under a plain process supervisor, or run by hand.

Set `selfUpdateEnabled = false` in `config.toml` to manage updates manually instead (e.g. via your own configuration management).

## Troubleshooting

`jman agent token list` (and the jman-ui Settings token table) show both **Last Seen** and the reporting agent's **version**. These update at different points, which makes them useful together for diagnosing a silent agent:

- **Last Seen** updates on *any* authenticated request, including the manifest fetch at the start of every collection cycle.
- **Version** only updates after a full, successfully parsed `POST /api/agent/report` — so it doubles as confirmation that reports are actually getting through, not just that the token is valid.

If **Last Seen** is recent but **Version** never appears (or stops updating), the agent is authenticating fine but its reports aren't landing — check `journalctl -u jman-agent` on that server for collection or network errors. Per-site collection failures (e.g. a `du`/`wp-config.php` read failing for a specific site) are logged at normal verbosity by default, no `--debug` flag needed.

## API Endpoints

`jman-api` exposes these endpoints for `jman-agent` (authenticated via the `X-Agent-Token` header, not JWT):

- `GET /api/agent/manifest`: Returns the list of sites this server's token is scoped to, along with each site's Unix user and public folder name (the agent resolves the actual filesystem path itself via the OS's own user database, since site layouts vary by hosting provisioning convention).
- `POST /api/agent/report`: Accepts a batch of collected data for those sites. Reports for sites outside the calling server's own token are rejected.

And for admins managing tokens (JWT, admin level):

- `GET /api/agent-tokens`: List all agent tokens.
- `POST /api/agent-tokens`: Create a new token for a server (returns the plaintext token once).
- `DELETE /api/agent-tokens/{id}`: Revoke a token.
