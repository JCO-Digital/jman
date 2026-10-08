# jman-api

`jman-api` is a lightweight REST API that serves the data cached by the `jman` CLI and provides management endpoints for site monitoring. This is useful for building dashboards or exposing your SpinupWP/Plugin data to other local services without needing to re-fetch from external APIs.

All data endpoints require JWT authentication. The API will refuse to start unless a valid `users.toml` configuration file is present.

## Running the API

You can build and run the API using:

```bash
make build
JMAN_API_PORT=8080 ./bin/jman-api
```

The API listens on the port specified by the `JMAN_API_PORT` environment variable (default: `8080`).

## CLI Helpers

`jman-api` includes built-in subcommands for managing users and credentials. Run `jman-api --help` to see all available commands.

### `useradd` — Add a new user

Creates a new user entry in `users.toml`. Prompts for a password interactively (with echo disabled). If `users.toml` does not yet exist, a new file is created with a randomly generated JWT secret.

```bash
jman-api useradd --username admin --display-name "Admin User"
```

| Flag             | Required | Description                   |
| ---------------- | -------- | ----------------------------- |
| `--username`     | Yes      | Username for the new user     |
| `--display-name` | Yes      | Display name for the new user |

The command will:

1. Create `users.toml` with a generated JWT secret if it doesn't exist.
2. Reject duplicate usernames.
3. Prompt for the password twice (with confirmation).
4. Hash the password with bcrypt (cost factor 12).
5. Append the `[[users]]` entry and save with `0600` permissions.

### `hashpw` — Hash a password

A standalone utility that prompts for a password and prints the bcrypt hash to stdout. Useful for manually constructing or editing `users.toml` entries.

```bash
jman-api hashpw
```

### `totp-setup` — Configure TOTP for a user

Generates a new TOTP secret for an existing user, prints the base32 secret and an `otpauth://` URI (suitable for QR code generation), and updates `users.toml`.

```bash
jman-api totp-setup --username admin
```

| Flag         | Required | Description                    |
| ------------ | -------- | ------------------------------ |
| `--username` | Yes      | Username to configure TOTP for |

The command will:

1. Load `users.toml` and find the specified user.
2. Warn and ask for confirmation if the user already has a TOTP secret.
3. Generate a new secret compatible with standard authenticator apps (Google Authenticator, Authy, etc.).
4. Print the base32 secret and `otpauth://` URI to stdout.
5. Save the updated `users.toml`.

Once configured, the user must provide a valid TOTP code at login. If the `totpSecret` field is empty or omitted, TOTP is not required for that user.

## Authentication Setup

### 1. Create `users.toml`

The quickest way to get started is with the `useradd` command:

```bash
jman-api useradd --username admin --display-name "Admin User"
```

This creates `~/.config/jman/users.toml` automatically. You can also create it manually with the following structure:

```toml
# Secret used to sign and verify JWT tokens.
# Generate with: openssl rand -hex 32
jwtSecret = "your_64_char_hex_string_here"

# Lifetime of the short-lived access token (JWT) in minutes (default: 15)
accessTokenLifetimeMinutes = 15

# How long a login session lasts without being used, in days (default: 30).
# Each refresh restarts this window.
refreshTokenLifetimeDays = 30

[[users]]
username = "admin"
passwordHash = "$2a$12$..."  # bcrypt hash
displayName = "Admin User"
totpSecret = ""              # empty = TOTP not required

[[users]]
username = "readonly"
passwordHash = "$2a$12$..."
displayName = "Read-Only User"
totpSecret = "JBSWY3DPEHPK3PXP"  # base32-encoded TOTP secret
```

### 2. Generate a JWT Secret

If you used `jman-api useradd` to create the first user, a JWT secret was generated automatically. To generate one manually:

```bash
openssl rand -hex 32
```

The secret must be at least 32 characters long.

### 3. Generate Password Hashes

The easiest way is with the built-in `hashpw` command:

```bash
jman-api hashpw
```

You can also use external tools. For example, with `htpasswd`:

