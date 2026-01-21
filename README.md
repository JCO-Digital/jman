# jman

`jman` is a command-line utility designed to manage WordPress sites hosted on SpinupWP, with additional support for MainWP integration. It provides a streamlined way to fetch site data, run remote `wp-cli` commands, manage plugins, and create administrative users across multiple sites.

## Features

- **SpinupWP Integration**: Fetch and list site/server data directly from the SpinupWP API.
- **Remote WP-CLI**: Execute `wp-cli` commands on remote sites via SSH.
- **MainWP Support**: Automated MainWP Child plugin installation and site management.
- **Bulk Operations**: Perform actions like disabling file modifications or installing plugins across multiple sites.
- **Site Aliases**: Generate YAML-based alias files for SSH and WP-CLI, supporting both individual sites and server-based groups.
- **Local Caching**: Optimized performance by caching site and server metadata locally.

## Installation

The best way to install `jman` is via `bun`:

```bash
bun install -g @jcodigital/jman
```

### Prerequisites

- Bun (v1.2 or later recommended)
- SSH access configured for your SpinupWP servers.

### Building from source

1. Clone the repository:

   ```bash
   git clone https://github.com/JCO-Digital/jman.git
   cd jman
   ```

2. Install dependencies:

   ```bash
   bun install
   ```

3. Build the project:
   ```bash
   bun run build
   ```

The binary will be generated at `./dist/jman`.

## Configuration

`jman` uses a TOML configuration file located in your XDG config directory (typically `~/.config/jman/config.toml` on Linux).

Create the file and add your credentials:

```toml
tokenSpinup = "your_spinupwp_api_token"
tokenMainwp = "your_mainwp_api_token" (optional)
urlMainwp = "https://your-mainwp-dashboard.com" (optional)
```

## Usage

```bash
jman <command> [target] [args...]
```

### Local Caching

To speed up operations, `jman` caches site and server data from SpinupWP. If you add new sites or servers, you should run the `fetch` command to update your local cache:

```bash
jman fetch
```

### Available Commands

| Command    | Description                                                                         |
| :--------- | :---------------------------------------------------------------------------------- |
| `fetch`    | Fetch latest data from SpinupWP and update local cache.                             |
| `list`     | List cached data from SpinupWP.                                                     |
| `wp`       | Run a `wp-cli` command on a target site.                                            |
| `search`   | Search for a specific term across sites.                                            |
| `admin`    | Create a new administrator user on target sites.                                    |
| `plugin`   | Install a plugin on target sites. Supports WordPress.org slugs or custom repo URLs. |
| `mods`     | Set `DISALLOW_FILE_MODS` to true on target sites.                                   |
| `alias`    | Create SSH/WP-CLI alias files for all sites or a filtered collection.               |
| `inactive` | List sites that don't have an active MainWP Child connection.                       |
| `mainwp`   | Install and configure MainWP on sites.                                              |

### Examples

**Run WP-CLI on a site:**

```bash
jman wp mysite.com plugin list --status=active
```

**Install a plugin on a site:**

```bash
jman plugin mysite.com akismet
```

**Create an admin user:**

```bash
jman admin mysite.com myusername user@example.com
```

**Generate WP-CLI aliases for a server group:**

```bash
# This outputs a YAML structure compatible with WP-CLI's alias configuration
jman alias my-server-name > ~/.wp-cli/alias.yml
```

## Development

- **Run in development mode:** `bun run dev`
- **Linting:** `bun run test`
- **Formatting:** `bun run format`

## License

GPL-3.0-only
