# jman-api REST Specification

This document provides a comprehensive technical specification for the `jman-api` REST service. It is designed to be used as a reference for implementing client libraries or frontend applications.

## General Information

- **Base URL**: `http://<host>:<port>/api`
- **Content-Type**: `application/json`
- **Authentication**: JWT Bearer Token required for all protected endpoints.
- **User Levels**:
  - `basic`: Read-only access to most data.
  - `edit`: Read/Write access to database records (Organizations, Assets, etc.).
  - `execute`: Execution of maintenance commands on sites.
  - `admin`: Full system access, including user management.
- **Password Strength**:
  - Enforced using an entropy-based calculation: `poolSize ^ length`.
  - Required minimum variations: 200,000,000,000,000.
  - Pool sizes: Lowercase (26), Uppercase (26), Numbers (10), Special characters (16).
- **Date Format**: ISO 8601 / RFC 3339 (`YYYY-MM-DDTHH:MM:SSZ`)

---

## Authentication

### Login

`POST /auth/login`

Authenticates a user and returns a short-lived access JWT. Also sets the `jman_refresh` refresh token cookie (`HttpOnly`, `Secure`, `SameSite=Strict`, `Path=/api/auth`).

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `username` | string | Yes | |
| `password` | string | Yes | |
| `totp` | string | No | Required if user has TOTP enabled |

**Success Response (200 OK)**

```json
{
	"token": "string",
	"expiresAt": "datetime",
	"user": {
		"username": "string",
		"displayName": "string",
		"level": "string"
	}
}
```

### Token Refresh

`POST /auth/refresh` (Public, authenticated by the `jman_refresh` cookie)

Exchanges the refresh token cookie for a new access JWT and rotates the cookie. Reusing a rotated refresh token after a 30-second grace period revokes the session. Returns `401` if the session is missing, expired or revoked.

**Success Response (200 OK)**

```json
{
	"token": "string",
	"expiresAt": "datetime",
	"user": {
		"username": "string",
		"displayName": "string",
		"level": "string"
	}
}
```

### Logout

`POST /auth/logout` (Public)

Revokes the session in the `jman_refresh` cookie and clears the cookie. Returns `204 No Content`.

---

## User Management

### List All Users

`GET /users` (Protected: `basic`)

Returns a list of all users in the system. To prevent data leakage, sensitive fields like `level` and `has2FA` are only returned for users with the **`admin`** level.

**Response (200 OK - Admin)**

```json
[
	{
		"username": "admin",
		"displayName": "Administrator",
		"level": "admin",
		"has2FA": true
	}
]
```

**Response (200 OK - Basic)**

```json
[
	{
		"username": "admin",
		"displayName": "Administrator"
	}
]
```

### Create User

