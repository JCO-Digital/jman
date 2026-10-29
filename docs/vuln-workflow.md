# Vulnerability workflow

Decided 2026-10-29. Replaces the per-site daily Slack report and the
auto-generated vulnerability Tasks with a vulnerability view under
Monitoring.

## Why

- The daily per-site report (`vuln.SendSiteReports`, sent once a day at
  `vulnReportTime`) was fine while few vulnerabilities were open. Now it is
  too long: common plugins repeat on many sites, and some vulnerabilities
  can't be patched.
- Auto-generated Tasks (`tasks.SyncVulnerabilities`, one per site) were meant
  to spread the work, but in practice one person updates plugins per plugin,
  not per site, so closing the Tasks is just an extra step. Task updates also
  post to Slack through `NotifyTaskChange`.
- The new-vulnerability Slack alert (`vuln.AlertNewVulnerabilities`) is
  useful and stays.

## Design

### Unit

The open item is a **vulnerable plugin or core version**, not a single
vulnerability: "plugin X has known vulnerabilities on N sites". It lists the
affected sites with their installed versions and the vulnerabilities each
version has.

State is kept per (kind, slug, site), where kind is `plugin` or `core`. For
core, the slug is `wordpress` and the installed version decides which
vulnerabilities apply. A site's exposure resolves automatically when it no
longer runs an affected version (it updated, deleted the plugin, or a backport
rule or ignore entry now covers it). The group resolves when no site is left.

New table, separate from `incidents`: `vuln_exposures`

- `kind`, `slug`, `site_id`, `version` (installed)
- `vuln_uuids` (JSON, the unsuppressed vulnerabilities that apply), `max_cvss`
  (NULL when none is scored)
- `fix_state`: `fix_available` | `no_fix`
- `fix_available_since`: starts the overdue clock (reset when the state changes)
- `first_seen_at`, `last_seen_at`, `resolved_at`

It is synced on the refresh scheduler's slow tick (`runSlowTick`), in the
place where `tasks.SyncVulnerabilities` runs now, reusing
`vuln.ProcessVulnerabilities` / `ProcessCoreVulnerabilities`, so the existing
ignore list, suppression and backport rules apply unchanged.

### Fix state

From the wpvulnerability.net data already parsed into `models.Operator`:

- **Fix available:** every applicable vulnerability has `max_operator == "lt"`
  (the same rule as `vuln.FixedIn`). The lowest safe version is shown.
- **No fix:** any applicable vulnerability has `unfixed == "1"` or has no
  `lt` bound.
- **Hint, not a state:** on a no-fix item, show "newer version available,
  might fix it" when the latest version (wp.org `plugin_info.version`, or the
  site's `site_plugins.update_available`) is newer than the highest affected
  version.
- Core: fix available when the site has a minor or major update
  (`SiteCore.MinorUpdate` / `MajorUpdate`).

### Closed label

wpvulnerability.net returns closure at the plugin level:
`"closed": 1, "closed_reason": "security-issue", "closed_date": "2022-12-27"`.
`models.VulnData` doesn't parse these yet; add them. Don't parse the
"(closed)" / "(unfixed)" suffix in vulnerability titles. Show the reason and
date on the label. `latest` in the same payload is a Unix timestamp of the
last update, not a version (it is typed as `Latest *string`), and is useful
next to the label ("closed, last updated 2021").

### Overdue (red)

A fix-available exposure turns red when `fix_available_since` is older than N
days, where N depends on the highest CVSS among its vulnerabilities.
Unscored vulnerabilities use the medium tier. Defaults, set with
`vulnDueCriticalDays`, `vulnDueHighDays`, `vulnDueMediumDays` and
`vulnDueLowDays`:

| CVSS                 | N       |
| -------------------- | ------- |
| ≥ 9.0                | 2 days  |
| ≥ 7.0                | 7 days  |
| ≥ 4.0 or unscored    | 14 days |
| < 4.0                | 30 days |

No-fix items never turn red: they need a decision (remove the plugin, or
mute).

Escalation is only visual: red on the page plus a nav badge counting
unmuted red exposures. No Slack, no PagerDuty.

### Mutes

New table `vuln_mutes`, soft delete like `scan_path_acks`:

- Scope: one plugin (or core) on one site: `kind`, `slug`, `site_id`
- `reason` required; `created_by/at`, `updated_by/at`, `removed`
- `fix_state_at_mute`, `vuln_uuids_at_mute`, `latest_version_at_mute`
- `review_at` = created + 90 days
- Status: `active` | `needs_review` | `lifted` (+ `lifted_reason`)

Lifecycle:

- **Muting a fix-available item** is allowed, with a warning in the dialog
  (e.g. a client pins a version, or the update breaks the site).
- **Lifted automatically** when a new vulnerability applies that wasn't in
  `vuln_uuids_at_mute`, for any mute.
- **Lifted automatically for no-fix mutes** when the item becomes fix
  available, or a newer version than `latest_version_at_mute` appears. This
  trigger does not apply to fix-available mutes, because every release would
  lift them.
- **After 90 days**, the mute goes to `needs_review`: it shows again but
  doesn't count toward red until someone re-confirms it with a reason (which
  starts a new 90 days) or lets it lapse.
- A mute covers only that site; installing the plugin on another site is a
  newly affected site.

Muted sites stay visible in their group, collapsed, with the reason.

Mutes accept risk, so they need the exec level, like the scan allowlist and
triage endpoints.

### Kept as they are