```bash
htpasswd -nbBC 12 "" 'your_password' | cut -d: -f2
```

Or with Python:

```bash
python3 -c "import bcrypt; print(bcrypt.hashpw(b'your_password', bcrypt.gensalt(12)).decode())"
```

### 4. Set File Permissions

The `users.toml` file contains sensitive credentials. Files created by the CLI helpers already have `0600` permissions. If you created the file manually, restrict its permissions:

```bash
chmod 600 ~/.config/jman/users.toml
```

The API will log a warning at startup if permissions are more open than `0600`.

### 5. TOTP (Optional)

The easiest way to configure TOTP is with the built-in command:

```bash
jman-api totp-setup --username admin
```

This generates a secret, updates `users.toml`, and prints the base32 secret and `otpauth://` URI for your authenticator app.

If a user has a `totpSecret` configured, they must provide a valid TOTP code at login. If the `totpSecret` field is empty or omitted, TOTP is not required for that user.

## Authentication Endpoints

### `POST /api/auth/login`

Authenticate with username and password to receive a short-lived access token (JWT). The response also sets the refresh token as a `jman_refresh` cookie (`HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/api/auth`).

**Request:**

```json
{
	"username": "admin",
	"password": "your_password",
	"totp": "123456"
}
```

The `totp` field is only required if the user has a `totpSecret` configured.

**Success Response (`200 OK`):**

```json
{
	"token": "eyJhbGciOiJIUzI1NiIs...",
	"expiresAt": "2025-01-16T14:30:00Z",
	"user": {
		"username": "admin",
		"displayName": "Admin User"
	}
}
```

**Error Responses:**

| Status | Condition                        | Body                                                    |
| ------ | -------------------------------- | ------------------------------------------------------- |
| 400    | Malformed JSON or missing fields | `{"error": "Invalid request body"}`                     |
| 401    | Wrong username or password       | `{"error": "Invalid credentials"}`                      |
| 401    | TOTP required but not provided   | `{"error": "TOTP code required"}`                       |
| 401    | TOTP code invalid                | `{"error": "Invalid TOTP code"}`                        |
| 429    | Too many failed attempts         | `{"error": "Too many login attempts, try again later"}` |

### `POST /api/auth/refresh`

Exchange the `jman_refresh` cookie for a new access token. No `Authorization` header is needed, so this works after the access token has expired. The refresh token is rotated: the response sets a new cookie, and the old one stops working. Presenting an already rotated refresh token more than 30 seconds after its rotation revokes the whole session, since it suggests the token was copied.

**Success Response (`200 OK`):**

```json
{
	"token": "eyJhbGciOiJIUzI1NiIs...",
	"expiresAt": "2025-01-17T14:30:00Z",
	"user": {
		"username": "admin",
		"displayName": "Admin User",
		"level": "admin"
	}
}
```