`POST /users` (Protected: `admin`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `username` | string | Yes | |
| `password` | string | Yes | Must meet entropy requirements |
| `displayName` | string | Yes | |
| `level` | string | No | `basic`, `edit`, `admin`, or `execute` (default: `basic`) |

### Update User

`PATCH /users/{username}` (Protected: `admin`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `displayName` | string | No | |
| `level` | string | No | |
| `password` | string | No | Must meet entropy requirements |

### Delete User

`DELETE /users/{username}` (Protected: `admin`)

Deletes a user. Cannot delete self or the last administrator.

---

## Task Management

Tasks represent units of work or reminders and can be linked to Sites, Servers, Organizations, or Plugins.

### List Tasks

`GET /tasks` (Protected: `basic`)

Returns a list of tasks matching the provided filters.

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `status` | string | Filter by status (`pending`, `in_progress`, `completed`, `skipped`, `overdue`) |
| `priority` | string | Filter by priority (`low`, `medium`, `high`) |
| `assigned_to` | string | Filter by assigned username |
| `completed_by` | string | Filter by user who completed the task |
| `site_id` | integer | Filter by linked Site ID |
| `organization_id` | integer | Filter by linked Organization ID |
| `server_id` | integer | Filter by linked Server ID |
| `search` | string | Search in title or description |

**Response (200 OK)**

```json
[
	{
		"id": 1,
		"type": "one-time",
		"status": "completed",
		"priority": "high",
		"title": "Security Vulnerabilities - example.com",
		"description": "...",
		"site_id": 123,
		"assigned_to": "niklas",
		"metadata": "{\"vuln_uuids\":[\"...\"]}",
		"due_date": "2024-03-20T12:00:00Z",
		"reminder_date": "2024-03-13T12:00:00Z",
		"created_at": "2024-03-13T12:00:00Z",
		"completed_at": "2024-03-14T09:00:00Z",
		"updated_at": "2024-03-14T09:00:00Z",
		"created_by": "system",
		"completed_by": "niklas"
	}
]
```

### Get Task

`GET /tasks/{id}` (Protected: `basic`)

### Create Task

`POST /tasks` (Protected: `edit`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `type` | string | No | `one-time` (default), `repeating`, `dynamic` |
| `status` | string | No | Default: `pending` |
| `priority` | string | No | Default: `medium` |
| `title` | string | Yes | |
| `description` | string | No | |
| `site_id` | integer | No | |
| `server_id` | integer | No | |
| `organization_id` | integer | No | |
| `plugin_slug` | string | No | |
| `assigned_to` | string | No | Username |
| `interval` | string | No | e.g., `30d`, `1w`, `1m`, `1y` (required for repeating/dynamic) |
| `due_date` | datetime | No | |
| `reminder_date` | datetime | No | |
| `metadata` | string | No | JSON string |

### Update Task

`PATCH /tasks/{id}` (Protected: `edit`)

Updates specific fields of a task.

### Complete Task

`POST /tasks/{id}/complete` (Protected: `edit`)

Marks a task as completed. If the task is `repeating` or `dynamic`, a new task instance is automatically generated based on the `interval`.

### Delete Task

`DELETE /tasks/{id}` (Protected: `edit`)

---

## User Self-Service

These endpoints allow any authenticated user to manage their own account.

### Get Profile

`GET /user/profile` (Protected: `basic`)

Returns the profile information for the logged-in user.

**Response (200 OK)**

```json
{
	"username": "string",
	"displayName": "string",
	"level": "string",
	"has2FA": boolean
}
```

### Update Profile

`PATCH /user/profile` (Protected: `basic`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `displayName` | string | No | |

### Change Password

`POST /user/password` (Protected: `basic`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `currentPassword` | string | Yes | |
| `newPassword` | string | Yes | Must meet entropy requirements |

### 2FA Setup

`POST /user/2fa/setup` (Protected: `basic`)

Generates a temporary TOTP secret and QR code URI.

**Response (200 OK)**

```json
{
	"secret": "string",
	"uri": "otpauth://..."
}
```

### 2FA Activation

`POST /user/2fa/activate` (Protected: `basic`)

Verifies a setup code and enables 2FA for the current user.

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `secret` | string | Yes | The secret from the setup step |
| `code` | string | Yes | 6-digit TOTP code |

### 2FA Deactivation

`POST /user/2fa/deactivate` (Protected: `basic`)

Disables 2FA for the current user. Requires a valid TOTP code.

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `code` | string | Yes | 6-digit TOTP code |

---

## Core Data (Read-Only)

These endpoints require at least **`basic`** level.

### List Servers / Sites / Plugins

`GET /servers` (Protected: `basic`)
`GET /sites` (Protected: `basic`)
`GET /plugins` (Protected: `basic`)
`GET /plugininfo` (Protected: `basic`)

### WordPress Users of Sites

These are the managed sites' WordPress users, not jman's own users (see User Management). Subscribers and WooCommerce customers aren't listed, only counted by role. At most 1000 users are kept per site (`truncated` is set when a site has more). Users are collected with the plugin data, by jman-agent or over SSH.

`GET /site-users` (Protected: `basic`)

Every site's users, ordered by site and user ID, with one summary per site whose users were read.

`post_count` is how many posts of any type and status (revisions and attachments included) the user authored, which is what deleting the user deletes unless their content is reassigned. It's absent when it isn't known, e.g. for users reported by an older jman-agent.

```json
{
  "users": [
    {
      "site_id": "…",
      "id": 1,
      "login": "admin",
      "email": "admin@example.com",
      "display_name": "Admin",
      "roles": "administrator",
      "registered": "2020-01-02 03:04:05",
      "post_count": 12
    }
  ],
  "summaries": [
    {
      "site_id": "…",
      "excluded_counts": { "subscriber": 1200, "customer": 0 },
      "updated_at": "2026-10-17T08:00:00Z"
    }
  ]
}
```

`GET /sites/{id}/users` (Protected: `basic`)

One site's users: `{"users": [...], "excluded_counts": {...}, "truncated": false, "updated_at": "…"}`. `updated_at` is empty if the site's users were never read.

#### Create a User

`POST /sites/{id}/users` (Protected: `execute`)

Queues a background job (see Plugin Update Operations) that creates a WordPress user on the site, and returns it with `202 Accepted`. `role` defaults to `administrator`.

```json
{ "login": "jco", "email": "jco@example.com", "role": "administrator", "password": "at least 12 characters" }
```

The password is only held in jman-api's memory until the job runs. It's never stored, logged or returned, so a job queued when jman-api restarts fails, and the user has to be created again. The site's user list is refreshed after the job.

- `400`: the login (up to 60 letters, digits, spaces and `_ . - @`), email, role or password (at least 12 characters, no control characters) is invalid.
- `404`: the site isn't reachable over WP-CLI.
- `409`: the site's user list already has the login or email. The job also fails if WordPress finds one.

#### Delete a User

`DELETE /sites/{id}/users/{userId}?reassign=<login or email>` (Protected: `execute`)

Queues a background job that deletes a user from the site's user list, and returns it with `202 Accepted`. `reassign` names the user who gets the deleted user's content, by exact login or email. Without it, a user who has posts isn't deleted. Before deleting, the job checks on the site that:
- the user still has the listed login and email;
- it has no posts, or the reassign target matches exactly one other user.

The site's user list is refreshed after the job.

- `400`: the user ID is invalid, or `reassign` is the user being deleted.
- `404`: the user isn't in the site's user list, or the site isn't reachable over WP-CLI.
- `409`: the site is a multisite (not supported), or the user has posts and no `reassign` is given.

---

### File Scans

jman-agent scans each site's files about once an hour (see README_AGENT.md) and reports what it flags. Basic level.

`GET /api/scan/findings` returns findings, most recently seen first. `?site_id=` limits them to one site, `?limit=` caps how many are returned (default 200, at most 1000).

```json
{
  "findings": [
    {
      "id": 12,
      "site_id": "…",
      "engine": "builtin",
      "rule": "uploads-php",
      "severity": "high",
      "path": "wp-content/uploads/2026/10/x.php",
      "sha256": "…",
      "snippet": "<?php eval($_POST['x']);",
      "detail": "new PHP file in uploads",
      "rule_set_version": "1",
      "first_seen_at": "2026-10-23T14:00:00Z",
      "last_seen_at": "2026-10-23T14:00:00Z",
      "incident_id": 41,
      "status": "open"
    }
  ]
}
```

A finding is identified by its site, rule, path and `sha256`, so reporting the same one again only updates `last_seen_at`. `engine` is `api` for findings jman-api raises itself: `baseline-reset` (low) when an agent lost its baseline for a site or compared against one other than it last reported.

Each finding also has a `status`: `open`, `closed` (dismissed, or its incident was closed), `allowlisted` (its hash is on the allowlist) or `acknowledged` (its path is acknowledged for the site), with `status_by` and `status_at`, and the `incident_id` of the security incident it's in. A closed finding reported again is open again.

`new-admin` (high, engine `api`) is jman-api's own: a user who became an administrator of a site (new, or promoted) since its users were last collected, that jman didn't create (no `created` user entry in the site's update ledger, and no user create job running for that login). Its path is `users/<login>` and its snippet describes the account. A site's first collected users are a baseline. It's medium instead of high when the account's email is in one of the `trustedAdminDomains` (or a subdomain), as for the agency's own staff. It can be acknowledged by path like a stray file; allowlisting doesn't apply.

`new-app-password` (engine `api`) is an application password that appeared on a site since its users were last collected: high on an administrator, medium on anyone else. jman never creates them. Application passwords log in to the REST API without two-factor authentication, and one on a real administrator survives deleting rogue accounts. The path is `users/<login>/app-passwords/<uuid>`; the detail says when its creation date is older than jman's previous look at the site's users, which means it was written by hand. The first users collected with their application passwords are a baseline.

`cleared_at` and `cleared_detail` are set when the agent reports that the finding no longer holds: the file was `removed`, `replaced`, is `no longer flagged` (still on disk, such as a foreign-owned file given back to the site's owner), `now owned by the site's owner` or `now readable`. Clearing doesn't change the status or close anything; finding the file again unsets it. An incident's summary counts its cleared findings, such as "2 open findings: 2 uploads-php (1 cleared)".

`GET /api/scan/findings` takes these filters as well: `?incident_id=`, `?status=`, `?sha256=` and `?unassigned=1` (findings without an incident).

`GET /api/scan/sites` returns the latest scan of every scanned site: `site_id`, `scanned_at`, `rule_set_version`, `entries` (files in the baseline), `omitted`, `duration_ms`, `error` (set when the last scan failed) and `rebaselined_at`.

#### Security incidents

Findings of medium severity and above are grouped into one security incident per site (`GET /api/incidents?type=security`). Low findings stay out of incidents; the web UI lists them as informational. jman-api never resolves a security incident itself: a person closes it, which closes its open findings. A finding reported again goes back into the incident it was in, reopening it, and so does a new finding whose file hash was in a closed incident of the same site. Other findings join the site's active security incident, or open a new one. Findings stored before security incidents existed are assigned on startup, without alerts.

New findings alert by severity: `scanSlackSeverity` (default `high`) and above in Slack (`slackSecurityChannel`, or `slackChannel`), `scanPagerDutySeverity` (default `critical`) and above in PagerDuty, keyed `jman-scan:incident:<id>`. Acknowledging or closing the incident resolves its PagerDuty alert.

#### Triage (execute level)

`POST /api/scan/allowlist` with `{"finding_id": 12, "reason": "…", "close_matching": false}` trusts the finding's file hash on every site, and marks the finding allowlisted. With `close_matching`, every open finding with that hash is marked too. Findings of an allowlisted hash are marked when they're reported, and don't open incidents. Each entry records the reason, who added it and when, and the site, rule, path and snippet of the finding it came from. `GET /api/scan/allowlist` (basic level; `?include_removed=1` for removed entries) lists them; `DELETE /api/scan/allowlist/{id}` removes one, keeping it for the record with who removed it and when.

`POST /api/scan/acks` with `{"finding_ids": [12, 13], "reason": "…"}` acknowledges the paths of `stray-file`, `foreign-owner` and `unreadable` findings on their sites: those rules don't flag the path there again, whatever the file holds. `GET /api/scan/acks` (basic level; `?site_id=`, `?include_removed=1`) and `DELETE /api/scan/acks/{id}` work as for the allowlist.

`POST /api/scan/findings/close` with `{"finding_ids": [12]}` dismisses findings.

`POST /api/scan/findings/redact` with `{"finding_ids": [12]}` removes the findings' `snippet` and `detail`, whatever their status, and sets `redacted_at` and `redacted_by`. They quote the files, which can hold personal data (a security plugin's log of failed logins, say); the rule, path and hash stay, so the history and reopening by hash still work. A finding reported again gets a fresh snippet. jman-api also removes them daily from findings closed, allowlisted or acknowledged longer ago than `scanSnippetRetentionDays` (`redacted_by` is then `jman-api`); open findings keep theirs.

Each of these closes the incidents it leaves without open findings.

## Reports (Read-Only)

Backend-defined tabular reports. The frontend renders the returned columns/rows
as a table and generates CSV export client-side — there is no separate export
endpoint.

### List Reports

`GET /reports` (Protected: `basic`)

Returns metadata for every registered report, including its input parameters.

```json
[
	{
		"id": "traffic",
		"name": "Traffic Analytics",
		"description": "Total visitor traffic per site for the selected date range.",
		"params": [
			{ "key": "range", "type": "daterange", "label": "Date range", "required": false }
		]
	},
	{
		"id": "asset-billing",
		"name": "Asset & Billing Ledger",
		"description": "Payments recorded against billable organization assets for the selected date range.",
		"params": [
			{ "key": "range", "type": "daterange", "label": "Date range", "required": false }
		]
	},
	{
		"id": "upcoming-billing",
		"name": "Upcoming Asset Billing",
		"description": "Active organization assets due to be billed by the selected date, including any already overdue.",
		"params": [
			{ "key": "end", "type": "enddate", "label": "Show billing due before", "required": false }
		]
	}
]
```

### Run a Report

`GET /reports/{id}/run` (Protected: `basic`)

Query parameters depend on the report's `params`:
- `daterange` takes `start`/`end` as `YYYY-MM-DD` (both optional — default to
  the trailing 30 days).
- `enddate` takes a single `end` as `YYYY-MM-DD` (optional — defaults to one
  month from today). There is no lower bound, so results already past due
  are always included alongside anything up to `end`.

Returns `400` for an unparseable or out-of-bounds range, `404` for an unknown
report ID.

`GET /reports/traffic/run?start=2026-01-01&end=2026-01-31`

```json
{
	"columns": [
		{ "key": "site", "label": "Site", "type": "text" },
		{ "key": "requests_total", "label": "Total Requests", "type": "number" },
		{ "key": "requests_human", "label": "Human Requests", "type": "number" },
		{ "key": "requests_bot", "label": "Bot Requests", "type": "number" },
		{ "key": "unique_visitors", "label": "Unique Visitors", "type": "number" }
	],
	"rows": [
		{ "site": "example.com", "requests_total": 3600, "requests_human": 2900, "requests_bot": 700, "unique_visitors": 950 }
	]
}
```

`GET /reports/asset-billing/run?start=2026-01-01&end=2026-01-31`

```json
{
	"columns": [
		{ "key": "organization", "label": "Organization", "type": "text" },
		{ "key": "identifier", "label": "Identifier", "type": "text" },
		{ "key": "asset_name", "label": "Asset", "type": "text" },
		{ "key": "asset_type", "label": "Type", "type": "text" },
		{ "key": "billing_freq", "label": "Billing Frequency", "type": "text" },
		{ "key": "status", "label": "Status", "type": "text" },
		{ "key": "amount", "label": "Amount", "type": "currency" },
		{ "key": "payment_date", "label": "Payment Date", "type": "date" },
		{ "key": "info", "label": "Info", "type": "text" }
	],
	"rows": [
		{ "organization": "Acme Inc", "identifier": "acme.example.com", "asset_name": "Yoast SEO", "asset_type": "Plugin", "billing_freq": "Monthly", "status": "active", "amount": 2000, "payment_date": "2026-01-15", "info": "January renewal" }
	]
}
```

`GET /reports/upcoming-billing/run?end=2026-02-28`

```json
{
	"columns": [
		{ "key": "organization", "label": "Organization", "type": "text" },
		{ "key": "identifier", "label": "Identifier", "type": "text" },
		{ "key": "asset_name", "label": "Asset", "type": "text" },
		{ "key": "asset_type", "label": "Type", "type": "text" },
		{ "key": "billing_freq", "label": "Billing Frequency", "type": "text" },
		{ "key": "price", "label": "Price", "type": "currency" },
		{ "key": "next_billing", "label": "Next Billing", "type": "date" },
		{ "key": "status", "label": "Status", "type": "text" }
	],
	"rows": [
		{ "organization": "Acme Inc", "identifier": "acme.example.com", "asset_name": "Yoast SEO", "asset_type": "Plugin", "billing_freq": "Monthly", "price": 2000, "next_billing": "2026-01-15", "status": "active" }
	]
}
```

---

## Organization Management (Read/Write)

### Organizations

`GET /organizations` (Protected: `basic`)
`GET /organizations/{id}` (Protected: `basic`)
`POST /organizations` (Protected: `edit`)
`PATCH /organizations/{id}` (Protected: `edit`)
`DELETE /organizations/{id}` (Protected: `edit`)

### Contacts

`GET /organizations/{id}/contacts` (Protected: `basic`)
`POST /contacts` (Protected: `edit`)
`PATCH /contacts/{id}` (Protected: `edit`)
`DELETE /contacts/{id}` (Protected: `edit`)

---

## Asset & Monitoring Management

### Asset Templates

`GET /assets` (Protected: `basic`)
`POST /assets` (Protected: `edit`)
`PATCH /assets/{id}` (Protected: `edit`)
`DELETE /assets/{id}` (Protected: `edit`)

### Organization Assets & Payments

`GET /organization-assets` (Protected: `basic`)
`POST /organizations/{id}/assets` (Protected: `edit`)
`POST /organization-assets/{id}/payments` (Protected: `edit`)
`DELETE /asset-payments/{id}` (Protected: `edit`)

### Unified Ignore List

`GET /ignore` (Protected: `basic`)
`POST /ignore` (Protected: `edit`)
`PATCH /ignore/{id}` (Protected: `edit`)
`DELETE /ignore/{id}` (Protected: `edit`)

**Ignore Entry Object**

```json
{
	"id": 1,
	"type": "site",
	"target": "123",
	"reason": "Maintenance",
	"negated_site_ids": [456],
	"use_for_vuln": true,
	"created_at": "datetime",
	"created_by": "username",
	"updated_at": "datetime",
	"updated_by": "username"
}
```

### Monitoring

`GET /monitor/history` (Protected: `basic`)
`GET /monitor/status` (Protected: `basic`)
`GET /monitor/checks` (Protected: `basic`): every monitor check, with the `domain` and `url` it checks and the names of its secret headers. Optional `?site_id=` filter.
`GET /monitor/checks/{id}` (Protected: `basic`)
`POST /monitor/checks` (Protected: `edit`): creates a check of its own (not a site's default check). Body below.
`PATCH /monitor/checks/{id}` (Protected: `edit`): edits a check; absent fields are kept. Changing anything the check probes (host, path, expectation, assertion, cache bypass) resolves its active incident with an "edited" Slack message and restarts its state. `site_id` can't change; a default check's host is its site's domain.
`POST /monitor/checks/{id}/reset` (Protected: `edit`): puts a site's default check back to `/`, expecting HTML, no assertion, global cache bypass.
`DELETE /monitor/checks/{id}` (Protected: `edit`): deletes a check with its history, incidents and secrets. A site's default check can only be deleted once orphaned.
`PUT /monitor/checks/{id}/secrets/{header}` (Protected: `edit`): body `{"value": "..."}`. Sets a header sent with the check's requests. Values are never returned; headers are dropped on redirects to another host.
`DELETE /monitor/checks/{id}/secrets/{header}` (Protected: `edit`)
`POST /monitor/checks/test` (Protected: `edit`): runs a check once and returns `{"up", "message", "status_code", "url", "duration_ms"}`, recording nothing. Takes the check body; with `"id"` it tests that stored check (with its secrets) with the body's fields applied on top. `"secret_headers": {"Name": "value"}` adds headers for this run only.

**Check body**

```json
{
	"name": "MCP health",
	"host": "mcp.example.com",
	"path": "/healthz",
	"expect": "json",
	"assert_path": "ok",
	"assert_op": "equals",
	"assert_value": true,
	"cache_bypass": null
}
```

- `expect`: `html` (default; the body must look like HTML) or `json` (the body must parse as JSON).
- One optional assertion on a JSON body: `assert_path` is dot-separated (`items.0.state`); `assert_op` is `equals` (with `assert_value`, compared as JSON, so `true` and `"true"` differ) or `exists`.
- `cache_bypass`: `true`/`false` override the global `monitorCacheBypass` setting; `null` follows it.
- A 2xx response is always required.

**Heartbeat checks (check-ins)**

A check with `"type": "heartbeat"` is told a job ran rather than probing anything. Create it with:

```json
{ "type": "heartbeat", "name": "Nightly backup", "interval_seconds": 86400, "grace_seconds": 3600, "fail_threshold": 1 }
```

- It goes down when no check-in finished within `interval_seconds` + `grace_seconds` of the last one, or after `fail_threshold` failed check-ins in a row (default 1). The next OK check-in brings it back up.
- A new check waits for its first check-in before it can alert.
- The create response includes `token`, the check-in token, shown this once. Other responses only carry `token_prefix` and `token_created_at`.
- `POST /monitor/checks/{id}/token` (Protected: `edit`) issues a new token, replacing the old one, and returns it once.
- `name` is required. Host, path, expectation, assertion and cache bypass don't apply, and `type` can't be changed later.

### Check-ins (public)

`POST /checkin` or `POST /checkin/ok`: the job finished OK.
`POST /checkin/fail`: the job finished with an error. A plain-text body (up to 500 characters) becomes the incident and history message.
`POST /checkin/start`: the job started. Recorded as `last_started_at` in the check's status; it doesn't move the deadline.

Authenticate with `Authorization: Bearer <token>`; the token names the check. Responses are `{"ok": true, "accepted": true}`, with `accepted: false` for a paused or orphaned check. An invalid token returns 401, and repeated invalid tokens from one client return 429 for 15 minutes.

```sh
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" https://jman.example.com/api/checkin/start
run-backup && s=ok || s=fail
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" https://jman.example.com/api/checkin/$s --data "exit $?"
```
`POST /monitor/checks/{id}/pause` (Protected: `edit`): pauses a check. An active incident is resolved, its PagerDuty alert resolved, and a "paused" message posted to Slack. Optional body `{"reason": "..."}`.
`POST /monitor/checks/{id}/resume` (Protected: `edit`)
`POST /incidents/{id}/pause` (Protected: `edit`): pauses the incident's check, resolving the incident.

Incidents have a `type`: `uptime`, or `security` for a site's file scan findings (see File Scans), which also carry `site_id`, `severity` and, when it's critical because its findings correlate, `escalation` (why). `GET /incidents` returns uptime incidents unless `?type=security` or `?type=all`, and `active_by_type` splits `active_count` by type.

Monitor history, statuses and incidents carry the `check_id` they belong to. Every site has a default check (path `/`, expecting HTML), created automatically; a check whose site is gone is marked `orphaned_at` and no longer runs (its active incident is resolved, as when pausing). PagerDuty alerts are keyed by check ID (`jman-monitor:check:<id>`).

---

## Settings Management

These endpoints allow users to store arbitrary key/value pairs for frontend configuration or personal preferences. Settings are private to each user.

### List All Settings

`GET /settings` (Protected: `basic`)

Returns all settings for the authenticated user.

**Response (200 OK)**

```json
[
	{
		"user_id": "username",
		"key": "theme",
		"value": { "dark": true },
		"created_at": "datetime",
		"updated_at": "datetime"
	}
]
```

### Get Setting

`GET /settings/{key}` (Protected: `basic`)

Returns a specific setting by key.

**Response (200 OK)**

```json
{
	"user_id": "username",
	"key": "theme",
	"value": { "dark": true },
	"created_at": "datetime",
	"updated_at": "datetime"
}
```

### Create or Replace Setting

`POST /settings/{key}` (Protected: `basic`)

Creates a new setting or completely replaces an existing one.

**Request Body**
Any valid JSON value.

### Merge Update Setting

`PATCH /settings/{key}` (Protected: `basic`)

Merges the provided JSON object with the existing setting. If both the current value and the new value are JSON objects (maps), they are merged. Otherwise, the value is replaced. Returns `404 Not Found` if the setting does not exist.

**Request Body**
Any valid JSON value.

### Delete Setting

`DELETE /settings/{key}` (Protected: `basic`)

Removes the setting with the specified key.

### Vulnerability Data

`GET /vulns` (Protected: `basic`)

Returns vulnerability reports for managed plugins.

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `plugin` | string | Filter by plugin slug. Returns detailed plugin metadata and all vulnerabilities. |

**Success Response (200 OK - No plugin parameter)**

```json
[
	{
		"plugin": "akismet",
		"slug": "akismet",
		"plugin_name": "Akismet Anti-Spam",
		"suppressed": false,
		"vulnerabilities": [
			{
				"uuid": "...",
				"name": "...",
				"impact": { "cvss": { "score": "7.5" } },
				"suppressed": false,
				"sites": [
					{
						"site_id": 123,
						"site_name": "example.com",
						"version": "5.0.0",
						"suppressed": false
					}
				]
			}
		]
	}
]
```

**Success Response (200 OK - With plugin parameter)**

Returns a single plugin report. Note that if active vulnerabilities are found, the structure matches a single item from the list above. If no active vulnerabilities are found after filtering, it returns the base plugin metadata.

```json
{
	"plugin": "akismet",
	"slug": "akismet",
	"plugin_name": "Akismet Anti-Spam",
	"suppressed": false,
	"vulnerabilities": [...]
}
```

**Notes on Suppression**

- Vulnerabilities ignored by their specific **UUID** are completely excluded from the response.
- If a **Plugin** is ignored, the root `"suppressed"` flag will be `true`.
- If a **Site** or **Server** is ignored, affected sites will be marked with `"suppressed": true`.
- A vulnerability is marked `"suppressed": true` if either the plugin is ignored or **all** affected sites are suppressed.

**Notes on Backport Rules**

- Sites whose installed version is covered by a [backport rule](#vulnerability-backport-rules) are not affected: they are listed in `"patched_sites"` instead of `"sites"`.
- A vulnerability whose affected sites are all patched is excluded from the response, so `"patched_sites"` only appears while some site is still affected.
- In `GET /vulns/core`, a vulnerability is excluded from a core version's report if a backport rule covers that version.

### Vulnerability Exposures

An exposure is one site running a vulnerable version of a plugin (or of WordPress core). jman-api syncs them on the refresh scheduler's slow tick: an exposure opens when a site runs an affected version and resolves when it no longer does (it updated, removed the plugin, or an ignore entry or backport rule now covers it). See `docs/vuln-workflow.md`.

`GET /vulns/exposures` (Protected: `basic`)

Returns exposures grouped by plugin, or by core version.

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `status` | string | `open` (default) or `resolved`. |
| `days` | int | For `resolved`: how many days back to list, 1–365 (default 30). |

**Response (200 OK)**

```json
[
	{
		"kind": "plugin",
		"slug": "gigpress",
		"name": "GigPress",
		"closed": true,
		"closed_reason": "security-issue",
		"closed_date": "2022-12-27",
		"last_updated": "2021-12-03T19:40:00Z",
		"max_cvss": 7.2,
		"fix_available_count": 1,
		"no_fix_count": 1,
		"overdue_count": 1,
		"vulnerabilities": [
			{ "uuid": "...", "name": "GigPress <= 2.3.10 - SQL Injection", "cvss": 7.2, "fixed_in": "2.3.11", "unfixed": false, "link": "https://..." }
		],
		"sites": [
			{
				"id": 12,
				"kind": "plugin",
				"slug": "gigpress",
				"site_id": "uuid",
				"site_name": "example.com",
				"version": "2.3.10",
				"vuln_uuids": ["..."],
				"max_cvss": 7.2,
				"fix_state": "fix_available",
				"fixed_in": "2.3.11",
				"fix_available_since": "datetime",
				"due_at": "datetime",
				"overdue": true,
				"first_seen_at": "datetime",
				"last_seen_at": "datetime",
				"resolved_at": null
			}
		]
	}
]
```

- Core groups have `kind` `core`, `slug` `wordpress` and the core `version`; there is one group per installed core version.
- `fix_state` is `fix_available` when every applicable vulnerability has a fixed version (for core: the site has a core update), with the lowest one in `fixed_in`. Otherwise it is `no_fix`, and `possible_fix_version` may name a newer available version outside every affected range, which may or may not fix them.
- A fix-available exposure is due `vulnDue*Days` after `fix_available_since`, by its highest CVSS (see `README_API.md`), and `overdue` after that. No-fix exposures are never due.
- `closed*` and `last_updated` come from the plugin's wpvulnerability.net record.
- Groups with overdue sites come first, then by highest CVSS; overdue sites come first in a group.

- An open exposure with a mute has it in `mute` (see below). A muted site never counts toward `overdue_count`, and the group's `muted_count` counts its muted sites.

`GET /vulns/exposures/summary` (Protected: `basic`)

Counts the open exposures, for badges. `overdue` and `no_fix` leave muted exposures out; `needs_review` counts mutes past their review date.

**Response (200 OK)**

```json
{ "groups": 14, "exposures": 52, "overdue": 3, "no_fix": 9, "muted": 4, "needs_review": 1 }
```

### Vulnerability Mutes

A mute accepts the risk of one plugin (or core) exposure on one site, with a reason: the exposure doesn't count as overdue. jman-api lifts a mute on its slow tick when:

- a vulnerability applies that wasn't there when it was muted or last confirmed,
- it was muted while there was no fix, and a fix (`fix_available`) or a new `possible_fix_version` shows up,
- the site is no longer affected (and wasn't resolved within the last 24 hours, so a site missing from one refresh keeps its mute).

90 days after it was made or last confirmed, an active mute becomes `needs_review`: it still keeps the exposure from counting as overdue until someone confirms it again or unmutes it. Lifted mutes are kept.

`GET /vulns/mutes` (Protected: `basic`)

Returns the open mutes (`active` and `needs_review`), soonest review first; with `?status=lifted`, the mutes lifted in the last `?days=` days (default 30, max 365), newest first.

**Mute Object**

```json
{
	"id": 3,
	"kind": "plugin",
	"slug": "gigpress",
	"site_id": "uuid",
	"reason": "Waiting for the client to test 2.3.11 on staging",
	"status": "active",
	"fix_state_at_mute": "fix_available",
	"vuln_uuids_at_mute": ["..."],
	"possible_fix_at_mute": "",
	"review_at": "datetime",
	"lifted_at": null,
	"lifted_reason": "",
	"created_at": "datetime",
	"created_by": "username",
	"updated_at": "datetime",
	"updated_by": "username",
	"site_name": "example.com",
	"name": "GigPress",
	"version": "2.3.10"
}
```

`site_name`, `name` and (for an open exposure) `version` are only set by this endpoint. `updated_by` is `system` when jman-api lifted the mute or put it up for review.

`POST /vulns/mutes` (Protected: `execute`)

Mutes the open exposure of a plugin (or core, slug `wordpress`) on a site.

**Request Body**

```json
{ "kind": "plugin", "slug": "gigpress", "site_id": "uuid", "reason": "..." }
```

Returns `201 Created` with the mute; `400` without a reason, `404` if the site has no open exposure for it, `409` if it is already muted.

`POST /vulns/mutes/{id}/confirm` (Protected: `execute`)

Keeps an open mute for another 90 days with a new `reason` (`{ "reason": "..." }`), recording the exposure as it is now. Returns the mute; `409` if the site is no longer affected.

`DELETE /vulns/mutes/{id}` (Protected: `execute`)

Lifts an open mute (`lifted_reason` "Unmuted by <user>"). Returns `204 No Content`; `404` if it isn't open.

### Vulnerability Backport Rules

`GET /vuln-backports` (Protected: `basic`)
`POST /vuln-backports` (Protected: `edit`)
`DELETE /vuln-backports/{id}` (Protected: `edit`)

A backport rule records that a vulnerability was fixed in a release the vulnerability database doesn't list: installed versions within `branch` that are at least `min_version` are not affected. A vulnerability can have one rule per branch. Rules can't be edited; delete and recreate them instead.

**Backport Rule Object**

```json
{
	"id": 1,
	"type": "plugin",
	"slug": "woocommerce",
	"vuln_uuid": "...",
	"branch": "11.1",
	"min_version": "11.1.2",
	"reason": "Fix backported in 11.1.2",
	"created_at": "datetime",
	"created_by": "username",
	"updated_at": "datetime",
	"updated_by": "username",
	"matched_sites": 3
}
```

- `type` is `plugin` or `core`; `slug` is required for plugin rules and empty for core rules.
- `matched_sites` is the number of sites whose installed version the rule currently matches.

**Create Request**

`type`, `slug`, `vuln_uuid`, `branch`, `min_version` and `reason` as above. `branch` and `min_version` are dotted numbers, and `min_version` must lie within `branch` and have more segments than it.

**Create Response (201 Created)**

```json
{
	"rule": { "...": "Backport Rule Object" },
	"warnings": ["Version 11.2.0 is outside the vulnerability's affected range, so the rule has no effect."]
}
```

Warnings flag rules that can't be verified against cached vulnerability data or look like they'd have no effect; the rule is saved anyway. A malformed rule returns `400`, and a second rule for the same vulnerability and branch returns `409`.

---

## Plugin Update Operations

Plugin and core updates run as background jobs (`POST /plugin-update-jobs`, `POST /sites/{id}/core-update`; progress via `GET /update-jobs`). Queueing them requires the **`execute`** level.

Every job has `id`, `kind`, `site_id`, `status` (`queued`, `running`, `done`, `failed`, `interrupted`), `results`, `error`, `created_by` and the timestamps. What the job changes is in `payload`, whose shape depends on `kind`:

| `kind` | `payload` |
| :--- | :--- |
| `plugins`, `activate`, `deactivate`, `delete`, `uninstall` | `{"plugins": [{"name": "akismet", "old_version": "5.0"}], "allow_major": false}` |
| `core` | `{"target": "minor", "allow_major": false}` |
| `install` | `{"source": "akismet", "activate": true}` |
| `user_create` | `{"login": "jco", "email": "jco@example.com", "role": "administrator"}` |
| `user_delete` | `{"user_id": 7, "login": "mainwp", "email": "mainwp@example.com", "reassign": "admin"}` |

### Get Available Plugin Updates for a Site

`GET /sites/{id}/plugin-updates` (Protected: `execute`)

Calls WP-CLI live to fetch the current list of plugins that have updates available on the specified site.

**Path Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `id` | integer | Site ID |

**Response (200 OK)**

```json
[
	{
		"site_id": 123,
		"name": "akismet",
		"status": "active",
		"version": "5.0.0",
		"update": "5.1.0",
		"autoUpdate": false
	}
]
```

Returns an empty array if no updates are available.

### Update Locks

Sites that may not freely update can be **update-locked**: a site lock (empty `plugin`) covers WordPress core and every plugin on the site, a plugin lock covers one plugin on one site. Locked items only get **fix releases**:

- Plugins are updated with `wp plugin update --patch`: the newest stable release with the same `major.minor` version (9.3.0 → 9.3.3, even when 9.4.1 is the latest), looked up on WordPress.org. Only plugins whose pending update package comes from `downloads.wordpress.org` are updated this way; others (premium plugins, plugins with their own updater) are held back.
- Core only gets `minor` updates (6.6.1 → 6.6.2).

Locks are checked when a job runs, not when it is queued. A held-back plugin's job result has status `"Skipped (locked)"`; if it is still vulnerable, its `note` names the version the vulnerability database records as the fix.

A bigger update needs `"allow_major": true`: accepted on `POST /plugin-update-jobs` only for a single job with a single plugin (so bulk updates can't bypass locks), and on `POST /sites/{id}/core-update` with `"target": "major"`. Without it, a major core update on a locked site is refused with `409 Conflict`. The lock stays in place after such an update.

Locks only restrict jman's own updates. WordPress's automatic updates are reported so the UI can warn about them: the site's `wp_flags.auto_update_core` (`"true"` means major core updates are automatic; `""` means unknown) and each plugin's `autoUpdate`.

#### List Update Locks

`GET /update-locks` (Protected: `basic`)

Locks are also included in each site of `GET /sites` as `update_locks`.

```json
[
	{
		"id": 3,
		"site_id": "6467c171-4e72-5d82-b430-fb50868b9bf1",
		"plugin": "woocommerce",
		"comment": "Custom checkout, test updates on staging first",
		"created_by": "niklas",
		"created_at": "2026-10-08T12:00:00Z"
	}
]
```

#### Lock a Site or Plugin

`POST /sites/{id}/update-locks` (Protected: `execute`)

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `plugin` | string | No | Plugin slug; empty or missing locks the whole site |
| `comment` | string | No | Why it is locked (up to 1000 characters) |

Locking something already locked replaces the lock's comment and author. Returns the lock (`201 Created`), or `404` if the site isn't reachable over WP-CLI.

#### Remove a Lock

`DELETE /update-locks/{id}` (Protected: `execute`)

Returns `204 No Content`, or `404` if the lock doesn't exist. Removing a site lock leaves the site's plugin locks in place.

---

## Error Handling

The API returns a standard error object for all non-2xx/3xx responses:

```json
{
	"error": "Descriptive error message"
}
```

### Common Status Codes

- `200 OK`: Success
- `201 Created`: Successfully created a record
- `204 No Content`: Successfully deleted a record
- `400 Bad Request`: Validation error or malformed JSON
- `401 Unauthorized`: Missing or invalid JWT token
- `403 Forbidden`: Insufficient user level (permissions error)
- `404 Not Found`: Record does not exist
- `409 Conflict`: Username already exists
- `429 Too Many Requests`: Login rate limit exceeded
- `500 Internal Server Error`: Server-side error
