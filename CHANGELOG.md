## [4.14.1](https://github.com/JCO-Digital/jman/compare/v4.14.0...v4.14.1) (2026-03-23)


### Bug Fixes

* **admin:** Use normal verbosity for user creation messages ([d9219bc](https://github.com/JCO-Digital/jman/commit/d9219bc1416824cf2a2b71527f7d0748dc53fc53))
* **wpcli:** Pass skip parameter to GetPlugins ([230b8c6](https://github.com/JCO-Digital/jman/commit/230b8c65097ebeb73d3f232a79b902f3bddb00f1))



# [4.14.0](https://github.com/JCO-Digital/jman/compare/v4.13.0...v4.14.0) (2026-03-22)

## 4.15.0 (2026-03-23)

#### Features
- mods: Add ability to enable/disable file mods (4669dd5)

#### Bug Fixes
- cache: sanitize plugin metadata by decoding entities and stripping tags (1596077)

#### Refactor
- cache: make JSON cache TTL configurable (e100418)

#### Maintenance
- release: v4.14.0 [skip ci] (5b72dcb)
- release: v4.13.0 [skip ci] (dbea3ba)
- release: v4.12.1 [skip ci] (5b0b84e)

### v4.14.1 (2026-03-23)

#### Bug Fixes
- wpcli: Pass skip parameter to GetPlugins (230b8c6)
- admin: Use normal verbosity for user creation messages (d9219bc)

#### Refactor
- wpcli: Introduce CliOptions struct (a7378d5)

#### Documentation
- Add prerequisites to README (0d6a432)
- Update installation instructions (371a014)

#### Maintenance
- release: v4.14.1 [skip ci] (66b03e7)

## v4.14.0 (2026-03-22)

#### Features
- cache: Refactor plugin info update logic (29b37fc)
- api: Refactor response handling and add plugin info endpoint (1f4e1bb)
- cache: Add version comparison for plugin updates (0b070b8)
- cache: Add plugin info caching and fetching (f6d4003)

#### Bug Fixes
- cache: sanitize plugin metadata by decoding entities and stripping tags (2593d75)
- cache: Remove Latest field update from WPVuln (6854d95)

#### Refactor
- cache: make JSON cache TTL configurable (6648422)

#### Maintenance
- release: v4.14.0 [skip ci] (8c939de)

## v4.13.0 (2026-03-20)

#### Features
- cmd: Add CLI tools for user and credential management (ebcd5c7)
- api: Implement JWT authentication and rate limiting (5f14d56)

#### Maintenance
- release: v4.13.0 [skip ci] (cc52714)

### v4.12.1 (2026-03-20)

#### Bug Fixes
- update: Use AppVersion for current version check (218093b)

#### Maintenance
- release: v4.12.1 [skip ci] (7ae2041)

## v4.12.0 (2026-03-20)

#### Features
- plugin: colorize site names in output (d1d6a52)
- plugin: Add plugin alias support (7d2aeda)

#### Bug Fixes
- plugin: Correct site name formatting in remove output (8d885c7)
- wpcli: Ensure error is returned from RunWP (64ce754)
- cache: Improve plugin fetch error reporting (0edde0a)
- wpcli: Improve error handling for WP-CLI commands (f51f99e)
- plugin: Improve error handling for plugin operations (114b17d)

#### Maintenance
- release: v4.12.0 [skip ci] (d846159)

## v4.11.0 (2026-03-19)

#### Features
- wpcli: Return new version and language from UpdateCore (a6fc4bb)

#### Bug Fixes
- wpcli: Return structured data from UpdateCore (b7769c1)

#### Refactor
- verb: Rename ansi to verb and move ANSI color functions (ec587e5)
- verbosity: Rename verbosity package to verb (ca2481f)

