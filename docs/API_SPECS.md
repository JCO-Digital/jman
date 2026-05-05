# jman-api REST Specification

This document provides a comprehensive technical specification for the `jman-api` REST service. It is designed to be used as a reference for implementing client libraries or frontend applications.

## General Information

- **Base URL**: `http://<host>:<port>/api`
- **Content-Type**: `application/json`
- **Authentication**: JWT Bearer Token required for all protected endpoints.
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
		"displayName": "string"
	}
}
```

### Token Refresh

`POST /auth/refresh` (Protected)

Exchanges a valid, non-expired JWT for a new one.

**Success Response (200 OK)**

```json
{
	"token": "string",
	"expiresAt": "datetime"
}
```

---

## Core Data (Read-Only)

These endpoints serve data cached from external sources (SpinupWP, etc.) via the `jman` CLI.

### List Servers

`GET /servers` (Protected)

Returns a list of all managed servers.

### List Sites

`GET /sites` (Protected)

Returns a list of all managed sites.

### List Plugins

`GET /plugins` (Protected)

Returns all WordPress plugins installed across all sites.

### List Plugin Info

`GET /plugininfo` (Protected)

Returns metadata for known plugins (author, homepage, version info).

---

## User Management (Read-Only)

### List Users

`GET /users` (Protected)

Returns a list of all users in the system to resolve display names for auditing.

**Response (200 OK)**

```json
[
	{
		"username": "admin",
		"displayName": "Administrator"
	},
	{
		"username": "niklas",
		"displayName": "Niklas"
	}
]
```

---

## Organization Management (Read/Write)

### List Organizations

`GET /organizations` (Protected)

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `search` | string | Filter by name or VAT number |

**Response (200 OK)**

```json
[
	{
		"id": 1,
		"name": "Acme Corp",
		"vat_number": "US1234567",
		"info": "Notes about organization",
		"created_at": "datetime",
		"created_by": "string",
		"updated_at": "datetime",
		"updated_by": "string"
	}
]
```

### Create Organization

`POST /organizations` (Protected)

**Request Body**
| Field | Type | Required |
| :--- | :--- | :--- |
| `name` | string | Yes |
| `vat_number` | string | No |
| `info` | string | No |

### Get/Update/Delete Organization

`GET /organizations/{id}` (Protected)
`PATCH /organizations/{id}` (Protected)
`DELETE /organizations/{id}` (Protected)

---

## Contact Management (Read/Write)

### List Organization Contacts

`GET /organizations/{id}/contacts` (Protected)

Returns all contact people associated with a specific organization.

### List Organization Sites

`GET /organizations/{id}/sites` (Protected)

Returns all sites linked to a specific organization.

### Create Contact

`POST /contacts` (Protected)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `organization_id` | int | Yes | |
| `name` | string | Yes | |
| `email` | string | No | |
| `phone` | string | No | |
| `type` | string | Yes | One of: `Main`, `Technical`, `Billing` |

### Update/Delete Contact

`PATCH /contacts/{id}`
`DELETE /contacts/{id}` (Protected)

---

## Asset Management (Read/Write)

### List Asset Templates

`GET /assets` (Protected)

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `search` | string | Filter by name, identifier or type |

### Create Asset Template

`POST /assets` (Protected)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `type` | string | Yes | `Plugin`, `Domain`, `Hosting Package`, `Service Package`, `General` |
| `identifier` | string | No | TLD for domains, slug for plugins |
| `name` | string | Yes | |
| `description` | string | No | |
| `default_price` | int | No | Default price in cents |
| `default_freq` | string | No | `Yearly`, `Quarterly`, `Monthly`, `One-time` |
| `active` | bool | No | Default: `true` |

### Get/Update/Delete Asset Template

`GET /assets/{id}` (Protected)
`PATCH /assets/{id}` (Protected)
`DELETE /assets/{id}` (Protected)

---

## Organization Asset Management

### List All Organization Assets

`GET /organization-assets` (Protected)

Returns a list of all linked assets across all organizations. This is useful for dashboard views and tracking upcoming renewals.

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `search` | string | Filter by identifier, organization name, or asset name |
| `status` | string | Filter by `active`, `cancelled`, or `paused` |
| `before` | datetime | Filter for assets with `next_billing` on or before this date |

**Response (200 OK)**

```json
[
	{
		"id": 1,
		"organization_id": 10,
		"organization_name": "Acme Corp",
		"asset_id": 5,
		"asset_name": ".fi Domain",
		"site_id": 20,
		"identifier": "acme.fi",
		"price": 1500,
		"billing_freq": "Yearly",
		"next_billing": "2024-12-31T00:00:00Z",
		"status": "active",
		"description": "Primary domain",
		"created_at": "datetime",
		"created_by": "string"
	}
]
```

### List Organization Assets

`GET /organizations/{id}/assets` (Protected)

### Link Asset to Organization

`POST /organizations/{id}/assets` (Protected)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `asset_id` | int | No | Template ID |
| `site_id` | int | No | Optional site link |
| `identifier` | string | No | Specific domain or product name |
| `price` | int | No | Price in cents (defaults to template) |
| `billing_freq` | string | No | Defaults to template |
| `next_billing` | datetime | No | |
| `status` | string | No | One of: `active`, `cancelled`, `paused`. Default: `active` |
| `description` | string | No | |

### Get/Update/Delete Organization Asset

`GET /organization-assets/{id}` (Protected)
`PATCH /organization-assets/{id}` (Protected)
`DELETE /organization-assets/{id}` (Protected)

### Asset Payment History

`GET /organization-assets/{id}/payments` (Protected)

`POST /organization-assets/{id}/payments` (Protected)

Recording a payment automatically advances the `next_billing` date of the linked asset based on its `billing_freq` (Yearly, Quarterly, or Monthly). If the frequency is `One-time`, the `next_billing` date is cleared.

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `amount` | int | Yes | Amount in cents |
| `payment_date` | datetime | No | Defaults to now |
| `info` | string | No | Description/Note |
| `next_billing` | datetime | No | Explicitly set the next billing date (overrides auto-advancement) |

`DELETE /asset-payments/{id}` (Protected)

---

## Site Linking

These endpoints manage the relationship between external site IDs and organization records.

### Get Organization for Site

`GET /sites/{site_id}/organization` (Protected)

Returns the `Organization` object linked to the specific Site ID.

### Link Site to Organization

`POST /sites/{site_id}/link` (Protected)

**Request Body**

```json
{ "organization_id": 123 }
```

### Unlink Site

`DELETE /sites/{site_id}/link` (Protected)

---

## Notes

Notes can be attached to either an `Organization` or a `Site`.

### List Notes

`GET /notes` (Protected)

**Query Parameters**
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `type` | string | Yes | `Organization` or `Site` |
| `id` | int | Yes | The ID of the record |

### Create Note

`POST /notes` (Protected)

**Request Body**
| Field | Type | Required |
| :--- | :--- | :--- |
| `parent_type` | string | Yes (`Organization`|`Site`) |
| `parent_id` | int | Yes |
| `content` | string | Yes |

### Update/Delete Note

`PATCH /notes/{id}` (Protected) (Body: `{"content": "..."}`)
`DELETE /notes/{id}` (Protected)

---

## Monitoring

### Get History

`GET /monitor/history` (Protected)

**Query Parameters**
| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `hours` | int | 48 | Lookback window |

### Get Status

`GET /monitor/status` (Protected)

**Query Parameters**
| Parameter | Type | Description |
| :--- | :--- | :--- |
| `domain` | string | Filter for specific site |

### Manage Ignored Sites

`GET /monitor/ignored`
`POST /monitor/ignored` (Body: `{"domain": "...", "reason": "..."}`)
`DELETE /monitor/ignored/{domain}`

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
- `404 Not Found`: Record does not exist
- `429 Too Many Requests`: Login rate limit exceeded
- `500 Internal Server Error`: Server-side error
