# jman-api REST Specification

This document provides a comprehensive technical specification for the `jman-api` REST service. It is designed to be used as a reference for implementing client libraries or frontend applications.

## General Information

- **Base URL**: `http://<host>:<port>/api`
- **Content-Type**: `application/json`
- **Authentication**: JWT Bearer Token required for all protected endpoints.
- **User Levels**:
  - `basic`: Read-only access to most data.
  - `edit`: Read/Write access to database records (Organizations, Assets, etc.).
  - `execute`: Administrative access (User management, system commands).
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

## User Management (Admin)

These endpoints are restricted to users with the **`execute`** level.

### List All Users

`GET /users` (Protected: `execute`)

Returns a detailed list of all users in the system.

**Response (200 OK)**

```json
[
	{
		"username": "admin",
		"displayName": "Administrator",
		"level": "execute",
		"has2FA": true
	}
]
```

### Create User

`POST /users` (Protected: `execute`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `username` | string | Yes | |
| `password` | string | Yes | Must meet entropy requirements |
| `displayName` | string | Yes | |
| `level` | string | No | `basic`, `edit`, or `execute` (default: `basic`) |

### Update User

`PATCH /users/{username}` (Protected: `execute`)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `displayName` | string | No | |
| `level` | string | No | |

### Delete User

`DELETE /users/{username}` (Protected: `execute`)

Deletes a user. Cannot delete self or the last administrator.

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
| `displayName` | string | Yes | |

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

### Monitoring

`GET /monitor/history` (Protected: `basic`)
`GET /monitor/status` (Protected: `basic`)
`GET /monitor/ignored` (Protected: `basic`)
`POST /monitor/ignored` (Protected: `edit`)
`DELETE /monitor/ignored/{domain}` (Protected: `edit`)

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
- `400 Bad Request`: Validation error or malformed JSON
- `401 Unauthorized`: Missing or invalid JWT token
- `403 Forbidden`: Insufficient user level (permissions error)
- `404 Not Found`: Record does not exist
- `409 Conflict`: Username already exists
- `429 Too Many Requests`: Login rate limit exceeded
- `500 Internal Server Error`: Server-side error
