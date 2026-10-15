# Making the jman CLI a client of jman-api

Status: **in progress** (design agreed 2026-10-15). Phases 1–4 are done; Phase 5 is still to do.

## Why

- **Manually added sites are invisible to the CLI.** Sites that aren't on SpinupWP exist only in jman-api. The CLI fetches from SpinupWP itself, so it never sees them, and adding them by hand on every laptop isn't feasible.
- **Plugin update locks live in the API.** The CLI can't read them today.
- **It unblocks Postgres.** Once the CLI stops importing `internal/db`, both `inventory.db` and `api.db` are server-only. The constraint that kept inventory on SQLite in [postgres-migration.md](postgres-migration.md) goes away.

## Scope

- **Full switch:** jman-api is the only data source. The CLI never talks to SpinupWP, wordpress.org or the vulnerability feeds directly, and the SpinupWP token comes off every laptop.
- **Local cache:** the CLI keeps a JSON mirror of a single API endpoint. It reads this mirror first and also uses it as the offline fallback. There is no local SQLite.
- **Actions:**
  - All plugin changes (update, install, remove) and WordPress core updates go through the API. Update jobs already check locks server-side, and the API's view of each site's plugins and core version stays correct immediately.
  - The other SSH-based commands (`wp`, `admin`, `mods`, `setup`, `alias`) keep running from the laptop, using the SSH details from the mirror. `setup mu` and `setup compat` write mu-plugin files rather than managing regular plugins, so they stay on SSH by choice.
- **Online-only:** vuln reporting and other heavy queries come from the API and don't work offline.
- **Distribution:** the AUR package is dropped, and GitHub self-update is the only update path.
- **Out of scope:** the Postgres migration itself. It starts after this work is finished.

## Design

### Snapshot endpoint and local mirror

`GET /api/cli/snapshot` (basic level) returns one gzipped JSON document:

```json
{
  "schema_version": 1,
  "servers": [...],
  "sites":   [...],            // SpinupWP and manual, with SSH details, core version and update locks
  "plugins": [...],            // per-site plugin lists
  "plugin_info": [...]         // slug → display name, latest version
}
```

- **Manual sites override SpinupWP ones.** When a manual site and a SpinupWP site share a domain, the snapshot contains only the manual one. The merge happens on the server, so the CLI never has to resolve duplicates.
- **Same data for every role**, so one ETag covers everyone. The ETag is a hash of the serialized body.
- **Expected size:** a few hundred sites with about 30 plugins each, so roughly 10k plugin rows. That's fine gzipped. If it grows too large, split plugins out or add delta sync.
- **The CLI stores the body plus metadata** (`etag`, `fetched_at`, `last_attempt_at`, `last_error`) under the XDG data dir as `snapshot.json`.

**Freshness rules, applied on every command:**

1. If the mirror is younger than `cacheTTL`, use it.
2. Otherwise send a conditional `GET` (`If-None-Match`) with a `apiTimeout` timeout. A `304` only bumps `fetched_at`.
3. If the request fails, use the existing mirror, record `last_attempt_at`, and don't retry for `offlineBackoff`, so offline commands don't pay the timeout each time.
4. If the mirror is older than `staleWarning`, print a warning on stderr. If there is no mirror at all and the API is unreachable, fail with a clear message.
5. `jman refresh` (and `--fresh` on any command) skips the TTL. For execute level and higher, it also calls the server-side refresh first (see below).

Shell completions read the mirror only and never hit the network.

**Configuration.** All timing values live in the CLI's `config.toml`, as Go duration strings:

| Key | Default | Meaning |
|---|---|---|
| `cacheTTL` | `2m` | How old the mirror can get before a command re-checks the API |
| `apiTimeout` | `3s` | Timeout for the snapshot check |
| `offlineBackoff` | `15m` | How long to wait after a failed check before trying again |
| `staleWarning` | `168h` | Mirror age that triggers the stale-data warning |

### Server-side refresh

`POST /api/refresh` refreshes servers and sites from SpinupWP for the CLI:

