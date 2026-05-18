# Task System Specification

The `jman` task system provides a mechanism for creating, tracking, and automating units of work linked to various entities (Sites, Servers, Organizations, and Plugins).

## Task Data Model

A Task consists of the following fields:

- **ID**: Unique identifier (Auto-incrementing).
- **Type**: 
    - `one-time`: Created once, finishes when completed.
    - `repeating`: Calculates next due date as `previous due date + interval`.
    - `dynamic`: Calculates next due date as `completion date + interval`.
- **Status**: `pending`, `in_progress`, `completed`, `skipped`, `overdue`.
- **Priority**: `low`, `medium`, `high`.
- **Title**: Short summary.
- **Description**: Detailed information (can be auto-generated for vulnerabilities).
- **Linkage**:
    - `site_id`: Reference to a site.
    - `server_id`: Reference to a server.
    - `organization_id`: Reference to an organization.
    - `plugin_slug`: Reference to a specific plugin.
- **Assigned To**: Username of the assigned user.
- **Metadata**: JSON string for additional data (e.g., vulnerability UUIDs).
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

## Background Automation (Scheduler)

The `jman-api` process runs a background scheduler (every hour) that performs the following tasks:

### 1. Vulnerability Syncing
- Scans for vulnerabilities across all sites.
- If vulnerabilities exceed configured thresholds (`CVSSThreshold` or `VulnThreshold`):
    - **Update**: If an open vulnerability task for the site already exists, it updates the description, priority, and metadata.
    - **Create**: If no open task exists, it creates a new `one-time` task.
- **Priority Mapping**:
    - CVSS >= 7.0: `high`
    - CVSS >= 4.0: `medium`
    - Otherwise: `low`

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

## API Endpoints

All endpoints are prefixed with `/api`.

- `GET /tasks`: List tasks with filters (`status`, `priority`, `assigned_to`, `site_id`, `organization_id`, `server_id`, `search`).
- `GET /tasks/{id}`: Get task details.
- `POST /tasks`: Create a new task.
- `PATCH /tasks/{id}`: Update task fields (Title, Description, Status, Priority, etc.).
- `POST /tasks/{id}/complete`: Mark a task as completed (triggers recurrence logic).
- `DELETE /tasks/{id}`: Delete a task.

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
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tasks_site_id ON tasks(site_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_assigned_to ON tasks(assigned_to);
```
