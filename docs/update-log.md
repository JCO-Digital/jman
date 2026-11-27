# Update attempt log

jman-api appends every plugin, ZIP and core update attempt to a log file, one JSON object per line. It's for developers working out why updates fail, especially premium plugins: whether a plugin always fails or only sometimes, on which PHP versions, and what wp-cli actually printed. Nothing in jman reads it.

## Where it is

`updateLog` in jman-api's config sets the file. Empty (the default) means `update-attempts.jsonl` in jman-api's data directory (`$XDG_DATA_HOME/jman`, usually `~/.local/share/jman`); `off` turns the log off. The file is created with mode `0600`. It isn't stripped of secrets: premium plugins' download URLs, with their license keys, can be in the output.

jman-api opens the file for each job and closes it again, so logrotate can move it without `copytruncate`. For jman-api running as the user `jman` (adjust the path):

```
/home/jman/.local/share/jman/update-attempts.jsonl {
	weekly
	rotate 26
	compress
	missingok
	notifempty
}
```

## What a line holds

One line per attempt, which is one update command of a job:

| `type` | Command |
| :--- | :--- |
| `plugins` | `wp plugin update` of the job's plugins without an update lock, in one call |
| `patch` | `wp plugin update --patch` of locked plugins whose update comes from wordpress.org |
| `zip` | `wp plugin install --force` of a ZIP over an installed plugin (uploaded, downloaded or copied from another site) |
| `core` | `wp core update`, and `wp core update-db` when it succeeded |

Installing a new plugin isn't an update and isn't logged. Neither is a plugin a job skipped because its update needs a newer WordPress or PHP than the site has (`update_blocked`, see `docs/API_SPECS.md`): wp-cli isn't run for it. Neither is a job that never reached wp-cli (the site couldn't be found, or the locks couldn't be read); the job itself records those.

```json
{
  "time": "2026-11-24T02:10:04Z",
  "job_id": 5120,
  "job_kind": "plugins",
  "created_by": "nightly",
  "type": "plugins",
  "site_id": "6467c171-4e72-5d82-b430-fb50868b9bf1",
  "site": "example.com",
  "server": "vanillastag",
  "wp_version": "6.8.3",
  "php_version": "8.3",
  "cli_php_version": "8.1.27",
  "failed": true,
  "duration_ms": 14210,
  "results": [
    {"name": "gravityforms", "old_version": "2.9.1", "new_version": "2.9.1", "status": "failed", "error": "Warning: Download failed. Unauthorized; Error: No plugins updated (1 failed)."},
    {"name": "wp-rocket", "old_version": "3.17", "new_version": "3.17", "status": "Updated", "version_unchanged": true},
    {"name": "akismet", "old_version": "5.3", "new_version": "5.4", "status": "Updated"}
  ],
  "calls": [
    {"args": ["plugin", "update", "gravityforms", "wp-rocket", "akismet"], "stdout": "…", "stderr": "…", "error": "Error: No plugins updated (1 failed).", "duration_ms": 14210}
  ]
}
```

- `wp_version` is the cached WordPress version when the job started (for `core`, the version before the update).
- `php_version` is the site's PHP-FPM version from the inventory; `cli_php_version` is the PHP wp-cli ran on, which is what ran the update. They can differ: wp-cli uses whatever `php` the site user's shell finds. jman-api asks for it with `wp cli info` after the job.
- `failed` is true when a wp-cli call failed, a result failed, or a result has `version_unchanged`: wp-cli reported a success but the version installed afterwards (from the refreshed plugin list or core version) is the old one. Some premium updaters do that when the download isn't the release they announced. For `zip`, it also flags reinstalling the same version on purpose.
- `wp plugin update` runs without `--format=json`: with it, wp-cli silences the upgrader and a failed plugin only says `Error`. Its plain table is parsed instead, so `stdout` holds the upgrader's progress lines and `stderr` its warnings ("Download failed. Unauthorized", "some files could not be copied").
- A `plugins` call covers several plugins, and one plugin's fatal error can fail the whole call, so a failed result isn't always that plugin's own fault. `calls` shows which.
- Plugin update jobs queued back to back on a site run together in one call (`docs/update-queue.md`). Their attempt is logged once, under the first job's `job_id`, with every job in `job_ids` and all their plugins in `results`; `created_by` is the first job's user. A job that ran on its own has no `job_ids`.
- `calls` is only there when `failed` is true. Each stream is capped at 256 KiB, keeping the start and most of the end.

The same output is kept with the job for its 7 days (`calls` on `GET /update-jobs/{id}`), and the web UI shows it from the task center ("Show output").

## Reading it

```sh
# Every failed attempt of one plugin
jq -c 'select(.results[]?.name == "gravityforms" and .failed)' update-attempts.jsonl

# Attempts and failures per plugin
jq -r '.results[] as $r | [$r.name, (if $r.status == "failed" or $r.version_unchanged then "failed" else "ok" end)] | @tsv' update-attempts.jsonl | sort | uniq -c

# wp-cli's stderr of one job
jq -r 'select(.job_id == 5120) | .calls[]?.stderr' update-attempts.jsonl
```
