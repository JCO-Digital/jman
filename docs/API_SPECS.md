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

Authenticates a user and returns a JWT.

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

`POST /auth/refresh` (Protected: `basic`)

Exchanges a valid, non-expired JWT for a new one.

**Success Response (200 OK)**

```json
{
	"token": "string",
	"expiresAt": "datetime"
}
```

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

These endpoints allow a UI to perform plugin updates one at a time, so progress can be displayed per plugin. Both require the **`execute`** level.

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

### Update a Single Plugin on a Site

`POST /sites/{id}/plugin-updates` (Protected: `execute`)

Updates one plugin on the site. The plugin cache is refreshed in the background after the call returns.

**Path Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `id` | integer | Site ID |

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `plugin` | string | Yes | Plugin slug to update |

**Response (200 OK)**

```json
{
	"name": "akismet",
	"old_version": "5.0.0",
	"new_version": "5.1.0",
	"status": "Updated"
}
```

If the plugin is already up to date, `status` will be `"Up to date"` and versions will be identical.

**Response (500 Internal Server Error)**

```json
{
	"name": "akismet",
	"old_version": "5.0.0",
	"new_version": "5.0.0",
	"status": "failed"
}
```

If the update fails, `status` will be `"failed"` and versions will reflect the state before the attempt.

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
