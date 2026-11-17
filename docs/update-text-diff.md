# Update text diff

Proposed 2026-11-17. A check around every update that jman runs: compare
the site's visible text before and after, and report what changed so the
person who ran the update can spot breakage.

Status: backend built (`internal/textdiff`, `internal/updatejobs/text_diff.go`,
API in `docs/API_SPECS.md`). Web UI and CLI reporting are next. See
[Open questions](#open-questions).

## Why

Plugin updates sometimes break a site's frontend without any error. The
update succeeds, the page returns 200, and the monitor stays green. A recent
Polylang update broke the language switcher's markup, so the menu showed
`EN" >` next to the flag instead of `EN`. Nobody noticed until someone looked
at the page.

Screenshots were the first idea, but a visible-text diff catches this kind
of breakage without a headless browser on the jman-api host.

## Decisions

- **Visible text, not HTML or pixels.** A raw HTML diff is dominated by
  expected changes: `?ver=` strings, nonces, inline JSON config, generated
  class names (`wp-container-123`), builder data IDs and hashed bundle names.
  Pixels need a browser and are noisy too. The rendered text shows the
  Polylang bug as one changed line.
- **Pages:** the site's frontpage, plus extra paths configured per site
  (for example `/en/` on a Polylang site). Paths are on the site's own
  domain; full URLs aren't allowed.
- **Noise:** each page is fetched twice before the update. Lines that differ
  between those two fetches are treated as noise and left out of the diff.
- **Cache:** fetches bypass the page cache the way monitor checks do (a
  unique `jman_cache_bypass` parameter, `internal/monitor/http_check.go`).
- **Scope:** every update jman runs: plugin updates, ZIP updates (install
  jobs that replace a plugin) and core updates, from the CLI, the web UI and
  the nightly core run. There is no opt-out.
- **Granularity:** one before/after per job, around everything the job
  changes. A job covers one site, so this is per site per batch. The report
  lists what the job changed but doesn't try to say which plugin caused a
  diff.
- **Report only.** The check never blocks or rolls back an update, and never
  stops a bulk run.
- **False positives are acceptable** as long as the report makes the actual
  difference easy to see.
- **Mutes:** per site, on normalized lines, expiring after a settable time.

## Design

### Where it runs

In the job runner (`internal/updatejobs/runner.go`), around the change
itself:

1. `run` resolves the site as it does now.
2. **Baseline:** fetch every page twice and keep the extracted text.
3. Run the job (`runPlugins`, `runCore` or `runInstall`) unchanged.
4. **After:** fetch every page once.
5. Diff, apply noise and mutes, store the result on the job.

The job is marked finished before the after fetch, so its text diff has
its own status: `running` from the baseline until the after fetch is
stored, then `done`. A jman-api restart in between leaves it
`interrupted`.

The check's own failures (fetch errors, parse errors) never fail the job.
They are recorded in the diff result.

Fetching runs inside the job's worker, so it adds time per site. With
`Workers = 4` that's acceptable for a few pages per site, but it's worth
measuring (see Open questions).

### Fetching

- `GET https://<site domain><path>?jman_cache_bypass=<nanotime>`, with the
  monitor's cache bypass and redirect helpers (now in `internal/utils`).
- If the site's default uptime check has secret
  headers (basic auth on staging sites, for example), those headers are
  sent, and dropped on cross-host redirects as the monitor already does.
- Redirects are followed. The final URL is recorded, and if it differs
  between before and after (for example `/en/` now redirects to `/`), that
  is reported as a change.
- Timeout per fetch is 30 seconds.
- A page is an error, whatever its status, if it is WordPress's `wp_die()`
  page (`id="error-page"`: critical error, database connection error) or
  has a PHP fatal or parse error printed into it.

### Text extraction

Parse the HTML with `golang.org/x/net/html` and walk the DOM:

- Skip `script`, `style`, `noscript`, `template`, `svg` and HTML comments.
- Skip containers that visitors never see but whose text changes on every
  request. So far that's Gravity Forms' honeypot (`gform_validation_container`),
  which gets a random label (Name, Phone, LinkedIn, ...) on each render. Add
  to this list only for hidden content; visible random content is handled
  by noise filtering and mutes.
- Collect text nodes, plus `alt`, `title`, `aria-label` and `placeholder`
  attribute values and the `value` of submit buttons. The Polylang bug
  showed up in visible text, but a broken attribute often shows up in alt
  or aria text first.
- Emit one line per block-level element (`p`, `li`, `h1`–`h6`, `div`,
  `td`, `a` in navigation, and so on), with whitespace collapsed. Empty
  lines are dropped.

The result is a list of lines in document order. That's the stored
"snapshot" for a page; HTML isn't kept.

### Noise filtering

The two baseline fetches are diffed against each other. Any line that is
not identical in both is a **noisy line**: random testimonials, rotating
banners, "latest posts" blocks that shuffle, CSRF-style tokens that leak
into text.

When the before/after diff is computed, changes that only touch noisy
lines are dropped. Content that changes over time but not between two
quick fetches ("posted 3 minutes ago", a scheduled post going live) is not
caught by this and will show up as a false positive. Normalization (below)
removes most of it.

### Normalization

Lines are normalized before diffing and for mute matching:

- Collapsed whitespace, trimmed.
- Runs of digits become `#` (so dates, counts, prices and times match).
- Month and weekday names in English, Finnish (any inflection) and Swedish
  become `<d>`.

The report shows the original lines; normalization only decides whether
two lines are "the same".

### Diff

A line-level LCS diff between the normalized before and after lines, per
page (common prefix and suffix trimmed first; a huge differing middle is
reported as removed and re-added). For each changed hunk the report shows:

- removed and added lines, with a few lines of context;
- highlighting of the changed characters inside a line, so that a small
  change like `EN` → `EN" > EN` stands out;
- a link to the live page with the cache bypass parameter.

Page-level outcomes besides a text diff:

| Outcome | Meaning |
| --- | --- |
| `unchanged` | No differences after noise and mutes. |
| `changed` | Text differences remain, or the final URL after redirects changed. |
| `error` | The page worked before the update and doesn't after it: non-2xx, timeout, error page. Shown prominently. |
| `no_baseline` | Both baseline fetches failed, so there is nothing to compare. The update still ran. |

Each page also counts the changed lines it hid as noise or by a mute
(`hidden`).

### Mutes

A mute hides a line from future diffs on one site.

- **Key:** site, page path, direction (`added` or `removed`) and the
  normalized line. Direction is part of the key, so muting a line that keeps
  getting added doesn't hide it disappearing.
- **Expiry:** each mute has `expires_at`, set when it's created. The default
  comes from config (`textDiffMuteDays`, 30), and the UI lets the user
  pick another value.
- **Creating:** from a changed line in the report, with one click. The UI
  shows which lines a mute hides in later reports.
- Removing a mute sets `expires_at` to now. Expired mutes are kept for
  audit but no longer match.

### Per-site pages

A new per-site setting: a list of up to 20 paths (each starting with `/`,
no host or fragment). The frontpage is always included and can't be
removed. Edited on the site page
in the web UI and through the API; the CLI can list them.

### Storage

In api.db:

- `site_diff_pages`: `site_id`, `path`, `created_at`, `created_by`.
- `text_diff_mutes`: `id`, `site_id`, `path`, `direction`, `line`
  (normalized), `reason`, `expires_at`, audit fields.
- `plugin_update_jobs.text_diff`: the job's `models.TextDiff` as JSON. It is
  removed with its job (`jobRetention`, 7 days); the ledger entry stays.
  Job lists carry only its summary.

Baseline snapshots are only kept in memory for the duration of the job.

### Reporting

- **Job:** job lists carry `text_diff_summary` (pages checked, changed,
  errors, no baseline), and `GET /api/update-jobs/{id}` returns the full
  `text_diff`. The web UI's
  job view shows them.
- **Bulk runs:** the update view groups the jobs it started and shows a
  summary: sites with changes or errors first, unchanged sites collapsed.
- **CLI:** `jman plugin update` and `jman core update` print a one-line
  diff summary per site after the job finishes, with a link to the web UI
  for the hunks.
- **Nightly core:** `nightlySummary` adds the count of sites with changes
  and links to them.
- **Ledger (not built yet):** the ledger entry is written when the job
  finishes, before the after fetch, so noting "frontend text changed on N
  pages" there needs the entry updated afterwards.

No Slack alert for diffs: the person who ran the update reviews them.

## Measured noise

`scripts/textnoise` runs the extraction, normalization, noise filter and
diff described above against live sites, with no update in between, and
counts what the diff would report (the residual). Run 2026-11-17 on the 139
production WordPress sites in the jman cache, 4 fetches each, 15 s apart:

| Result | Sites |
| --- | --- |
| Measured (2 failed: one HTTP 500, one timeout) | 137 |
| No false positives | 110 |
| Only Gravity Forms honeypot labels | 25 |
| Random content from a large pool | 2 |
| Reordered content or changed redirects | 0 |

Pages averaged 133 text lines. Normalization made no difference on its
own, as no site showed digit- or date-only changes in that window. After
the honeypot rule (added after this run, then verified on five of the 25
sites), 135 of 137 sites are clean.

The two remaining sites rotate content from a pool larger than two baseline
fetches can learn: a testimonial carousel with 100+ quotes (72 lines) and a
randomized product list (7 lines). Per-site mutes cover them for now. If
more sites like this appear, treat lines that changed *position* between
the baseline fetches as noise, rather than only the changed lines.

## Open questions

- **Reordered content.** A sequential diff would report shuffled blocks as
  changes. None of the production sites showed reordering, so lines stay
  ordered.
- **Random pools.** See [Measured noise](#measured-noise): position-based
  noise if mutes turn out not to be enough.
- **Time per site.** Two baseline fetches plus one after, per page, inside
  the worker. Measure how much this adds to large bulk runs, and whether
  fetches for one site should run in parallel.
- **Plugin management jobs.** Activate, deactivate, delete and uninstall
  change the site too. They're not updates, so they're out of scope for now,
  but deactivation can break a page just as badly.
- **Bulk grouping.** Jobs are one site each and have no shared batch ID. The
  bulk summary needs one (a `batch_id` on jobs created together) or has to
  group on the client.
- **Which "fatal error" strings** to recognize beyond WordPress core's
  critical error page (PHP warnings printed into the page, white screen with
  an empty body).
- **Secret headers** live on monitor checks, not on sites. If a site needs
  headers but has no matching check, should the diff pages get their own
  setting?
