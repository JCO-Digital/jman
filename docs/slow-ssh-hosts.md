# Slow SSH hosts (WP Engine)

Some hosts, notably WP Engine's SSH gateway (`*.ssh.wpengine.net`), are slow over SSH. Each WP-CLI call can take tens of seconds before WordPress has even started. jman makes several WP-CLI calls per plugin update, and each opens its own SSH connection. On these hosts that adds up to timeouts.

There are two independent fixes: reuse SSH connections, and give those hosts longer WP-CLI timeouts.

## 1. Reuse SSH connections

WP-CLI's `--ssh` runs the system `ssh`, which reads `~/.ssh/config`. That makes connection reuse a pure SSH setting, with no jman code involved. Add this to the SSH config of the user that runs jman: the jman-api service user on the server, and your own user for laptop CLI use.

```
Host *.ssh.wpengine.net
    ControlMaster auto
    ControlPath ~/.ssh/cm-%C
    ControlPersist 10m
    ServerAliveInterval 30
```

- `ControlMaster auto` makes the first connection to a host a master. Later `ssh` processes reuse it instead of doing a new handshake and authentication.
- `ControlPath ~/.ssh/cm-%C` names the socket after a hash of the connection details, so each site user gets its own master. Keep it in a directory only that user can read; `~/.ssh` is.
- `ControlPersist 10m` keeps the master open for 10 minutes after the last command. A batch of updates to one site then shares a single connection.
- `ServerAliveInterval 30` stops the gateway dropping an idle master.

To check it, run the same command twice against one site and compare timings:

```bash
time wp --ssh=<user>@<user>.ssh.wpengine.net --path=<path> core version
time wp --ssh=<user>@<user>.ssh.wpengine.net --path=<path> core version
ssh -O check <user>@<user>.ssh.wpengine.net   # "Master running"
```

To undo it, remove the block and run `ssh -O exit <target>` for any open masters.

This only helps if the handshake is the slow part. If the second run is no faster, the time goes into WordPress starting up on WP Engine's side, and only longer timeouts help.

## 2. Longer WP-CLI timeouts

jman uses two timeouts:

- Reads (plugin lists, version checks): 1 minute.
- Writes (plugin and core installs, updates, removals): 10 minutes. Killing a write midway can leave a plugin half-installed or the site in maintenance mode, so a long wait is the safer failure.

`wpcliHostTimeouts` in `config.toml` raises the timeout, in minutes, for every WP-CLI call to matching SSH hosts. Matching uses a shell glob on the host, ignoring the user and port. Each entry raises timeouts and never lowers them, so writes keep at least 10 minutes.

```toml
[[wpcliHostTimeouts]]
host = "*.ssh.wpengine.net"
minutes = 5
```

Plugin updates in the web UI are synchronous HTTP requests. If jman-api sits behind a reverse proxy, the proxy's read timeout must be longer than the slowest update. nginx's default `proxy_read_timeout` is 60 seconds and will otherwise answer with a 504 while the update is still running.

## When an update fails or times out

After a failed plugin update, jman checks the site:

- the installed version of each plugin it tried to update (`wp plugin list --skip-plugins`)
- whether the site is in maintenance mode (`wp maintenance-mode is-active`)

The failure message and the site update ledger record what the check found. In the API, if the update timed out but the new version is installed and the site is not in maintenance mode, it is reported as a successful update.

A site left in maintenance mode shows its maintenance page until WordPress ignores the `.maintenance` file, 10 minutes after the update started. Once you're sure the update has stopped, `wp maintenance-mode deactivate` clears it immediately.
