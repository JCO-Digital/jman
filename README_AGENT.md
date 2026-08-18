# jman-agent

`jman-agent` is a lightweight sidecar service that runs directly on each server managed by `jman`. It collects data that can't be pulled from SpinupWP or over SSH — per-site disk usage, whether a site is running WordPress Multisite, whether `DISALLOW_FILE_MODS` is set, and hourly visitor traffic from access logs — and pushes it to `jman-api` on a schedule.

Unlike `jman-api` and `jman-monitor`, `jman-agent` does **not** need `jman` installed alongside it. It runs standalone, on the managed server itself, with only a small config file and its own binary.

## How it Works

On each collection cycle, `jman-agent`:

1. Calls `GET /api/agent/manifest` on `jman-api` (authenticated with its own per-server token) to find out which sites live on this server and where.
2. Collects, locally and without SSH:
   - **Disk usage** per site, via `du -sb` (falling back to a manual directory walk if `du` isn't available).
   - **WordPress flags** per site, by reading `wp-config.php` directly for the `MULTISITE` and `DISALLOW_FILE_MODS` constants.
   - **Visitor traffic** per site, by tailing its nginx access logs — see [Visitor Traffic Analytics](#visitor-traffic-analytics) below.
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
| `stateDir`                       | `/var/lib/jman-agent` (root) or XDG state dir | Where per-site log-tailing state (byte offsets, processed-rotation markers) is kept. Local only, never sent to jman-api. |

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

`jman-agent` checks GitHub Releases for a newer version two ways:

- **Periodic ticker**: on its own schedule (`selfUpdateCheckIntervalHours`, jittered by a few minutes so a fleet of agents doesn't all check at once).
- **Manifest-triggered fast path**: every `GET /api/agent/manifest` response includes `jman-api`'s own running version. Since every binary in this repo is built from one shared version per release, a newer `jman-api` implies a newer `jman-agent` release exists too — so on every collection cycle (`reportIntervalMinutes`, default 15) the agent compares that against its own version, and if it's behind, checks GitHub immediately rather than waiting for the next ticker fire. This comparison is local (no extra network call); GitHub is only actually contacted once a real version gap is detected.

Neither depends on `jman` or `jman update` being present on the server at all.

When an update is found, the same Ed25519 signature verification used by `jman`/`jman-api`/`jman-monitor` applies: the new binary and its `.minisig` are downloaded, the signature is checked against the hardcoded public key, and only a valid, signed binary is installed. The agent then **re-executes itself in place** (replacing its own running process image, same PID) to pick up the update immediately — it does not rely on `systemctl restart` or the service manager noticing the file changed, so this works correctly under systemd, under a plain process supervisor, or run by hand.

Set `selfUpdateEnabled = false` in `config.toml` to manage updates manually instead (e.g. via your own configuration management).

## Visitor Traffic Analytics

`jman-agent` tails each site's nginx access logs at `/sites/<domain>/logs/access.log` and reports hourly visitor stats: total/human/bot request counts, unique visitors, and top pages/referrers.

**Log rotation**: the live log is always `access.log`; rotated logs are immediately compressed as `access.log-YYYYMMDD.gz` (no numbered `.1`/`.2` intermediates). Rotated files are immutable once they exist, so each is parsed exactly once and never revisited; the live file is tailed incrementally by byte offset, advancing only past complete lines so a line still being written is picked up whole on the next cycle.

**Traffic classification**:
- Requests from jman's own synthetic traffic — jman-monitor's uptime checks (user agent `jman/1.0 ...`, or a `jman_cache_bypass` query parameter) — are excluded entirely, not counted as either human or bot, since they aren't real visitors and would otherwise inflate every site's numbers by however often jman-monitor pings it.
- Everything else is classified as bot (a static, dependency-free user-agent substring list — `bot`, `crawl`, `spider`, `facebookexternalhit`, etc.) or human. This is a simple v1: it won't catch a bot spoofing a real browser's user agent.

**Hourly close-then-send model**: an hour's data is only sent once it's fully elapsed (the wall clock has moved past it) — not incrementally every report cycle. This means jman-api can store each hour with a plain replace-style upsert (no merge logic needed) and unique-visitor counts are exact *within* an hour, at the cost of a delay of up to one report interval past the hour's end before that hour's data appears in jman-ui. The **daily** rollup shown in jman-ui is derived by summing/re-merging each day's hourly rows — its `unique_visitors` is therefore an approximate upper bound (a visitor active across multiple hours is counted once per hour), not a true daily-distinct count, and its top pages/referrers are merged from each hour's already-truncated top-20 lists rather than the day's full raw counts.

**Geo-IP / country breakdown is not implemented** — deferred pending a decision on distributing a MaxMind GeoLite2 database (which requires a license key and can't be bundled with the agent binary).

## Troubleshooting

`jman agent token list` (and the jman-ui Settings token table) show both **Last Seen** and the reporting agent's **version**. These update at different points, which makes them useful together for diagnosing a silent agent:

- **Last Seen** updates on *any* authenticated request, including the manifest fetch at the start of every collection cycle.
- **Version** only updates after a full, successfully parsed `POST /api/agent/report` — so it doubles as confirmation that reports are actually getting through, not just that the token is valid.

If **Last Seen** is recent but **Version** never appears (or stops updating), the agent is authenticating fine but its reports aren't landing — check `journalctl -u jman-agent` on that server for collection or network errors. Per-site collection failures (e.g. a `du`/`wp-config.php` read failing for a specific site) are logged at normal verbosity by default, no `--debug` flag needed.

A site missing from the manifest (no data reported, no error either) usually means it isn't fully `deployed` on SpinupWP yet — e.g. a staging site mid-clone. `jman-api` excludes such sites from the manifest; they'll appear automatically once deployment finishes and jman's local cache next refreshes (`jman fetch sites`).

If a site *is* in the manifest but its disk usage/wp-flags never show up, check for a "no valid site path found" error in the log — the agent looks for the site first at SpinupWP's standard `/sites/<domain>/files` layout, falling back to the home directory of `site_user` (if it resolves to a real local Unix account) for servers provisioned with a dedicated user per site. If neither exists, the site is skipped and logged. Note this deliberately ignores SpinupWP's own `public_folder` field, which describes the web-server-exposed docroot (sometimes nested for security) rather than where the WordPress install itself lives.

Traffic data specifically assumes logs live at `/sites/<domain>/logs` — this path is fixed, not resolved via the same user-home fallback as disk usage/wp-flags. A "failed to read logs directory" error means that path doesn't exist on this server (traffic is simply skipped for that site; other collected data is unaffected).

## API Endpoints

`jman-api` exposes these endpoints for `jman-agent` (authenticated via the `X-Agent-Token` header, not JWT):

- `GET /api/agent/manifest`: Returns the list of sites this server's token is scoped to, along with each site's domain and (optionally) Unix user — the agent resolves the actual filesystem path itself, since site layouts vary by hosting provisioning convention.
- `POST /api/agent/report`: Accepts a batch of collected data for those sites. Reports for sites outside the calling server's own token are rejected.

And for admins managing tokens (JWT, admin level):

- `GET /api/agent-tokens`: List all agent tokens.
- `POST /api/agent-tokens`: Create a new token for a server (returns the plaintext token once).
- `DELETE /api/agent-tokens/{id}`: Revoke a token.

And for jman-ui to display the collected data (JWT, basic level):

- `GET /api/sites/{id}/traffic?period=hourly|daily&days=N`: Returns a site's aggregated visitor traffic (default `period=hourly`, `days=7`, capped at 90 days).