- **Execute level or higher.** It blocks until the fetch is done.
- **Singleflight.** Requests that arrive while a refresh is running wait for that same refresh. The scheduler's fast tick and the web UI's `POST /api/providers/spinupwp/sync` go through the same guard (`internal/refresh/fast.go`), so a manual refresh and a scheduled one never overlap.
- **Rate limit.** If the last successful refresh finished less than 60s ago, return immediately without fetching. The caller then re-fetches the snapshot as usual.
- **Errors.** On a SpinupWP error or timeout, return the error. The CLI prints a warning and keeps the cached data.

Without a manual refresh, the worst-case lag for a new SpinupWP site is the fast tick (`refreshFastInterval`, default 5 min) plus the 2-minute client TTL.

### Authentication

The web UI handles login, so the CLI never handles passwords or TOTP.

**`jman login [api-url]`** is the only setup step. The URL argument is saved to `config.toml` as `apiUrl`. It is optional when `apiUrl` is already set, and required otherwise. Passing a URL when one is configured replaces it, and the old token is dropped, because it belongs to the other server.

The CLI asks the API where its web UI lives (a `webUrl` field in `GET /api/health`, from the API config), so users only ever enter one URL even when the web UI is on a different origin.

**The login uses a loopback redirect:**

1. The CLI starts a listener on `127.0.0.1:<random port>`, generates a `state` value and a PKCE verifier, and opens `<web>/cli-login?port=…&state=…&challenge=…`.
2. The user logs in in the browser as usual, including 2FA, and approves "Authorize jman CLI on `<hostname>`".
3. The web app redirects to the loopback address with a one-time code. The CLI exchanges the code plus the verifier at `POST /api/cli/token` for a CLI token.
4. If the browser can't be opened, print the URL. A device-code flow can be added later if headless use ever comes up.

**CLI tokens:**

- **Random 32 bytes, prefixed** `jman_cli_…`.
- **Stored in a new `cli_tokens` table** in `api.db` as a SHA-256 hash, along with the user, device name, `created_at` and `last_used_at`. Use portable SQL.
- **Sliding expiry.** A token is rejected when `now - last_used_at > 90 days`. `last_used_at` is written at most once an hour. The token value never rotates. Usage only comes from real commands, since there is no background job, so the expiry reflects actual activity.
- **Role and 2FA status are resolved on every request** from `users.toml`, the same way `AuthMiddleware` does for JWTs, including the `RestrictedFrom` downgrade. Demoting a user takes effect immediately.
- **Managed from the web UI** under the user's profile: list tokens with device and last use, and revoke them. Admins can revoke any user's tokens.
- **`AuthMiddleware`** accepts either a JWT or a `Bearer jman_cli_…` token.
- **Client-side storage:** the OS keyring via `zalando/go-keyring` or similar, with a fallback file at mode 0600 in the XDG config dir. `jman logout` revokes the token server-side and deletes it locally.

### Version gating

Every API response carries two headers:

- `X-Jman-Min-CLI-Version`: below this version the CLI refuses to run, apart from `update`, `login`, `logout` and `version`, and tells the user to run `jman update`.
- `X-Jman-Latest-CLI-Version`: if the CLI is older, it prints a one-line notice on stderr, at most once a day.

The values go in headers because a `304` has no body, and because headers stay readable even if a future body format breaks an old CLI's parser. The mirror stores the last-seen minimum, so the gate also applies offline. `schema_version` in the body covers only the format of the mirror itself.

## CLI command mapping

