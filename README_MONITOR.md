# jman-monitor

`jman-monitor` is a sidecar utility for the `jman` toolset that provides automated uptime monitoring for your WordPress sites. It performs lightweight HTTP checks against all sites cached by `jman` and sends alerts via Slack when downtime is detected.

## How it Works

`jman-monitor` uses an intelligent state machine to balance low server load with high responsiveness to downtime.

### Monitoring States

Each site exists in one of three modes:

- **Normal Mode**: Sites are checked every 5 minutes. This is the default state.
- **Investigation Mode**: Triggered by a single failure. The site is checked every minute until there are either 3 consecutive failures (transition to Alert) or 3 consecutive successes (transition to Normal).
- **Alert Mode**: Triggered after 3 consecutive failures. The site is checked every minute to detect recovery as fast as possible. Slack alerts are sent at specific intervals (30-120 mins) while the site remains down.

### Load Spreading

When running in service mode, the monitor staggers (jitters) the initial checks for all sites over a 5-minute window on startup. This prevents "thundering herd" spikes where hundreds of sites are hit at the exact same second.

## Installation

`jman-monitor` is built alongside the main CLI:

```bash
make build
# The binary will be available at ./bin/jman-monitor
```

## Usage

### Service Mode (Recommended)

To run as a continuous background process with the internal scheduler:

```bash
./bin/jman-monitor --service
```

This mode handles its own scheduling, staggering, and state transitions.

### One-off Mode

You can run a single check of all sites manually (useful for testing or legacy cron setups):

```bash
./bin/jman-monitor
```

### Flags

- `-s`, `--service`: Run as a continuous background service.
- `-v`, `--verbose`: Enable verbose output (shows mode transitions and check results).
- `-d`, `--debug`: Enable debug output (shows every request being made).

## Configuration

`jman-monitor` shares the configuration file with `jman` (typically `~/.config/jman/config.toml`).

| Key                   | Default        | Description                                                   |
| :-------------------- | :------------- | :------------------------------------------------------------ |
| `slackToken`          | -              | Your Slack Bot User OAuth Token.                              |
| `slackMonitorChannel` | `slackChannel` | The Slack channel to send monitoring alerts to.               |
| `monitorThreshold`    | `3`            | Number of consecutive failures before sending an alert.       |
| `monitorTimeout`      | `10`           | Timeout in seconds for each HTTP check.                       |
| `monitorCacheBypass`  | `false`        | Enable frontend cache bypass (adds a random query parameter). |
| `ignoreSites`         | `[]`           | (Deprecated) List of domains to skip. Migrated to DB.         |

Example `config.toml` snippet:

```toml
slackToken = "xoxb-your-token"
slackMonitorChannel = "#ops-alerts"
monitorThreshold = 5
monitorTimeout = 15
monitorCacheBypass = true
# ignoreSites = ["dev.example.com", "staging.example.com"] # Now managed via CLI/API
```

## Alerting Logic

- **Down Alert**: Sent when a site enters **Alert Mode** (after 3 consecutive failures).
- **Repeated Alerts**: While a site is in Alert Mode, Slack notifications are repeated to prevent them from being forgotten:
  - **500 Errors**: Every 30 minutes.
  - **400 Errors**: Every 60 minutes.
  - **Other Errors**: Every 120 minutes.
- **Recovery Alert**: Sent as soon as a site in Alert Mode returns a successful 2xx status code. The site then transitions back to **Normal Mode**.
- **Concurrency**: The monitor uses a worker pool of up to 24 concurrent goroutines to process checks.

## Running as a Service (systemd)

A systemd service file is provided in the repository. To install it:

1. Copy the binary to `/usr/local/bin/jman-monitor`.
2. Copy `jman-monitor.service` to `/etc/systemd/system/`.
3. Reload and start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable jman-monitor
   sudo systemctl start jman-monitor
   ```

## Ignored Sites Management

The list of ignored sites is now stored in the database. You can manage it using the `jman` CLI:

- **List ignored sites**: `jman monitor list`
- **Ignore a site**: `jman monitor ignore <domain> [reason]`
- **Unignore a site**: `jman monitor unignore <domain>`

Sites previously defined in `config.toml` under `ignoreSites` are automatically migrated to the database on the first run of `jman-monitor`.

## API Endpoints

The `jman-api` provides several endpoints for monitoring data (all require JWT authentication):

- `GET /api/monitor/history?hours=48`: Returns status history for all sites for the last X hours.
- `GET /api/monitor/status?domain=...`: Returns the current health status and failure count for a site.
- `GET /api/monitor/ignored`: Lists all ignored sites and their reasons.
- `POST /api/monitor/ignored`: Add a site to the ignore list.
- `DELETE /api/monitor/ignored/{domain}`: Remove a site from the ignore list.
