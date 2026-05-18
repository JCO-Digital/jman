# Unified Ignore List Specification

The `jman` suite uses a unified ignore system to suppress alerts and reports for specific assets across monitoring and vulnerability scanning.

## Database Schema

Ignore rules are stored in the `ignore_entries` table.

- **`id`** (INTEGER): Primary key.
- **`type`** (TEXT): The type of target being ignored. Valid values: `site`, `server`, `plugin`, `vulnerability`.
- **`target`** (TEXT): The identifier for the target:
  - `site`: SpinupWP Site ID.
  - `server`: SpinupWP Server ID.
  - `plugin`: Plugin slug.
  - `vulnerability`: Vulnerability UUID.
- **`reason`** (TEXT): A free-text description of why the item is ignored.
- **`negated_site_ids`** (TEXT): A JSON array of SpinupWP Site IDs. Used for `server` ignores to keep specific sites active while the rest of the server is ignored.
- **`use_for_monitor`** (BOOLEAN): If true, this rule applies to uptime monitoring (`jman-monitor`).
- **`use_for_vuln`** (BOOLEAN): If true, this rule applies to vulnerability scanning (`jman vuln`).
- **`created_at` / `updated_at`** (DATETIME): Audit timestamps.
- **`created_by` / `updated_by`** (TEXT): The user who created or last modified the entry.

## Logic & Hierarchy

### Uptime Monitoring

A site check is skipped if **any** of the following match with `use_for_monitor = true`:

1.  **Site Match**: A `site` entry exists for the site's ID.
2.  **Server Match**: A `server` entry exists for the site's server ID, **and** the site ID is **not** present in the `negated_site_ids` list.

### Vulnerability Reporting

A vulnerability found on a site is suppressed if **any** of the following match with `use_for_vuln = true`:

1.  **Vulnerability Match**: A `vulnerability` entry exists for the specific UUID.
2.  **Plugin Match**: A `plugin` entry exists for the plugin's slug.
3.  **Site Match**: A `site` entry exists for the site's ID.
4.  **Server Match**: A `server` entry exists for the site's server ID, **and** the site ID is **not** present in the `negated_site_ids` list.

## CLI Usage

The system is managed via the `jman ignore` command.

### Listing Entries

```bash
jman ignore list
```

Displays all entries. Site and Server IDs are automatically resolved to domains and names for readability.

### Adding Entries

```bash
jman ignore add <type> <identifier> [reason] [flags]
```

- **Site**: `jman ignore add site example.com "Maintenance" --monitor`
- **Server**: `jman ignore add server "Production Node 1" --monitor --negate mysite.com`
- **Plugin**: `jman ignore add plugin akismet "Internal fork" --vuln`
- **Vulnerability**: `jman ignore add vuln <uuid> "False positive" --vuln` (Type `vuln` is mapped to `vulnerability`)

### Removing Entries

```bash
jman ignore remove <id>
```

## API Specification

Endpoints are protected by JWT and require the `edit` level for modifications.

- **`GET /api/ignore`**: Returns all entries. Supports optional `?type=` filter.
- **`POST /api/ignore`**: Create a new entry.
- **`PATCH /api/ignore/{id}`**: Update reason, flags, or negated sites for an entry.
- **`DELETE /api/ignore/{id}`**: Remove an entry.
