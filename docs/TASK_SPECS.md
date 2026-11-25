# Task System Specification

The `jman` task system provides a mechanism for creating, tracking, and automating units of work linked to various entities (Sites, Servers, Organizations, and Plugins).

## Task Data Model

A Task consists of the following fields:

- **ID**: Unique identifier (Auto-incrementing).
- **Type**: 
    - `one-time`: Created once, finishes when completed.
    - `repeating`: Calculates next due date as `previous due date + interval`.
    - `dynamic`: Calculates next due date as `completion date + interval`.
- **Status**: `pending`, `in_progress`, `on_hold`, `blocked`, `completed`, `skipped`, `overdue`.
- **Priority**: `low`, `medium`, `high`.
- **Title**: Short summary.
- **Description**: Detailed information.
- **Linkage**:
    - `site_id`: Reference to a site.
    - `server_id`: Reference to a server.
    - `organization_id`: Reference to an organization.
    - `plugin_slug`: Reference to a specific plugin.
- **Assigned To**: Username of the assigned user.
- **Metadata**: JSON string for additional data (e.g., vulnerability UUIDs).
- **Previous task**: for a repeating or dynamic task made by completing another, the ID of that one (`previous_task_id`).
- **Checklist**: ordered items, each a line of text that can be ticked (by whom and when is kept). An item can stand for a site, server, plugin or inventory item (`ref_type`/`ref_id`); its text is then that record's name when it was added, so it still reads right after the record is gone. In the web UI, "Add from…" on the checklist picks sites, servers, plugins or inventory items, one item each; sites can be narrowed to a server, an organization or the sites running a plugin, and all of them added at once. An item whose site or server is gone shows as removed.
- **Watchers**: users who are told about changes and notes besides the assignee (see Notifications).
- **Notes**: the shared notes system, with parent type `Task`.
- **Interval**: Duration string supporting standard Go durations and custom units: `d` (days), `w` (weeks), `m` (30 days), `y` (365 days).
- **Dates**:
    - `created_at`: Creation timestamp.
    - `due_date`: Deadline.
    - `reminder_date`: When the task starts appearing in reminder lists/notifications.
    - `completed_at`: Actual completion timestamp.
    - `last_notified_at`: Last time a Slack reminder was sent.
    - `updated_at`: Last update timestamp.

## Recurrence Logic

When a task of type `repeating` or `dynamic` is marked as `completed`:
1. The system calculates the `next_due_date` based on the `interval`.
2. For `repeating` tasks, it uses `previous_due_date + interval`.
3. For `dynamic` tasks, it uses `now + interval`.
4. If the original task had a `reminder_date`, the new task will have a `reminder_date` with the same lead time relative to the new `due_date`.
5. A new task instance is created with status `pending`.
6. The new task gets the same checklist with nothing ticked, the same watchers, and `previous_task_id` pointing back. Notes stay on the task they were written on.

## Background Automation (Scheduler)

The `jman-api` process runs a background task scheduler (every hour) for reminders and cleanup.

### 1. Vulnerability Syncing (removed)
- Vulnerability Tasks are no longer created automatically: one Task per site didn't match how vulnerable plugins are updated (per plugin, across sites), and the daily per-site Slack report is gone too. See `docs/vuln-workflow.md` for what replaces them.
- On upgrade, the one-time `skip_auto_vuln_tasks` migration marks the open Tasks the old sync created (`Security Vulnerabilities - <site>`, created by `system`) as `skipped`, without notifications.
- One-off Slack alerts for new vulnerabilities and newly affected sites run on the refresh scheduler's slow tick (see `README_API.md`).
- Vulnerability Tasks can still be created by hand.

### 2. Task Reminders
- Sends Slack notifications for `pending` tasks that have reached their `reminder_date`.
- Only `high` and `medium` priority tasks trigger notifications.
- **Routing**:
    - If `assigned_to` is set, it looks up the `slack_id` in the `settings` table for that user and sends a DM.
    - Otherwise, it sends to the channel configured in `SlackTasksChannel`.
- Updates `last_notified_at` to prevent duplicate alerts in the next tick.

### 3. Orphan Cleanup
- Periodically checks for tasks linked to `site_id` or `server_id` that no longer exist in the cache.
- Orphaned tasks are automatically marked as `skipped`.

## Notifications

Besides the reminders above (which fire later, once `reminder_date` arrives), the assignee and the task's watchers get a Slack DM when a task is created, edited, (re)assigned, completed or gets a note — via `POST /tasks`, `PATCH /tasks/{id}`, `POST /tasks/{id}/complete` and `POST /notes`.

- **Watchers**: whoever creates a task watches it, and so does whoever writes a note on it, unless they untick "Watch this task" (`"watch": false`). Anyone can watch or unwatch a task (`PUT`/`DELETE /tasks/{id}/watch`). The assignee is told whether or not they watch.
- **No self-notifications**: no one hears about their own change or note.
- **Grouped**: messages are held per task and recipient, and sent once the task has been left alone for a minute, or five minutes after the first change at the latest. So editing a task in place one field at a time sends one message. A field changed twice shows its first old and last new value; one changed back drops out. Someone who stopped watching (or was unassigned) in the meantime isn't sent anything. Held messages live in memory, so a jman-api restart (a deploy) within that window drops them.
- **Created, or (re)assigned to someone**: the new assignee gets the full task (title, type, priority, status, description, due/reminder dates, recurrence interval), as it is when the message goes out.
- **Other changes**: only the fields that changed (status, priority, title, description, due date, reminder date, interval, assignee), with title and type/priority always included for context.
- **Notes**: the note's author and text.
- Routing: a Slack DM via the recipient's `slack_id` setting, with no channel fallback. Checklist changes don't notify.

## API Endpoints

All endpoints are prefixed with `/api`.

- `GET /tasks`: List tasks with filters (`status`, `priority`, `assigned_to`, `site_id`, `organization_id`, `server_id`, `search`).
- `GET /tasks/{id}`: Get task details.
- `POST /tasks`: Create a new task.
- `PATCH /tasks/{id}`: Update task fields (Title, Description, Status, Priority, etc.).
- `POST /tasks/{id}/complete`: Mark a task as completed (triggers recurrence logic).
- `DELETE /tasks/{id}`: Delete a task, with its checklist, watchers and notes.
- `POST /tasks/{id}/checklist`, `PATCH`/`DELETE /tasks/{id}/checklist/{itemId}`, `PUT /tasks/{id}/checklist/order`: the checklist (see `API_SPECS.md`).
- `PUT`/`DELETE /tasks/{id}/watch`: watch or unwatch as the signed-in user.

## Database Schema

```sql
CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    priority TEXT NOT NULL DEFAULT 'medium',
    title TEXT NOT NULL,
    description TEXT,
    site_id INTEGER,
    server_id INTEGER,
    organization_id INTEGER,
    plugin_slug TEXT,
    assigned_to TEXT,
    metadata TEXT,
    interval TEXT,
    due_date DATETIME,
    reminder_date DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    last_notified_at DATETIME,
    created_by TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    previous_task_id INTEGER
);

CREATE TABLE task_checklist_items (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    text TEXT NOT NULL,
    done INTEGER NOT NULL DEFAULT 0,
    done_by TEXT,
    done_at DATETIME,
    ref_type TEXT,          -- site | server | plugin | inventory_item
    ref_id TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_by TEXT
);

CREATE TABLE task_watchers (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, username)
);

CREATE INDEX idx_tasks_site_id ON tasks(site_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_assigned_to ON tasks(assigned_to);
```
