# jman-api

`jman-api` is a lightweight, read-only REST API that serves the data cached by the `jman` CLI. This is useful for building dashboards or exposing your SpinupWP/Plugin data to other local services without needing to re-fetch from external APIs.

All data endpoints require JWT authentication. The API will refuse to start unless a valid `users.toml` configuration file is present.

## Running the API

You can build and run the API using:

```bash
make build
JMAN_API_PORT=8080 ./bin/jman-api
```

The API listens on the port specified by the `JMAN_API_PORT` environment variable (default: `8080`).

## Authentication Setup

### 1. Create `users.toml`

Create the file `~/.config/jman/users.toml` with the following structure:

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

```bash
openssl rand -hex 32
```

The secret must be at least 32 characters long.

### 3. Generate Password Hashes

Use any bcrypt tool to generate password hashes. For example, with `htpasswd`:

```bash
htpasswd -nbBC 12 "" 'your_password' | cut -d: -f2
```

Or with Python:

```bash
python3 -c "import bcrypt; print(bcrypt.hashpw(b'your_password', bcrypt.gensalt(12)).decode())"
```

### 4. Set File Permissions

The `users.toml` file contains sensitive credentials. Restrict its permissions:

```bash
chmod 600 ~/.config/jman/users.toml
```

The API will log a warning at startup if permissions are more open than `0600`.

### 5. TOTP (Optional)

If a user has a `totpSecret` configured, they must provide a valid TOTP code at login. The secret should be a base32-encoded string compatible with standard authenticator apps (Google Authenticator, Authy, etc.).

If the `totpSecret` field is empty or omitted, TOTP is not required for that user.

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
- `GET /api/servers` — Returns cached SpinupWP servers.
- `GET /api/sites` — Returns cached SpinupWP sites.
- `GET /api/vulns?plugin=<slug>` — Returns cached vulnerability data for a specific plugin.

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

## Security Notes

- The API does not handle TLS directly. It should be run behind a reverse proxy (nginx, Caddy, etc.) that terminates TLS.
- Passwords are never stored or logged in plaintext — only bcrypt hashes.
- Error messages for wrong username vs. wrong password are intentionally identical to prevent user enumeration.
- JWT tokens are signed with HS256 (HMAC-SHA256) using the `jwtSecret` from `users.toml`.
- Tokens are stateless and cannot be individually revoked. The configurable token lifetime (default: 24 hours) limits exposure.

_Note: The API does not fetch new data. You must use the `jman` CLI to populate and update the cache._