**Error Responses:** `401` with `Session expired` (missing, unknown or expired cookie), `Session revoked` (rotated token reused, or the user's password or 2FA changed) or `User no longer exists`. The cookie is cleared.

### `POST /api/auth/logout`

Ends the session named by the `jman_refresh` cookie and clears the cookie. Returns `204 No Content`. Access tokens already issued remain valid until they expire.

## Data Endpoints

All data endpoints require a valid JWT in the `Authorization` header:

```
Authorization: Bearer <token>
```

- `GET /api/plugins` — Returns all cached WordPress plugins across all sites.
- `GET /api/plugininfo` — Returns enriched information for all cached plugins (author, description, etc.).
- `GET /api/servers` — Returns cached SpinupWP servers.
- `GET /api/sites` — Returns cached SpinupWP sites.
- `GET /api/vulns?plugin=<slug>` — Returns cached vulnerability data for a specific plugin.

### Ignore List Endpoints

- `GET /api/ignore?type=...` — Returns all ignore entries (optional filter by type).
- `POST /api/ignore` — Adds a new ignore entry. Requires JSON body: `{"type": "site", "target": "123", "reason": "...", "use_for_monitor": true, "use_for_vuln": true}`.
- `PATCH /api/ignore/{id}` — Updates an existing ignore entry.
- `DELETE /api/ignore/{id}` — Removes an ignore entry.

### Monitoring & Incident Endpoints

- `GET /api/monitor/history?hours=48` — Returns aggregated status history for all sites.
- `GET /api/monitor/status?domain=...` — Returns current status for a specific site (or all sites if domain is omitted).
- `GET /api/incidents?filter=active` — Returns open and acknowledged incidents with total count and active count (filters: `active`, `resolved`, `closed`, `history`, `all`).
- `POST /api/incidents/{id}/acknowledge` — Acknowledges an active incident, notifies Slack, and cancels any pending/active PagerDuty alert.
- `POST /api/incidents/{id}/close` — Closes an incident manually and resets monitoring state for that domain.
- `POST /api/incidents/{id}/ignore` — Adds the site to the monitor ignore list, closes the incident, and cancels PagerDuty.

**Authentication error responses:**

| Status | Condition         | Body                                   |
| ------ | ----------------- | -------------------------------------- |
| 401    | No token provided | `{"error": "Authentication required"}` |
| 401    | Invalid token     | `{"error": "Invalid token"}`           |
| 401    | Expired token     | `{"error": "Token expired"}`           |

## Public Endpoints

These endpoints do not require authentication:

- `GET /api/health` — API health status and version.

Every response also carries the running version in an `X-Jman-Version` header. The web UI reloads itself when it changes, so open tabs pick up a new deploy.
- `POST /api/auth/login` — Authentication.
- `POST /api/auth/refresh` — Authenticated by the refresh token cookie.
- `POST /api/auth/logout` — Ends the session in the refresh token cookie.

## Usage Example

```bash
# 1. Login to get a token; the refresh token cookie goes into cookies.txt
TOKEN=$(curl -s -c cookies.txt -X POST https://jman.example.com/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your_password"}' \
  | jq -r '.token')

# 2. Use the token to access protected endpoints
curl -s https://jman.example.com/api/servers \
  -H "Authorization: Bearer $TOKEN" | jq .

# 3. Get a new access token with the (rotated) refresh token cookie
NEW_TOKEN=$(curl -s -b cookies.txt -c cookies.txt -X POST https://jman.example.com/api/auth/refresh \
  | jq -r '.token')
```

## Rate Limiting

The login endpoint is rate-limited to prevent brute-force attacks:

- **Limit:** 5 failed attempts per username within a 15-minute window.
- **Lockout:** After 5 failures, the username is locked out for 15 minutes.
- **Reset:** A successful login resets the failure counter.

## Data Refresh

`jman-api` refreshes its own cached data in the background — it no longer depends on an
external `jman fetch` cron job. Two independent schedulers run inside the API process:

- A **fast tick** (default every 5 minutes) refreshes the SpinupWP servers/sites cache.
- A **slow tick** (default every 30 minutes) refreshes installed plugins, plugin metadata,
  plugin vulnerabilities, and WordPress core versions/vulnerabilities — the more expensive
  work, since it fans out over SSH/wp-cli to every managed site. On success, it also syncs
  vulnerability findings into Tasks (creating/updating a `Task` per affected site — see
  `docs/TASK_SPECS.md`) and posts a one-off Slack alert (`slackChannel`) for each newly
  found vulnerability with CVSS >= `cvssThreshold`, listing the sites it is installed on.
  Each vulnerability is alerted only once (tracked in the `api.db` `vuln_alerts` table);
  on the first run after upgrading, existing vulnerabilities are recorded without alerting.
- Once a day, on the first slow tick at or after `vulnReportTime` (server local time),
  the per-site vulnerability summary is posted to Slack (`slackChannel`). Sites whose
  report is identical to one already sent are skipped.

If you have an external cron or systemd-timer job running `jman fetch` or
`jman vuln sites --slack` against this host, remove it — `jman-api` now performs both of
these internally. Leaving the old jobs in place is harmless but redundant (they'd compete
for the same limited SSH/wp-cli concurrency, and would send Slack messages without the
cross-run dedup `jman-api` gets from having `api.db` open).

Configuration (in `config.toml` or as `JMAN_*` environment variables):

| Key                   | Env var                    | Default | Description                      |
| --------------------- | -------------------------- | ------- | -------------------------------- |
| `refreshDisabled`     | `JMAN_REFRESHDISABLED`     | `false` | Disable both refresh schedulers. |
| `refreshFastInterval` | `JMAN_REFRESHFASTINTERVAL` | `5`     | Fast-tick interval, in minutes.  |
| `refreshSlowInterval` | `JMAN_REFRESHSLOWINTERVAL` | `30`    | Slow-tick interval, in minutes.  |
| `vulnReportTime`      | `JMAN_VULNREPORTTIME`      | `10:00` | Daily per-site vuln report time. |

### Agent collection

On servers running `jman-agent`, the agent collects each WordPress site's plugins and core
version itself, running wp-cli locally as the site's owner (see
[README_AGENT.md](README_AGENT.md#wordpress-data)). Once the agent has collected a site,
the slow tick no longer reaches it over SSH; jobs, "Check for updates" and the refresh after
an update still use SSH. A site the agent hasn't collected yet (an older agent, or a failure
on the first attempt) stays on the SSH refresh.

Whichever way a site's plugins and core version are read, jman compares them with what it
knew and records changes it didn't make itself (updates in wp-admin, WordPress
auto-updates, installs, removals, activations and deactivations, and major core changes) in
the site's update ledger with the status `detected`. Must-use plugins, drop-ins and minor
core changes are left out, and a read that suddenly lacks most of a site's plugins is only
believed once it's seen twice.

