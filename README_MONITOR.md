# jman-monitor

`jman-monitor` is a sidecar utility for the `jman` toolset that provides automated uptime monitoring for your WordPress sites. It performs lightweight HTTP checks against all sites cached by `jman` and sends alerts via Slack when downtime is detected.

## How it Works

The monitor reads the list of sites from the local `jman` cache. For each site, it:

1. Performs an HTTP GET request to the site's domain.
2. Tracks the success or failure of the request.
3. Increments a failure counter if the site is unreachable or returns a non-2xx status code.
4. Sends a Slack alert if the failure count reaches a configured threshold.
5. Tracks state (failures and alert times) in the local SQLite database to avoid spamming alerts and provide historical data.

## Installation

`jman-monitor` is built alongside the main CLI:

```bash
make build
# The binary will be available at ./bin/jman-monitor
```

## Usage

You can run a single check manually:

```bash
./bin/jman-monitor
```

For continuous monitoring, it is recommended to run `jman-monitor` as a cron job or a systemd timer (e.g., every 5-15 minutes).

### Flags

- `-v`, `--verbose`: Enable verbose output (shows failure counts and individual check results).
- `-d`, `--debug`: Enable debug output (shows every site being checked).

## Configuration

`jman-monitor` shares the configuration file with `jman` (typically `~/.config/jman/config.toml`).

| Key                   | Default        | Description                                             |
| :-------------------- | :------------- | :------------------------------------------------------ |
| `slackToken`          | -              | Your Slack Bot User OAuth Token.                        |
| `slackMonitorChannel` | `slackChannel` | The Slack channel to send monitoring alerts to.         |
| `monitorThreshold`    | `3`            | Number of consecutive failures before sending an alert. |
| `monitorTimeout`      | `10`           | Timeout in seconds for each HTTP check.                 |
| `ignoreSites`         | `[]`           | (Deprecated) List of domains to skip. Migrated to DB.   |

Example `config.toml` snippet:

```toml
slackToken = "xoxb-your-token"
slackMonitorChannel = "#ops-alerts"
monitorThreshold = 5
monitorTimeout = 15
# ignoreSites = ["dev.example.com", "staging.example.com"] # Now managed via CLI/API
```

## Alerting Logic

- **Down Alert**: Sent when a site exceeds the `monitorThreshold`. After the initial alert, it will only re-alert once per hour if the site remains down.
- **Recovery Alert**: Sent as soon as a site that was previously marked as "down" returns a successful 2xx status code.
- **Concurrency**: The monitor checks up to 24 sites in parallel to ensure fast execution even for large site lists.

## Ignored Sites Management

The list of ignored sites is now stored in the database. You can manage it using the `jman` CLI:

- **List ignored sites**: `jman monitor list-ignored`
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
