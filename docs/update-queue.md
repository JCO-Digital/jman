# Update queue

jman-api runs every site change (plugin updates and management, ZIP
updates, core updates, mu-plugin installs, WordPress users) as a background
job. Jobs wait in one queue, kept in memory and in `plugin_update_jobs`, and
a set of workers take them from it. The scheduling is in
`internal/updatejobs/queue.go`.

## Rules

1. **One job per site at a time, in order.** A site's jobs run in the order
   they were queued; a job never overtakes an older job on the same site.
   Jobs on one site can depend on each other (install, then activate), and
   two jobs on one site at once would fight over maintenance mode and the
   text diff.
2. **Priority.** Each job has a priority: `low`, `normal`, `high` or
   `urgent`. Sites are ranked by the highest priority among their queued
   jobs, so a high job pulls its site's older jobs forward with it.
3. **Round-robin between servers.** Among sites of equal rank, the server
   that started a job longest ago goes first, so a bulk run queued on one
   server doesn't hold back the same priority on the others. Within a
   server, equal ranks run in queue order. A site jman can't place on a
   server takes turns on its own. Priority always comes first: a server
   that just had its turn still runs its high job before another server's
   normal one.
4. **Aging.** A `low` job counts as `normal` once it has waited 30 minutes,
   so a long run of normal jobs can't hold it back for ever. Aging never
   goes past `normal`.
5. **Per-server limit.** At most `PerServer` (2) jobs run on one server at a
   time, as many sites share a server. The express worker may go one over,
   so a server busy with bulk jobs doesn't hold a high job back. Sites jman
   can't place on a server have no limit.
6. **Express worker.** Besides the `Workers` (4) general workers,
   `ExpressWorkers` (1) only take sites that have a `high` or `urgent` job.
   Jobs can't be interrupted, so without it a high job would wait for a
   general worker to finish a slow bulk job.
7. **Combined runs.** When a worker takes a site whose first job is a plugin
   update, the plugin update jobs right behind it on the site join it, up to
   `MaxPluginsPerRun` (50) plugins and as long as they agree on
   `allow_major`. The first job of another kind ends the group. The group
   runs as one `wp plugin update` call with one text diff and one exposure
   sync; each job still gets its own results and its own ledger entry
   (with its own user), and they all get the same text diff and the wp-cli
   output of the shared call when it failed. The update log
   (`docs/update-log.md`) has each call once, with every job in
   `job_ids`. The jobs record the first job's ID as `batch_id`.

## Default priorities

The API sets the priority from the request; a client can ask for another.

| Request | Priority |
| :--- | :--- |
| One site: plugin update, activate/deactivate/delete/uninstall, install, core update, user create/delete | `high` |
| Several sites: plugin updates, ZIP updates, mu-plugin installs | `normal` |
| A plugin update where a listed plugin has an open vulnerability exposure on the site | `high` |
| The nightly core update | `low` |
| `urgent` | only on request, by an admin |

The CLI queues `jman plugin update` one site per request; with several
sites it sends `"bulk": true` so they get the bulk default.
`jman plugin update --priority <level>` sets the priority explicitly.

## Changing a priority

`PATCH /api/update-jobs/{id}` with `{"priority": "high"}` changes a queued
job's priority (`execute` level; `urgent` needs `admin`). The task center's
**Prioritize** action on a queued normal or low job sets it to `high`.

## Queue position

Queued jobs returned by `GET /api/update-jobs` and
`GET /api/update-jobs/{id}` carry `position`, their approximate place in
the queue (1 runs next), worked out by playing the queue out with the same
rank and round-robin rules. It doesn't account for busy sites, server
limits or combined runs, so it's an estimate.

## Restart

Queued jobs survive a jman-api restart: they're loaded back in ID order
with their stored priority. Running jobs are marked interrupted, every job
of a combined run separately.
