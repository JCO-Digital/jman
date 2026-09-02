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

# JWT token lifetime in hours (default: 24)
tokenLifetimeHours = 24

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

Authenticate with username and password to receive a JWT token.

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

Exchange a valid JWT for a fresh token with a new expiry. Requires authentication.

**Request Headers:**

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

**Success Response (`200 OK`):**

```json
{
	"token": "eyJhbGciOiJIUzI1NiIs...",
	"expiresAt": "2025-01-17T14:30:00Z"
}
```

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

### Monitoring Endpoints

- `GET /api/monitor/history?hours=48` — Returns aggregated status history for all sites.
- `GET /api/monitor/status?domain=...` — Returns current status for a specific site (or all sites if domain is omitted).

**Authentication error responses:**

| Status | Condition         | Body                                   |
| ------ | ----------------- | -------------------------------------- |
| 401    | No token provided | `{"error": "Authentication required"}` |
| 401    | Invalid token     | `{"error": "Invalid token"}`           |
| 401    | Expired token     | `{"error": "Token expired"}`           |

## Public Endpoints

These endpoints do not require authentication:

- `GET /api/health` — API health status and version.
- `POST /api/auth/login` — Authentication.

## Usage Example

```bash
# 1. Login to get a token
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your_password"}' \
  | jq -r '.token')

# 2. Use the token to access protected endpoints
curl -s http://localhost:8080/api/servers \
  -H "Authorization: Bearer $TOKEN" | jq .

# 3. Refresh the token before it expires
NEW_TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/refresh \
  -H "Authorization: Bearer $TOKEN" \
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
  work, since it fans out over SSH/wp-cli to every managed site.

If you have an external cron or systemd-timer job running `jman fetch` against this host,
remove it — `jman-api` now performs this refresh internally. Leaving the old job in place is
harmless but redundant (both would compete for the same limited SSH/wp-cli concurrency
against your managed servers).

Configuration (in `config.toml` or as `JMAN_*` environment variables):

| Key | Env var | Default | Description |
| --- | --- | --- | --- |
| `refreshDisabled` | `JMAN_REFRESHDISABLED` | `false` | Disable both refresh schedulers. |
| `refreshFastInterval` | `JMAN_REFRESHFASTINTERVAL` | `5` | Fast-tick interval, in minutes. |
| `refreshSlowInterval` | `JMAN_REFRESHSLOWINTERVAL` | `30` | Slow-tick interval, in minutes. |

The `jman fetch` CLI command still works exactly as before for manual/ad-hoc refreshes —
only the automatic external-cron dependency has been removed.

## Site Monitoring

`jman-api` also runs the uptime-monitoring scheduler in-process (the same scheduler
previously only available via the standalone `jman-monitor` daemon — see `README_MONITOR.md`).
Set `monitorDisabled = true` (or `JMAN_MONITORDISABLED=true`) to turn this off, for example if
you are still running a separate `jman-monitor` process against the same database during a
migration window. **Never run both jman-api's in-process monitor and a standalone
`jman-monitor` process against the same database at the same time** — each can independently
decide a site is down and send its own Slack alert, so running both risks duplicate/flapping
notifications and unnecessary database write contention.

## Security Notes

- The API does not handle TLS directly. It should be run behind a reverse proxy (nginx, Caddy, etc.) that terminates TLS.
- Passwords are never stored or logged in plaintext — only bcrypt hashes.
- Error messages for wrong username vs. wrong password are intentionally identical to prevent user enumeration.
- JWT tokens are signed with HS256 (HMAC-SHA256) using the `jwtSecret` from `users.toml`.
- Tokens are stateless and cannot be individually revoked. The configurable token lifetime (default: 24 hours) limits exposure.
