# Moving jman-api's api.db to PostgreSQL

Status: **deferred** (assessed 2026-10-02). Phase 0 is done; Phases 1–5 are still to do.

## Scope

- **In scope:** `api.db`, which only jman-api uses: organizations, billing, tasks, monitor state, incidents, agent tokens, traffic and disk usage.
- **Out of scope:** `inventory.db` stays SQLite. The `jman` CLI runs on laptops with its own local `inventory.db`, and on the server only jman-api uses it (about 1.3 MB). The CLI never opens `api.db` except in the one-off `MigrateSplitDB`.

## Verdict

**Feasible, medium-sized, low architectural risk, not urgent.**

- **No SQL query joins across the two databases.** Where data from both is combined, it happens in Go: `loadAPISites` in `internal/api/data_handlers.go`, `cleanupOrphanedTasks` in `internal/tasks/scheduler.go`, and `internal/reports/traffic.go`. That code is unaffected.
- **jman-agent never touches a database.**
- **Builds stay pure Go.** pgx/v5 needs no CGO, so the CGO-free builds, the Windows build of `jman` and minisign-signed releases all keep working. Put the Postgres code in a package only jman-api imports, so the CLI never links pgx.
- **The original trigger is fixed.** The slowness came from SQLite connection contention, fixed in 8f08dc7. Postgres is worth doing for:
  - several API processes or zero-downtime deploys
  - real time types
  - online schema changes
  - point-in-time recovery (PITR) backups
  - much larger traffic and history tables
- **Costs:**
  - running and monitoring a Postgres service
  - two SQL dialects to maintain until the SQLite path for `api.db` is removed

## Problems found

| Area | Size | Notes |
|---|---|---|
| `?` placeholders | ~375, ~16 in dynamically built queries | Fix centrally in a rebinding `GetAPIDB` wrapper (Phase 2) |
| `LastInsertId()` | 10 | Replaced with `RETURNING id` in Phase 0 |
| Time storage | ~45 DATETIME columns | Normalized to canonical UTC in Phase 0. On Postgres they become `TIMESTAMPTZ` |
| SQLite date functions | ~13 | `'now'`-modifier and `strftime` forms replaced in Phase 0. `date(col)` is valid on Postgres |
| Schema system | 1 subsystem | `migrateTable` / `TableDefinition` rely on PRAGMA and `sqlite_master`. Replace with versioned migrations for `api.db` |
| Case-insensitivity | 6 `LIKE`, 3 `COLLATE NOCASE` domain columns | Code stopped relying on SQLite's case folding in Phase 0 |
| Postgres-rejected queries | 2 | Fixed in Phase 0 |
| SQLite-only statements | ~4 | `INSERT OR IGNORE` and boolean literals fixed in Phase 0. `UPDATE OR REPLACE` stays: it's only in the legacy UUID migration, which doesn't apply to a fresh Postgres |
| Backups | 1 | `VACUUM INTO` → `pg_dump -Fc` for `api.db`. Inventory keeps `VACUUM INTO` |
| Data copy | 20 tables | Possible orphaned foreign keys; identity sequences need `setval` |
| Tests / CI | ~20 DB test files | Need a Postgres service in CI |

Raw SQL outside `internal/db` (`monitor/state.go`, `monitor/schedule.go`, `slack/slack.go`) was moved into `internal/db` in Phase 0.

## Phases

### Phase 0: portable SQL while still on SQLite (done)

- Added the `APIDB` wrapper. Every `time.Time` argument is written in UTC, and `_time_format=sqlite` is set. The canonical stored form is `YYYY-MM-DD HH:MM:SS[.fff]+00:00`.
- Added a one-off `normalize_api_timestamps` migration that rewrote the existing mixed-format and mixed-offset values. Before it, text comparisons such as the billing `before` filter could be off by the UTC offset.
- Times are passed as `time.Time` values instead of strings at the edges: agent ingestion, billing filters, reports and cutoffs.
- `RETURNING id`, `ON CONFLICT DO NOTHING`, `TRUE`/`FALSE`, `LOWER() LIKE LOWER()`, domain lowercasing, and portable `GROUP BY` and downsample queries.
- Moved the raw api.db SQL out of `monitor/` and `slack/` into `internal/db`.
- Visible effect: API JSON now returns these timestamps as UTC (`…Z`) instead of with the writer's local offset. They are the same instants, and the web UI parses them with `new Date()`. Slack task dates are formatted in the server's local zone.

### Phase 1: versioned migrations for api.db

- Add an embedded-SQL runner (`internal/db/migrations/api/{sqlite,postgres}/NNNN_*.sql`) on top of the existing `schema_migrations` table. On existing databases the SQLite baseline is marked as already applied.
- Postgres types:
  - `TIMESTAMPTZ`
  - `BIGINT GENERATED ALWAYS AS IDENTITY`
  - `BOOLEAN DEFAULT false`
  - `DATE` for `day`
  - `citext` or `lower()` indexes for domains
- `migrateTable` stays for inventory only.

### Phase 2: dialect switch

- Add `github.com/jackc/pgx/v5/stdlib`.
- Add `apiDatabaseURL` to `AppConfig` and `envBindings` in `internal/config/config.go`. If it's set, `InitAPI` opens Postgres; if not, it opens SQLite.
- Extend the `APIDB` wrapper to rewrite `?` placeholders as `$n` on Postgres.
- Backups: `pg_dump -Fc`, keeping the timestamped and latest-symlink layout, 48 h retention and Slack alerts in `internal/backup`.

### Phase 3: tests and CI

- api DB tests run on SQLite by default, and on Postgres too when `JMAN_TEST_PG_URL` is set (one schema per test).
- Add a `services: postgres` container to `.github/workflows/ci.yml` and run the suite on both engines.

### Phase 4: data migration and cutover

- Add `jman-api migrate-to-postgres --dry-run`. It should:
  1. pre-check for orphaned foreign keys
  2. copy tables in foreign-key order
  3. run `setval` on the identity sequences
  4. verify row counts and checksums
- Ops:
  - Postgres on the same host over a local socket
  - `After=` / `Requires=postgresql.service` in `jman-api.service`
  - a written restore runbook
- Cutover: stop the API, take a final backup, migrate, set the DSN, start the API. To roll back, unset the DSN; the original `api.db` is left untouched.

### Phase 5: cleanup (optional)

- Once Postgres is stable, remove the SQLite `api.db` path. That halves the test matrix.
- If more than one API process will run, revisit `globalWriteMu` and the unguarded read-then-write code (`CreateIncident`, `RecordHistory`).

## Risks

- **Hidden reliance on SQLite being lenient:** type affinity, and orphaned foreign keys that SQLite tolerated in rows older than enforcement.
- **Two dialects to maintain** until Phase 5. Every new query must pass on both engines.
- **A new service to operate:** upgrades, monitoring and disk space.

## Verification

- Run the dry-run migration against a copy of the production `api.db` and compare row counts and spot-checked values.
- Run a staging jman-api on Postgres with a copy of the data. Compare the main list endpoints, monitor history, traffic reports, agent ingestion and the backup job against the SQLite instance.
