# Site & Server Identity Contract

After the host-agnostic refactor (see `PLAN.md`), every site and server is identified by a
**UUID string** end to end: database, API, agent protocol and web UI. Legacy SpinupWP integer IDs
survive only as provider metadata (`provider_site_id` / `provider_server_id`) and as an accepted
*input* alias on path parameters.

SpinupWP entities get deterministic UUIDv5s (`utils.SpinupWPSiteUUID(id)` /
`utils.SpinupWPServerUUID(id)`); manual/external entities get UUIDv7s.

## Rules

1. **Storage** — every `site_id` / `server_id` column (and `ignore_entries.target` for
   `site`/`server` rules, `ignore_entries.negated_site_ids`, `notes.parent_id` where
   `parent_type = 'Site'`) holds a UUID string. Nothing writes integers.
2. **Responses** — every `id` of a site/server and every `site_id` / `server_id` field in any
   JSON response is a UUID string. An absent link is `null` or omitted, never `0` or `""`.
3. **Requests** — path params `{id}` for sites/servers accept a UUID *or* a legacy SpinupWP
   integer (resolved via `resolveSiteUUID` / `resolveServerUUID`). Body fields `site_id` /
   `server_id` accept the same (a JSON string UUID; a legacy number is converted). Stored values
   are always the canonical UUID.
4. **Migration** — the legacy-integer → UUID migration runs **once per database** (recorded in a
   `schema_migrations` table) and understands `"123"` and `"123.0"` forms.

## API shapes

### `GET /api/sites`
Built from the authoritative `sites` table, enriched with SpinupWP cache details where
`provider = "spinupwp"`.

```jsonc
{
  "id": "0190…",                 // UUID
  "provider": "spinupwp",        // "spinupwp" | "manual" | "wpengine" | …
  "provider_site_id": "12345",   // SpinupWP ID as string; omitted for non-SpinupWP
  "server_id": "0190…",          // UUID or null
  "server_name": "lon1.example", // "" when no server
  "domain": "example.com",
  "environment": "production",
  "status": "active",
  "is_wordpress": true,
  "php_version": "8.3",
  "site_user": "example",
  // capabilities (from the sites table)
  "connection_type": "agent",    // "agent" | "ssh" | "none"
  "can_wp_cli": true,
  "has_agent": true,
  "has_monitoring": true,
  // SpinupWP-only detail fields (https, nginx, backups, page_cache, database, basic_auth,
  // additional_domains, public_folder, user_auth, wp_*_update(s), created_at) are present only
  // when provider == "spinupwp" (omitted otherwise).
  "disk_usage": { … }, "wp_flags": { … }, "last_update": { … }, "wp_core": { … }
}
```

### `GET /api/servers`
Built from the `servers` table, enriched with SpinupWP cache details for SpinupWP servers.

```jsonc
{
  "id": "0190…", "provider": "spinupwp", "provider_server_id": "678",
  "name": "lon1.example", "is_logical": false, "ip_address": "…", "ssh_port": 22,
  // SpinupWP-only detail fields (provider_name, ubuntu_version, timezone, region, size,
  // disk_space, database, connection_status, reboot_required, upgrade_required, status, …)
  // present only when provider == "spinupwp".
}
```
`?format=managed` is kept as an alias returning the same list.

### Other responses (all IDs UUID strings)
| Endpoint | Field(s) |
|---|---|
| `GET /api/plugins` | `site_id` |
| `GET /api/vulns`, `/api/vulns/core` | `sites[].site_id` |
| `GET /api/core-versions` (or equivalent) | `sites[].site_id` |
| `GET /api/tasks…` | `site_id`, `server_id` (omitted when unset) |
| `GET /api/organization-assets…` | `site_id` (null when unset) |
| `GET /api/sites/{id}/update-ledger`, `sites[].last_update` | `site_id` |
| `GET /api/agent-tokens` | `server_id` |
| `GET /api/ignore` | `target` (UUID for `site`/`server`), `negated_site_ids: string[]` |
| `GET /api/organizations/{id}/sites` | same site shape as `/api/sites` |
| `GET /api/sites/{id}/organization` | organization |
| Notes for sites | `parent_type: "Site"`, `parent_id: <site UUID>` |

### Capability gating (UI + API)
- Plugin updates / core update / test-connection require `can_wp_cli`.
- Traffic analytics require `has_agent` (endpoint returns empty data otherwise).
- Agent tokens can be created for any server in `/api/servers`.

## jman-agent protocol (breaking, agents self-update)
- `GET /api/agent/manifest` → `{ server_id: UUID, sites: [{ site_id: UUID, legacy_site_id?: int, domain, site_user, is_wordpress }], api_version }`
- `POST /api/agent/report` → `sites[].site_id: UUID`
- Agent log state is stored as `site-<uuid>.json`; on first run a legacy `site-<legacy_site_id>.json` is renamed.
- Old agents fail to decode the new manifest; their periodic self-update ticker upgrades them.