- Backport rules (`vuln_backports`, `PatchedSites`)
- UUID ignore list (faulty slugs or version ranges in the feed), plugin, site
  and server ignores, and `Suppressed`
- The `jman vuln` CLI and its `vulnThreshold` / `cvssThreshold` handling

### Slack

Only new-vulnerability alerts, for:

1. A new vulnerability (today's alert)
2. A known vulnerability on a newly affected site (installed or downgraded)
3. A new vulnerability on a muted plugin (which also lifts the mute)

`vuln_alerts` dedups per UUID, so cases 2 and 3 need dedup per
(uuid, kind, site). Keep the seeding so the first run doesn't send the
backlog. Today the alert covers plugins only; core is added. The alert keeps
the `cvssThreshold` filter.

Removed: `SendSiteReports`, `runDailySiteReport`, `vulnReportTime`, the
`vuln_site_report_last_date` setting.

### Tasks

- Removed: `tasks.SyncVulnerabilities` and the default vulnerability-assignee
  setting (`DefaultVulnAssigneeSettingKey`, `vuln_settings_handlers.go`, its
  UI in GeneralSettings).
- Kept: a "Create task" action on a group or site row in the new view, with
  the title and description prefilled. Tasks are created manually only.
- Migration: on deploy, mark open auto-generated tasks (created by `system`,
  `vuln_uuids` in metadata) as `skipped` in bulk, without per-task Slack
  notifications (`SkipAutoVulnTasks`).
- Check what uses `web/src/utils/taskVulnerability.ts`, `TaskInfoModal` and
  `TaskReminderWidget` once there are no new auto-generated tasks.

### UI

#### Monitoring layout

Today the two areas are built differently. Uptime is spread over three
top-level tabs (Checks, Active Incidents, Incident History). Security is one
top-level tab (`SecurityPanel`) with its own segmented sub-bar (Incidents,
Informational, History, Allowlist, Acknowledgements). Adding Vulnerabilities
as a third style would make it worse, so all three get one shape:

- **Top-level tabs:** Summary · Uptime · Security · Vulnerabilities. The
  area tab badges count what needs attention (open uptime incidents, open
  security incidents, red exposures).
- **Summary** is the default tab. It has one card per area with its
  needs-attention count, the few most urgent items (open incidents,
  red exposures, mutes needing review) and a link to the area. When
  everything is clear, it shows what is being watched (number of checks,
  sites scanned, plugins tracked).
- **The same segmented sub-bar in every area.** The first segment is always
  "what needs attention now" and is the default. The other views come next,
  then history, then the configuration and exceptions:

| Area            | Needs attention | Other         | History  | Config / exceptions         |
| --------------- | --------------- | ------------- | -------- | --------------------------- |
| Uptime          | Incidents       | Checks        | History  | Rules                       |
| Security        | Incidents       | Informational | History  | Allowlist, Acknowledgements |
| Vulnerabilities | Open            | —             | Resolved | Muted (incl. needs review)  |

- Uptime's Rules (`MonitorRulesModal`) becomes a section with a table, like
  Security's Allowlist. The create and edit forms stay modals.
- Vulnerabilities' Open section has a fix-state filter (all / fix available /
  no fix) and a "closed only" toggle. No fix is not a separate section.

- **One toolbar layout:** search and filters on the left, the area's actions on
  the right (like `MonitorChecksPanel`'s `checks-toolbar`), and a "Show
  removed" toggle where soft-deleted entries exist.
- **One empty state** per "needs attention" segment ("All clear" + what is
  being watched).
- **URL:** `?tab=<area>&section=<segment>`, with redirects for the current
  `?tab=checks|active|history|security` links (incl. `/incidents`). Slack
  and PagerDuty links to incidents keep working.
- Uptime moves into a `UptimePanel` that matches `SecurityPanel`, so
  `MonitoringView` becomes only the tab shell. Shared pieces (sub-bar,
  toolbar, empty state) become components in `components/monitoring/`.

This restructure ships before the Vulnerabilities tab (phase 3a), so the new
tab is built on it from the start.

#### Vulnerabilities tab

Groups by plugin or core, sorted red first, then by CVSS. Each group shows the
label (fix available / no fix / closed), the lowest safe version or the
newer-version hint, the affected sites with versions, and mute and
create-task actions. The Monitoring nav badge also counts red exposures.

`DashboardView`'s `VulnerabilityWidget` and the plugin pages
(`PluginVulnerabilityList`) can link to the tab.

## Phases

1. **Cut the noise** (PR #107): remove the daily report and `SyncVulnerabilities`, and
   bulk-close the open auto-generated tasks. This doesn't depend on the rest.
2. **Data** (`feat/vuln-exposures`): parse plugin `closed*` fields, `vuln_exposures` sync with fix
   state and overdue tiers, read API (`GET /api/vulns/exposures`, summary
   count for badges).
3. **UI and mutes:**
   a. Monitoring layout: Summary and area tabs, shared
      sub-bar/toolbar/empty state, `UptimePanel`, Rules as a section, URL
      redirects. The Summary gets its vulnerability card in 3b.
   b. Vulnerabilities tab, nav and tab badges, `vuln_mutes` with the
      lifecycle above, manual task creation.
4. **Slack:** per-site dedup, newly affected site and muted-plugin alerts,
   core alerts.

## Open questions

- The CVSS tier defaults above are a first guess; tune them once real data
  is in.
- Is the red count combined with the active incident count in one Monitoring
  nav badge, or shown as a second badge?