| Command | Today | After |
|---|---|---|
| `fetch` | SpinupWP, wp.org, vuln feeds → local cache/DB | Removed. `jman refresh` replaces it |
| `search`, completions, target resolution | `cache.*` JSON and SpinupWP | Mirror, filtered in memory in Go |
| `alias` | Cache + `db.ListManagedSites` | Mirror |
| `wp`, `admin`, `mods` | SSH using cached site data | Unchanged, with SSH details from the mirror |
| `setup mu` / `setup compat` | SSH, plus `db.GetSitesWithPlugin` | SSH, with the plugin lookup from the mirror |
| `plugin list` | SSH | Mirror. `--live` reads the site over SSH |
| `plugin info` | SSH | Unchanged (SSH) |
| `plugin update` | SSH via `wpcli.UpdatePlugin` | `POST /api/plugin-update-jobs`, then poll `GET /api/update-jobs/{id}` |
| `plugin install` | SSH | `POST /api/sites/{id}/plugin-install`. A local ZIP is uploaded as multipart, a slug or URL is sent as JSON |
| `plugin remove` | SSH | `POST /api/sites/{id}/plugin-actions`: `deactivate` if active, then `uninstall` |
| `core version` | SSH | Mirror |
| `core check` | SSH | `GET /api/sites/{id}/core-update` |
| `core update` | SSH | `POST /api/sites/{id}/core-update` |
| `vuln list/sites/core/backports` | Local scan via `internal/vuln` | `GET /api/vulns`, `/api/vulns/core`, `/api/vuln-backports`, formatted by `internal/vulnview`. `--slack` removed |
| `ignore list/add/remove` | `db.*` directly | `/api/ignore` endpoints, which already exist |
| `migrate-db` | Local `db.MigrateSplitDB` | Removed from the CLI. jman-api has its own |
| `update` | GitHub release + minisign | Unchanged. The `build-pkg` variant is removed |

## Phases

### Phase 1: API groundwork (done)

- `cli_tokens` table in `api.db`, CLI tokens in `AuthMiddleware`, and the login endpoints `POST /api/cli/authorize` (web session only) and `POST /api/cli/token`. Approved login codes live in memory for 2 minutes, so a restart only fails logins in progress.
- `GET /api/cli/tokens` (`?all=true` for admins), `DELETE /api/cli/tokens/{id}`, and `DELETE /api/cli/token` to revoke the token a request is made with (for `jman logout`).
- Web UI: the `/cli-login` approval page, and a CLI Tokens tab in Settings rather than on the profile page. Logging in now returns to the page that required it.
- `GET /api/cli/snapshot` with ETag and gzip. Each site carries its own `wp_core` and `update_locks`, so there are no separate top-level `core_versions` and `update_locks` lists. The last SpinupWP refresh time is sent in an `X-Jman-Refreshed-At` header instead of the body, so the ETag only changes with the data.
- `POST /api/refresh` at execute level. The existing `POST /api/providers/spinupwp/sync` stays at edit level for the web UI's sync button, but it, the new endpoint and the scheduler's fast tick all share one in-flight fetch.
- The version headers, `webUrl` in `GET /api/health`, and the manual-over-SpinupWP override by domain.

None of this changes existing behaviour, so it can ship on its own.

### Phase 2: CLI client layer (done)

- `internal/cliproto`: the snapshot types, request and response bodies and header names, shared by jman-api and the CLI without pulling in the database.
- `internal/client`: the HTTP client. It reads the version headers on every response and refuses to continue below the minimum version.
- `internal/credentials`: the token store, using the OS keyring (`zalando/go-keyring`, pure Go) with a mode-0600 `credentials.json` fallback, one token per API URL.
- `internal/mirror`: the local copy under `<data dir>/mirror/` with the freshness rules, backoff and stale warning, the offline version gate, the daily update notice, and lookups (`CliSites`, `Plugins`, `SitePlugins`, `CoreVersions`, `PluginName`, `IsUpdateLocked`, …) for Phase 3. Switching to another API URL deletes the old copy.
- `jman login [api-url]`, `jman logout` and `jman refresh`, and the timing keys in `config.toml`. The version gate runs before every command except `update`, `login`, `logout`, `help` and completion.
- The snapshot now also leaves out SpinupWP sites that are no longer in SpinupWP. The inventory sync only adds and updates, so it can still hold deleted sites; the old CLI handled this the same way.

### Phase 3: command migration (done)

