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

---

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
	"use_for_monitor": true,
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

### Vulnerability Task Settings

Unlike the per-user settings above, these two endpoints manage a single global setting (admin only).

`GET /vuln-settings` (Protected: `admin`)

Returns the configured default assignee for newly-created vulnerability tasks (see [Background Automation](TASK_SPECS.md)).

**Response (200 OK)**

```json
{ "defaultAssignee": "username" }
```

An empty `defaultAssignee` means vulnerability tasks are left unassigned.

`POST /vuln-settings` (Protected: `admin`)

Sets the default assignee. `defaultAssignee` must be an existing username, or empty to clear it. Only applies to newly-created vulnerability tasks — existing tasks are not reassigned.

**Request Body**

```json
{ "defaultAssignee": "username" }
```

---

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

---

## Plugin Update Operations

Plugin and core updates run as background jobs (`POST /plugin-update-jobs`, `POST /sites/{id}/core-update`; progress via `GET /update-jobs`). Queueing them requires the **`execute`** level.

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