Agent health is checked every 5 minutes and reported to Slack (`slackMonitorChannel`, or
`slackChannel`): once when an agent stops calling in, once when a site's WordPress data
hasn't been collected for a while (sites that went stale together share one message), with
a daily reminder while it lasts and a message when it recovers.

| Key                       | Env var                        | Default | Description                                                                                       |
| ------------------------- | ------------------------------ | ------- | ------------------------------------------------------------------------------------------------- |
| `agentWpDataInterval`     | `JMAN_AGENTWPDATAINTERVAL`     | `60`    | How often agents collect each site's plugins and core, in minutes. Negative turns it off.         |
| `agentServerStaleMinutes` | `JMAN_AGENTSERVERSTALEMINUTES` | `30`    | Warn when a server's agent hasn't called in for this long.                                        |
| `agentSiteStaleMinutes`   | `JMAN_AGENTSITESTALEMINUTES`   | `180`   | Warn when an agent site's WordPress data hasn't been collected successfully for this long.        |

The `jman fetch` CLI command still works exactly as before for manual/ad-hoc refreshes —
only the automatic external-cron dependency has been removed.

## Plugin Management and Core Updates

Plugin updates, plugin activation, deactivation, deletion and installs, and WordPress core updates run as background jobs inside jman-api. The web UI polls `GET /api/update-jobs` while any job is queued or running and shows them in its task list.

