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
  provider_data TEXT DEFAULT '{}'       -- JSON snapshot from each linked API, keyed by provider name; refreshed by syncs, read-only
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

// Optional: entities that can be recognized in another provider's snapshot.
type Matcher interface {
    MatchKeys(itemType models.InventoryItemType, data map[string]any) []string
}
```

`Sync(ctx, providerName)` upserts items by link: existing link → refresh the
provider's `provider_data` entry, `name` and status; new entity → link it to
the item another entity of this provider with the same `(type, name)` is
linked to, else to the item of another provider whose snapshot shares a match
key with it (see below), or else create an item (`origin='provider'`,
deterministic UUIDv5) plus link; links whose entity
disappeared are deleted, and an `origin='provider'` item left with no links is
marked `status='removed'` (kept for history, like SpinupWP sites today).

- An entity that comes back, or was unlinked by hand, is linked again to the
  item that still exists under its deterministic UUID, and that item becomes
  `active` again. Unlinking a provider-created item therefore only lasts until
  the next sync.
- `provider_data` is keyed by provider (`{"spinupwp": {...}, "hetzner":
  {...}}`). Each sync writes only its own entry, so one item can be linked to
  several providers without their syncs overwriting each other. A provider's
  entry is dropped when the item has no link of that provider left.
- An item with several links of one provider takes that provider's snapshot
  and the site link from one entity: the one it was created for, else the one
  with the smallest `(provider_type, provider_id)`. The fetch order doesn't
  matter.
- The **name** is written only by the provider that owns it: the one whose
  entity created the item, else the first by name among its links. A server
  that SpinupWP and Hetzner name differently keeps its creator's name.
- **Matching across providers**: a provider implementing `Matcher` returns
  match keys for a snapshot. A new entity joins the active item whose snapshot
  from another (`Matcher`) provider it still links has one of its keys. Keys
  are read from the stored snapshots, so it doesn't matter which provider
  syncs first. A key found on two items is ambiguous and matches nothing.
  Servers match by public IPv4 address (`ipv4:<address>`; private, loopback
  and other non-public addresses never match), so a Hetzner Cloud server
  joins the SpinupWP server it runs. `SyncResult.matched` counts these joins.
- `site_id` is only filled in when the item has none, so a site link set by
  hand stays (clearing it lets the sync fill it in again).
- Items are written only when something changed, and a sync never touches
  `updated_at`/`updated_by`: those show the last manual edit.
- Nothing is removed when the fetch is empty, or for a provider type the fetch
  returned no entities of (no sites but all servers is taken for a partial
  fetch). The last entity of a type is therefore never removed automatically.

### SpinupWP

**SpinupWP** (`internal/inventory/spinupwp`) reads the shared servers/sites
cache:

- servers → `type=server` items (`provider_type='server'`, `provider_id` =
  SpinupWP server ID); `provider_data` carries IP, SSH port, provider,
  region, size, OS, disk size (`disk_total`, bytes), connection status, …
  (not disk usage, which changes on every refresh, or the database host)
- each site's primary domain **and** `additional_domains` → `type=domain`
  items named by apex domain (`provider_type='site_domain'`,
  `provider_id='<siteID>:<domain>'`), soft-linked to the managed site via
  `site_id`; `provider_data` carries only domain facts (`domain`,
  `apex_domain`, `additional`, `certificate_expires`), not the site's
  PHP version, status, environment and so on.

### Hetzner Cloud

**Hetzner Cloud** (`internal/inventory/hetzner`) lists the cloud servers of
every project in `hetznerApiTokens` (jman-api config, one API token per
project; read-only tokens are enough; `JMAN_HETZNERAPITOKENS` takes them
comma-separated). Without tokens the provider is skipped.

- servers → `type=server` items (`provider_type='cloud_server'`,
  `provider_id` = Hetzner server ID, unique across projects); `provider_data`
  carries status, server type, cores, `memory_gb`, `disk_gb`, CPU type,
  architecture, location/city/country, `ipv4`/`ipv6`, image, backup window,
  labels, `delete_protection` and `created_at` (not the ID or name, which the
  link and the item hold, nor traffic counters, which change all the time).
- It reads up to 50 servers per page via `GET /v1/servers`. A failure in any
  project fails the whole fetch, so a sync never removes the servers of a
  project it couldn't read.
- Servers match SpinupWP servers by public IPv4 address (above).

Sync runs after every successful fast tick of jman-api's refresh scheduler
(`internal/refresh`), and manually via `POST /api/inventory/sync/{provider}`.
The CLI does not consume inventory data (no `cliproto.Snapshot` change).

## Domain registry lookups

Every active domain item, whatever its origin, is looked up at its registry
(`internal/inventory/domainlookup`) for its delegated nameservers, registrar
and expiry date. The result is stored per item in `inventory_domain_lookups`,
separate from `details` (manual) and `provider_data` (per provider link).

- **RDAP first**: the registry's RDAP server comes from IANA's bootstrap file
  (`https://data.iana.org/rdap/dns.json`, cached for a day). `expiration`
  event → expiry, `nameservers` → nameservers, the `registrar` entity's vCard
  name → registrar.