- All commands read sites, servers and plugins from the local copy (`internal/mirror`). A global `--fresh` flag checks jman-api first however recent the copy is. Shell completion only reads the local copy.
- `plugin install`, `update` and `remove` and `core check` and `update` go through jman-api. Installs, updates and removals are queued as jobs, and jman waits for them and prints the results; Ctrl-C stops the waiting but not the jobs.
  - `plugin update` shows locked plugins and has `--allow-major` for one plugin on one site.
  - `plugin remove` deactivates an active plugin first, then uninstalls it, as `wp plugin uninstall --deactivate` did.
  - `core update --major` on an update-locked site also needs `--allow-major`.
- `plugin list` reads the local copy and marks locked plugins; `--live` reads the site over SSH instead. `plugin info` stays a live SSH read, since the local copy has no plugin details such as the author.
- `vuln list/sites/core/backports` format jman-api's reports with the new `internal/vulnview` package, which jman-api's daily Slack site report uses too. `--slack` was removed (a hidden flag explains why); jman-api already sends the reports.
- `ignore` reads and writes jman-api's ignore list. It used to write to the laptop's own `inventory.db`, so entries added with the CLI never reached jman-api.
- `wp`, `admin`, `mods` and `setup mu/compat` still run over SSH, with their sites from the local copy.

### Phase 4: dependency cleanup (done)

- `fetch` and `migrate-db` are gone, and the CLI no longer opens `inventory.db` or migrates a legacy `jman.db`. Old local data (`inventory.db`, the JSON caches) is left where it is on laptops; nothing reads it any more.
- `internal/wpcli` caches each site's admin user ID through an `AdminUserCache` interface. jman-api plugs in `db.AdminUserCache`; the CLI and jman-agent leave it unset and look the ID up live, as jman-agent effectively did already.
- `go list -deps ./cmd/jman` no longer contains `internal/db`, `internal/cache`, `internal/fetch/*`, `internal/sitestate`, `internal/vuln`, `internal/slack` or `modernc.org/sqlite`. `cmd/jman/deps_test.go` fails `make test` (and so CI) if any of them, or `internal/api`, comes back.
- `README.md`, `README_API.md` and `AGENTS.md` describe the login, the local copy, the new settings and endpoints, and drop `tokenSpinup` and `jman fetch` from the CLI docs.

### Phase 5: distribution and rollout

- Delete `aur/` and the `build-pkg` Makefile target, along with the build tag that excludes `update`.
- Release, then raise `min_cli_version` to that release once internal users have moved to the GitHub build. The last AUR version keeps working against SpinupWP for anyone outside the team.

## Risks

- **Server availability matters more.** Sites already in the mirror keep working offline, but new sites, vuln data and plugin updates need the API.
- **Plugin and core commands behave differently.** They become asynchronous jobs, and bulk updates skip major versions of locked plugins. Users need to know about this before Phase 3 ships.
- **Fewer sites from an incomplete mirror.** If the snapshot is missing a site or SSH detail that the old SpinupWP fetch had, commands target fewer sites without saying so. Compare the old and new target lists during Phase 3.
- **The loopback redirect can fail** on locked-down machines or behind browser policies. The fallback is printing the URL, and device code if it's ever needed.

## Verification

- `go list -deps ./cmd/jman | grep -E 'internal/db|sqlite'` is empty (`cmd/jman/deps_test.go`).
- With the API stopped, read-only commands work from the mirror with a stale-data warning, and API-only commands fail with a clear message without hanging.
- Two concurrent `jman refresh` calls cause one SpinupWP fetch, and a third within 60s causes none.
- A token unused for 90 days is rejected, and a revoked token is rejected on the next request.
- A CLI below `min_cli_version` refuses to run both online and offline, and `jman update` still works.
- A locked plugin is not major-updated by `jman plugin update`.
- A domain that exists as both a manual and a SpinupWP site appears once in the snapshot, with the manual site's details.
- `jman login` without a URL fails when `apiUrl` isn't set, and succeeds when it is.