- `POST /api/plugin-update-jobs` with `{"jobs": [{"site_id": "<uuid>", "plugins": ["akismet"]}]}` queues plugin updates, one job per site. Each job updates all of its plugins in a single `wp plugin update` call.
- `POST /api/sites/{id}/core-update` with `{"target": "minor"|"major"}` queues a core update and returns `202` with the job. It returns `409` if a core update is already queued or running for the site. When the job finishes, its `core` field holds the refreshed core version state.
- `GET /api/update-jobs` lists queued and running jobs of both kinds (`kind`: `plugins` or `core`), plus jobs that finished in the last 10 minutes. `GET /api/update-jobs/{id}` returns one job. `GET /api/plugin-update-jobs[/{id}]` is kept as an alias.
- `POST /api/sites/{id}/plugin-actions` with `{"action": "activate"|"deactivate"|"delete"|"uninstall", "plugins": ["akismet"]}` queues a plugin management job and returns `202`. `delete` removes the plugin's files only; `uninstall` runs its uninstall routine, which usually also removes its settings and data. Active plugins must be deactivated before they're deleted or uninstalled. Must-use plugins and drop-ins can't be managed.
- `POST /api/sites/{id}/plugin-install` queues installing one plugin and returns `202`. Send `{"source": "<slug or https ZIP URL>", "activate": true}` as JSON, or upload a ZIP (up to 64 MB) as `multipart/form-data` with fields `file` and `activate`. Uploaded ZIPs are copied to the site's server with scp and removed afterwards. A slug that's already installed returns `409`.
- `GET /api/sites/{id}/plugins` returns a site's cached plugins. `GET /api/sites/{id}/plugin-updates` checks the site live and refreshes that cache.
- Up to four sites update at once, and a site never has two jobs running.
- Each finished job writes one entry to the site's update ledger, with one of these statuses:
  - `full`: the site was brought up to date. Every update succeeded and no plugin updates are left.
  - `vuln`: the job updated vulnerable plugins, and no plugin with an update left is vulnerable (ignored vulnerabilities don't count), but other updates remain.
  - `partial`: the requested updates succeeded but updates remain, or a plugin action worked for only some of its plugins.
  - `failed`: nothing in the job succeeded (for updates: any plugin failed).
  - `activated`, `deactivated`, `deleted` (deleting files or a full uninstall; the entry's data says which), `installed`: a plugin action that worked for every plugin.
  - Core updates keep their existing statuses: `full` when updated, `partial` when already at the latest version, `failed` on error.
- A site's `last_update` (in `GET /api/sites`) is its latest ledger entry that isn't a plugin action, so activating or installing a plugin doesn't count as updating the site.
- Jobs are stored in `api.db`, so they survive page reloads. If jman-api restarts mid-update, the running job is marked interrupted and still-queued jobs run again.

## Slow SSH Hosts

For hosts that are slow over SSH, such as WP Engine, see [docs/slow-ssh-hosts.md](docs/slow-ssh-hosts.md). It covers SSH connection reuse, per-host WP-CLI timeouts, and what jman reports when an update fails.

## Site Monitoring & Incident Management

`jman-api` runs the automated uptime-monitoring scheduler in-process.

- **Downtime Detection**: Performs HTTP health checks against cached WordPress sites. When a site fails multiple consecutive checks, it transitions to Alert Mode.
- **Incident Creation**: Creates an incident record in the database and sends an immediate Slack alert.
- **Acknowledgment**: Any authenticated user can acknowledge the incident via the web UI. Acknowledging posts an audit note to Slack and cancels/suppresses PagerDuty alerting.
- **PagerDuty Escalation**: If an outage remains unacknowledged and open after 10 minutes (configurable via `pagerdutyEscalationMinutes`), a critical incident is sent to PagerDuty to page the on-call person.
- **Recovery**: When the site comes back up, `jman-api` automatically marks the incident resolved, notifies Slack, and resolves PagerDuty.
- **Disabling**: Set `monitorDisabled = true` (or `JMAN_MONITORDISABLED=true`) in config if you wish to disable uptime checks.

## Security Notes

- The API does not handle TLS directly. It should be run behind a reverse proxy (nginx, Caddy, etc.) that terminates TLS.
- Passwords are never stored or logged in plaintext — only bcrypt hashes.
- Error messages for wrong username vs. wrong password are intentionally identical to prevent user enumeration.
- JWT tokens are signed with HS256 (HMAC-SHA256) using the `jwtSecret` from `users.toml`.
- Access tokens are stateless and short-lived (default: 15 minutes). The web UI keeps them in memory only.
- Refresh tokens are random, stored only as SHA-256 hashes in `api.db`, and rotated on every use. Logging out, changing a password, or changing 2FA revokes them.
- The refresh token cookie is `Secure`, so the web UI must be served over HTTPS (or `localhost`). If the web UI runs on a different origin than the API, list that origin explicitly in `allowedOrigins`; a `*` wildcard can't carry cookies.
