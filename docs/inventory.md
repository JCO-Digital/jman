# Inventory System

jman's inventory is a CMDB-style registry of the assets we own or manage —
servers (including ones unrelated to jman's WordPress hosting) and domains —
regardless of where they live. It is **not** the functional server/site data:
the `servers`/`sites` tables in `inventory.db` drive WP-CLI targeting, the
agent manifest and monitoring, and stay untouched by this system. Inventory
items may soft-link to those functional entities (`site_id`/`server_id`).

The billing feature (organizations → assets/payments) is a separate concept
that used to own the `/inventory` web route and `/api/assets*` endpoints;
those moved to `/billing` to make room for this system.

## Core model

The core is a **manual entry system**. On top of that, items can be **linked
to external APIs** ("providers"), starting with SpinupWP. A provider link is
what survives: syncs refresh the linked item's `provider_data`, never its
manual fields.

- Manual and provider-linked items with the same type+name are kept
  **separate**; the web UI prompts "link / dismiss" when a match is found.
- Sync never creates duplicates of linked items, and never touches
  `origin = 'manual'` items.
- The system stores **no secrets** — at most usernames. `details` is free
  text/JSON visible to every API user.

### Tables (api.db)

```sql
inventory_items
  id            TEXT PRIMARY KEY        -- UUIDv7 (manual) or deterministic UUIDv5 per (provider, provider_type, provider_id)
  type          TEXT NOT NULL           -- 'server' | 'domain' (extensible)
  name          TEXT NOT NULL           -- server name; for domains the apex domain (www.example.com -> example.com)
  origin        TEXT NOT NULL DEFAULT 'manual'  -- 'manual' | 'provider'
  status        TEXT NOT NULL DEFAULT 'active'  -- 'active' | 'removed'
  description   TEXT DEFAULT ''
  tags          TEXT DEFAULT '[]'       -- JSON array of strings
  details       TEXT DEFAULT '{}'       -- JSON, manual, free-form per type (IPs, access/username, services, registrar, DNS servers, expiry, ...)
  provider_data TEXT DEFAULT '{}'       -- JSON snapshot from the linked API; refreshed by syncs, read-only
  site_id       TEXT                    -- optional soft ref to a functional site (inventory.db)
  server_id     TEXT                    -- optional soft ref to a functional server
  created_at / created_by / updated_at / updated_by

inventory_item_links
  id            INTEGER PRIMARY KEY AUTOINCREMENT
  item_id       TEXT NOT NULL REFERENCES inventory_items(id) ON DELETE CASCADE
  provider      TEXT NOT NULL           -- 'spinupwp', ...
  provider_type TEXT NOT NULL           -- 'server', 'site_domain', ...
  provider_id   TEXT NOT NULL           -- external ID ('<siteID>:<domain>' for domains)
  UNIQUE(provider, provider_type, provider_id)

inventory_link_dismissals               -- remembers "dismiss" on a match prompt
  item_id, provider, provider_type, provider_id, created_at, created_by
  PRIMARY KEY (item_id, provider, provider_type, provider_id)
```

There is deliberately **no unique constraint on `name`**: a manual item and a
synced item may coexist with the same name; matches are surfaced, not merged.

Domain-type names are normalized to the **apex (registrable) domain** on save
and on sync, via `publicsuffix.EffectiveTLDPlusOne`, so `www.example.com` and
`example.com` are one inventory item.

## Providers

`internal/inventory` defines the framework:

```go
type ExternalItem struct {
    Type         string         // inventory item type ('server', 'domain')
    ProviderType string         // entity kind at the provider ('server', 'site_domain')
    ProviderID   string         // stable external ID
    Name         string         // item name (apex domain for domains)
    Data         map[string]any // goes into provider_data
    SiteID       string         // optional soft link to a functional site UUID
}

type Provider interface {
    Name() string
    Fetch(ctx context.Context) ([]ExternalItem, error)
}
```

`Sync(ctx, providerName)` upserts items by link: existing link → refresh
`provider_data`, `name` and status; new entity → link it to the item another
entity of the same `(type, name)` is linked to, or else create an item
(`origin='provider'`, deterministic UUIDv5) plus link; links whose entity
disappeared are deleted, and an `origin='provider'` item left with no links is
marked `status='removed'` (kept for history, like SpinupWP sites today).

- An entity that comes back, or was unlinked by hand, is linked again to the
  item that still exists under its deterministic UUID, and that item becomes
  `active` again. Unlinking a provider-created item therefore only lasts until
  the next sync.
- An item with several links takes `name`, `provider_data` and the site link
  from one entity: the one it was created for, else the one with the smallest
  `(provider_type, provider_id)`. The fetch order doesn't matter.
- `site_id` is only filled in when the item has none, so a site link set by
  hand stays (clearing it lets the sync fill it in again).
- Items are written only when something changed, and a sync never touches
  `updated_at`/`updated_by`: those show the last manual edit.
- Nothing is removed when the fetch is empty, or for a provider type the fetch
  returned no entities of (no sites but all servers is taken for a partial
  fetch). The last entity of a type is therefore never removed automatically.

The first provider is **SpinupWP** (`internal/inventory/spinupwp`), reusing
`internal/fetch/spinupwp`:

- servers → `type=server` items (`provider_type='server'`, `provider_id` =
  SpinupWP server ID); `provider_data` carries name, IP, SSH port, provider,
  region, size, OS, disk, connection status, …
- each site's primary domain **and** `additional_domains` → `type=domain`
  items named by apex domain (`provider_type='site_domain'`,
  `provider_id='<siteID>:<domain>'`), soft-linked to the managed site via
  `site_id`.

Sync runs after every successful fast tick of jman-api's refresh scheduler
(`internal/refresh`), and manually via `POST /api/inventory/sync/{provider}`.
The CLI does not consume inventory data (no `cliproto.Snapshot` change).

## REST API

| Method & path | Auth | Purpose |
|---|---|---|
| `GET /api/inventory/types` | basic | item types + their `details` template (key/kind/hint per field; the UI renders it as a form, empty fields are not saved, with an add-field button and a raw-JSON editor toggle) |
| `GET /api/inventory/items?type=&status=&search=` | basic | list, joined with links |
| `POST /api/inventory/items` | edit | create manual item; domain names normalized to apex; `409` + `existing_item_id` on type+name match |
| `GET /api/inventory/items/{id}` | basic | item + links + match suggestions (undismissed) |
| `PATCH /api/inventory/items/{id}` | edit | edit manual fields; provider-owned fields rejected on linked items |
| `DELETE /api/inventory/items/{id}` | edit | manual items only; provider-linked → `409` |
| `POST /api/inventory/items/{id}/links` | edit | link an external entity to the item (merges the auto-created item into it) |
| `DELETE /api/inventory/items/{id}/links/{provider}/{providerType}/{providerId}` | edit | unlink |
| `POST /api/inventory/items/{id}/dismiss` | edit | dismiss a match prompt |
| `POST /api/inventory/sync/{provider}` | edit | run a provider sync now |

## Web UI

- Billing moved to `/billing*` routes and `/api/billing/*` endpoints; the nav
  label stays "Assets".
- New "Inventory" nav section at `/inventory`: list with type filter, search,
  provider badge and a sync button; detail/edit view with a type-specific form
  writing `details`, the links list, and the link/dismiss prompt when a manual
  item matches a synced item of the same type+name.

## Files

- `internal/models/inventory.go` — `InventoryItem`, `InventoryItemLink`, `InventoryLinkDismissal`, type/status constants
- `internal/db/inventory_repo.go` — CRUD + link/dismissal queries
- `internal/db/db.go` — the three table definitions in `initAPISchema`
- `internal/inventory/` — provider framework, registry, sync runner
- `internal/inventory/spinupwp/` — SpinupWP provider
- `internal/api/inventory_handlers.go` — REST handlers, registered in `handlers.go`
- Notes attach through the generic notes system (`/api/notes`) with
  `parent_type = "InventoryItem"`; the detail view embeds `NotesWidget`. The
  `notes` key is deliberately not part of the details templates.
- `web/src/views/Inventory/` — list/detail views; router + `AppNav` changes
