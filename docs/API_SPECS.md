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

## Company Management (Read/Write)

### List Companies
`GET /companies` (Protected)

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
    "info": "Notes about company",
    "created_at": "datetime",
    "created_by": "string",
    "updated_at": "datetime",
    "updated_by": "string"
  }
]
```

### Create Company
`POST /companies` (Protected)

**Request Body**
| Field | Type | Required |
| :--- | :--- | :--- |
| `name` | string | Yes |
| `vat_number` | string | No |
| `info` | string | No |

### Get/Update/Delete Company
`GET /companies/{id}`
`PATCH /companies/{id}`
`DELETE /companies/{id}` (Protected)

---

## Contact Management (Read/Write)

### List Company Contacts
`GET /companies/{id}/contacts` (Protected)

Returns all contact people associated with a specific company.

### Create Contact
`POST /contacts` (Protected)

**Request Body**
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `company_id` | int | Yes | |
| `name` | string | Yes | |
| `email` | string | No | |
| `phone` | string | No | |
| `type` | string | Yes | One of: `Main`, `Technical`, `Billing` |

### Update/Delete Contact
`PATCH /contacts/{id}`
`DELETE /contacts/{id}` (Protected)

---

## Site Linking

These endpoints manage the relationship between external site IDs and company records.

### Get Company for Site
`GET /sites/{site_id}/company` (Protected)

Returns the `Company` object linked to the specific Site ID.

### Link Site to Company
`POST /sites/{site_id}/link` (Protected)

**Request Body**
```json
{ "company_id": 123 }
```

### Unlink Site
`DELETE /sites/{site_id}/link` (Protected)

---

## Notes

Notes can be attached to either a `Company` or a `Site`.

### List Notes
`GET /notes` (Protected)

**Query Parameters**
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `type` | string | Yes | `Company` or `Site` |
| `id` | int | Yes | The ID of the record |

### Create Note
`POST /notes` (Protected)

**Request Body**
| Field | Type | Required |
| :--- | :--- | :--- |
| `parent_type` | string | Yes (`Company`|`Site`) |
| `parent_id` | int | Yes |
| `content` | string | Yes |

### Update/Delete Note
`PATCH /notes/{id}` (Body: `{"content": "..."}`)
`DELETE /notes/{id}` (Protected)

---

## Monitoring

### Get History
`GET /monitor/history` (Protected)

**Query Parameters**
| Parameter | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `hours` | int | 24 | Lookback window |

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