- **WHOIS for the gaps**: when RDAP leaves a field out, or a TLD has no RDAP,
  the registry's WHOIS server is asked if `whoisServers` (`whois.go`) has an
  entry for the TLD: its address and the keys of the "key: value" lines that
  hold each field. Only `.fi` is configured: rdap.fi has no registrar, which
  `whois.fi` publishes (in Latin-1). Adding a WHOIS-only TLD is an entry in
  that map.
- A TLD with neither is stored with an "unsupported" error. A field the
  registry doesn't publish stays empty; the UI says "Not published by the
  registry".
- **Schedule**: each slow tick looks up at most 40 due items: never looked up,
  renamed since, last success older than 24 hours, or last failure older than
  6 hours. Requests to one registry host are at least 2 seconds apart.
- A failed lookup stores its error and keeps the last successful data and
  its time (`last_success_at`); a "not found at the registry" answer clears
  the data.
- Only the domain name is sent, straight to the TLD's registry.

## REST API

| Method & path | Auth | Purpose |
|---|---|---|
| `GET /api/inventory/types` | basic | item types + their `details` template (key/kind/hint per field; the UI renders it as a form, empty fields are not saved, with an add-field button and a raw-JSON editor toggle) |
| `GET /api/inventory/items?type=&status=&search=` | basic | list, joined with links |
| `POST /api/inventory/items` | edit | create manual item; domain names normalized to apex; `409` + `existing_item_id` on type+name match |
| `GET /api/inventory/items/{id}` | basic | item + links + match suggestions (undismissed) + `domain_lookup` for domain items |
| `PATCH /api/inventory/items/{id}` | edit | edit manual fields; provider-owned fields rejected on linked items |
| `DELETE /api/inventory/items/{id}` | edit | manual items only; provider-linked → `409` |
| `POST /api/inventory/items/{id}/links` | edit | link an external entity to the item (merges the auto-created item into it) |
| `DELETE /api/inventory/items/{id}/links/{provider}/{providerType}/{providerId}` | edit | unlink |
| `POST /api/inventory/items/{id}/dismiss` | edit | dismiss a match prompt |
| `POST /api/inventory/items/{id}/lookup` | edit | look a domain item up at its registry now (a lookup less than a minute old is returned as it is) |
| `POST /api/inventory/sync/{provider}` | edit | run a provider sync now |

## Web UI

- Billing moved to `/billing*` routes and `/api/billing/*` endpoints; the nav
  label stays "Assets".
- New "Inventory" nav section at `/inventory`: list with type filter, search,
  provider badge and a sync button; detail/edit view with a type-specific form
  writing `details`, the links list, and the link/dismiss prompt when a manual
  item matches a synced item of the same type+name. Domain items have a
  Registration card with the registry lookup and a "Look up now" button.

## Files

- `internal/models/inventory.go` — `InventoryItem`, `InventoryItemLink`, `InventoryLinkDismissal`, type/status constants
- `internal/db/inventory_repo.go` — CRUD + link/dismissal queries
- `internal/db/db.go` — the three table definitions in `initAPISchema`
- `internal/inventory/` — provider framework, registry, sync runner
- `internal/inventory/spinupwp/` — SpinupWP provider
- `internal/inventory/domainlookup/` — RDAP/WHOIS registry lookups of domain items; `internal/db/inventory_lookup_repo.go` stores them
- `internal/inventory/hetzner/` — Hetzner Cloud provider
- `internal/db/migrate_inventory_provider_data.go` — one-time migration keying the old flat `provider_data` (all SpinupWP's) by provider
- `internal/api/inventory_handlers.go` — REST handlers, registered in `handlers.go`
- Notes attach through the generic notes system (`/api/notes`) with
  `parent_type = "InventoryItem"`; the detail view embeds `NotesWidget`. The
  `notes` key is deliberately not part of the details templates.
- `web/src/views/Inventory/` — list/detail views; router + `AppNav` changes
