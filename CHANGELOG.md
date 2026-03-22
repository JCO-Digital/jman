# Changelog

### v1.3.1 (2026-03-23)

#### Features

- store: centralize plugin enrichment and formatting logic (95522c6)
- data: Fetch and cache plugin metadata (3df2fb3)

## v1.3.0 (2026-03-21)

#### Features

- auth: Implement authentication flow (6f5119d)
- state: Integrate Pinia for global state management (f648daa)

#### Bug Fixes

- safer JSON parsing (1fe8aa2)
- more specific error handling (335c64b)

## v1.2.0 (2026-03-11)

#### Features

- plugins: add plugin listing and detail views (248ab17)

#### Bug Fixes

- correct favicon type and remove manual plugin name encoding (5d938ba)

#### Refactor

- ui: move styles to global CSS and update cache storage (f96fe05)

#### Maintenance

- replace vite favicon and update .env.example (631e94b)

### v1.1.1 (2026-02-26)

#### Styles

- views: format code and update container responsiveness (4818fb6)

## v1.1.0 (2026-02-26)

#### Features

- app: add footer to display application version (e241bd0)

### v1.0.3 (2026-02-26)

#### Continuous Integration

- release: update SSH key secret name (bd8a850)

### v1.0.2 (2026-02-26)

#### Continuous Integration

- release: use variables for known_hosts instead of secrets (16ac7a9)

### v1.0.1 (2026-02-26)

#### Build System

- specify pnpm as the package manager (8664d5f)

## v1.0.0 (2026-02-26)

#### Documentation

- readme: update with project overview and setup instructions (c2476eb)

#### Continuous Integration

- add release workflow and Makefile (bc81ef4)

#### Maintenance

- add postversion script (33f6e6f)
- remove PLAN.md (0e4d37c)

### Misc

- Vibe coded initial version. (219d120)
