# Changelog

## 5.43.0 (2026-09-04)

#### Features

- components: add secondary copy functionality to InfoCard (ed8077d)
- site: display system user in site detail view (f6ee054)
- assets: add license key support (d387794)
- SiteDetailView: add expand/collapse toggle for ledger entries (ec45ece)
- site: implement site update ledger (224ebda)
- reports: add support for enddate parameter type (0cbff9e)
- reports: add report list and runner views (d91a1cb)
- components: increase hourly traffic data retention window to 7 days (4281eef)
- ui: add copy-and-extract functionality to asset description (2b11613)
- assets: add payment methods management (69816fb)
- ui: display status codes in site traffic card (2621e51)
- traffic: add monthly view to site analytics (152d505)
- traffic: add interactive site traffic chart (fae032a)
- components: implement site traffic analytics card (c8c343d)
- settings: display agent version in token list (0676d8c)
- settings: switch server ID input to selection list (555b80d)
- settings: add agent token management and site metrics (941a209)
- views: display disk space usage for servers (5075f2f)
- task: add vulnerability status tracking to tasks (32e057e)
- assets: standardize list view with sorting, pagination, and metadata (b5e9e86)
- notes: support string parent IDs and link plugin notes by slug (6a2aded)
- notes: focus note input on add and integrate notes in plugin detail view (0389fe9)
- notes: position Add Note button in card header (1fd0cc2)
- notes: collapse add-note form and truncate note list to recent 3 items (2aaed64)
- notes: implement reusable notes widget on organization and site views (120f501)
- assets: add asset type filter and optimize column density (5a8405c)
- sites: add organization name tooltip to sites table (019cd0a)
- site: add archived environment support (5ae55cf)
- site: add environment classification and batch editing (d756428)
- plugin: add support for updating vulnerable plugins (ef6a8ae)
- view: update header actions in TemplatesView (74d20ea)
- asset: add organization selection and linking to asset management (070e7a4)
- components: implement reusable AssetEditModal component (0dccdcc)
- assets: add edit modal to asset list (de52cc0)
- task: display completion details in task info modal (f5412bf)
- settings: add slack reminder time configuration (a9749d0)
- ui: implement asynchronous confirmation modal (d4be1e7)
- dashboard: implement modular and reorderable widget system (4bea0bd)
- style: add table column width and text alignment utility classes (4e9e16e)
- vulnerability: implement suppression status for ignored vulnerabilities (94f0e3b)
- vulnerability: add copy to clipboard functionality for vulnerability UUIDs (63b3313)
- store: optimize ignore lookup logic using computed index (06c22cd)
- settings: add edit functionality for ignore rules (b46a964)
- ignore: implement unified ignore management system (b191593)
- site: add database table prefix to site info details (ec8ae39)
- settings: add slack integration and replace modal close buttons with icons (3dfe80e)
- components: implement AppIcon component and replace inline SVGs (2963145)
- task: improve task management and user assignment (9a4e7d0)
- tasks: implement task management system (9336b04)
- ui: display update result status in modal badges (0cd7b68)
- PluginSiteUpdateModal: add support for updating only vulnerable plugin instances (40cc4bc)
- plugin: add modal to update plugin across sites (d3dba88)
- data: update local plugin version after successful update request (b668783)
- plugin: add update modal for site plugins (d65431c)
- settings: synchronize settings with API (f687387)
- dashboard: add vulnerability dashboard widget (d3e7ad9)
- auth: add admin user level and restrict user management access (c5a0dca)
- settings: allow password resets in user edit modal (caa9c6b)
- user: add loading and error states for profile fetching (8c5f9a1)
- settings: add user management and 2FA configuration (a168fdc)
- ui: implement toast notifications for error handling (507257a)
- auth: implement role-based access control with level-based permissions (417442f)
- assets: implement asset and subscription management module (26b4b06)
- audit: add audit trail to organization view (ccda9aa)
- organization: sync site links on organization fetch (24a6292)
- organization: add debounce and request cancellation to search functionality (f46a89e)
- EditableInfoCard: add async save support and loading state (50d54ee)
- organization: display linked sites in organizations table (3fb85da)
- data: implement local caching for site-company associations (b494b18)
- site: implement site and company linking functionality (c80bf4e)
- company: implement company and contact management system (81cdea9)
- monitor: render 24h proportional status timeline and update uptime calc (eeb4ae5)
- monitor: display ignored domains and pending live status (e6e1601)
- settings: add settings view and store with refresh interval controls (3c4ddb8)
- monitor: add monitor store and history UI component (6b6cd14)
- components: add refresh button to AppNav and copy-to-clipboard in InfoCard (e45fa5a)
- components: add reusable UI components and integrate into views (7279d3f)
- data: add vulnerabilities to enriched types and surface them in UI (8b7e921)
- vulns: add vulnerability support to store and UI (5bedf9c)
- router: Introduce Dashboard and Sites views (7de3123)
- router: add query params for pagination (dd95dd4)
- data: Add enrichedSites computed property (af3ccd0)
- store: centralize plugin enrichment and formatting logic (95522c6)
- data: Fetch and cache plugin metadata (3df2fb3)
- auth: Implement authentication flow (6f5119d)
- state: Integrate Pinia for global state management (f648daa)
- plugins: add plugin listing and detail views (248ab17)
- app: add footer to display application version (e241bd0)

#### Bug Fixes

- ci: pin pnpm version for pnpm/action-setup (18734f0)
- plugin: allow update modal interaction for installed plugins (be84d83)
- ui: update asset identifier helper text (3e9904a)
- assets: improve loading states and add asset management details (3edf6e0)
- components: adjust traffic window based on data retention policy (b91546b)
- ui: update status badge logic in update modals (1382187)
- notes: resolve color readability issues in dark mode (7c1d177)
- dashboard: wire up edit action in task reminder widget (99fdfdb)
- organization: fix plugin audit visibility and prefill convert-to-asset modal (fe96449)
- auth: log server response body on login failure (9a0b944)
- dashboard: prevent null item insertion during drag and drop reordering (4c0f9fc)
- vulnerability: exclude suppressed vulnerabilities from display and data processing (6493610)
- task: improve task filtering and vulnerability suppression naming (119ae13)
- components: add domain-based lookup fallback for monitor history ignore status (af1d51c)
- site: remove public folder field from site details (990df09)
- ui: improve date parsing logic in TaskFormModal (de2cfff)
- tasks: update current date reference and improve code formatting (84beac5)
- settings: validate password strength in user form submission (57795ce)
- navigation: rename assets route to inventory (ef5ce4d)
- organization: update phone placeholder format (c9a2ae1)
- component: allow falsy values in EditableInfoCard (7d4fd54)
- monitor: prevent marking history as fetched on error (439a342)
- timeline: render history blocks using duration-based flex-grow and ensure visibility (47fc639)
- components: preserve fractional uptime percentage and format to two decimals (b532903)
- monitor-history-card: treat error_code 0 as unknown and add status-unknown style (b0213d2)
- monitor: show tiny/zero-duration timeline items and make status checks case-insensitive (2b54626)
- components: avoid accessing first_seen when sortedHistory is empty (4878228)
- data: Display server name or "Unknown Server" (69aae28)
- types: remove unused vue-router import (1a6bda5)
- safer JSON parsing (1fe8aa2)
- more specific error handling (335c64b)
- correct favicon type and remove manual plugin name encoding (5d938ba)

#### Performance Improvements

- data: add computed lookup maps and replace repeated filters with map lookups (2313857)

#### Refactor

- views: improve layout of asset form fields (04b8b46)
- components: improve null safety in TrafficChart path calculations (0cf7086)
- settings: remove v-prefix from agent version display (7ab09ca)
- ui: manage asset price as string input (4422ac3)
- organization: extract modal components from detail view (8e3c54a)
- assets: decouple asset management logic from organization details (1bb781c)
- style: move component-specific styles to global CSS (47b63f7)
- icons: convert SVG assets to Vue components (1cc60fb)
- api: centralize BASE_URL definition (482306f)
- api: use centralized error handler in stores (385da54)
- data: update vulnerability enrichment logic in store (315ab31)
- data: normalize vulnerability data structures (cbe2580)
- data: use backend-provided suppression status for vulnerabilities (e587d3c)
- components: reorder script blocks for consistency (d325282)
- sites: reorder reactive state declarations in SitesView (8c91278)
- view: type InfoItem for site info items in SiteDetailView (3054990)
- ui: update organization tables to use standardized components and utilities (1752638)
- style: modularize and standardize CSS architecture (511f415)
- dashboard: move vulnerability widget position (7047a7d)
- types: replace PluginUpdate with Plugin interface (c0746ca)
- plugin: move update logic to Pinia store (3012723)
- settings: move ignored domains to separate component and tab (e9f54b2)
- style: remove inline styles in favor of scoped CSS classes (40daa90)
- organization: rename companies to organizations (32267b4)
- vulnerabilities: move inline styles into scoped CSS classes in PluginVulnerabilityList.vue (9c5cf8e)
- components: rename types, compress CSS, move vulnerability list in detail view (be3a1cd)
- ui: move styles to global CSS and update cache storage (f96fe05)

#### Documentation

- AGENTS: update icon implementation details and document intentional design decisions (9281562)
- add project styling guidelines documentation (3baab7b)
- add CHANGELOG.md (5cc2d8d)
- readme: update with project overview and setup instructions (c2476eb)

#### Styles

- refactor: format codebase with Prettier (233c094)
- ui: standardize AppIcon rendering and asset action button (4bd87c3)
- components: remove text-transform from badge-sm (d73b4c9)
- site: add visual separator to plugins header (10c2b70)
- ui: implement responsive design for mobile screens (a5f5db7)
- views: format code and update container responsiveness (4818fb6)

#### Build System

- specify pnpm as the package manager (8664d5f)

#### Continuous Integration

- release: update SSH key secret name (bd8a850)
- release: use variables for known_hosts instead of secrets (16ac7a9)
- add release workflow and Makefile (bc81ef4)

#### Maintenance

- wire up web/ (jman-ui) in the monorepo CI/build (9772f61)
- store: update vulnerabilities cache key (a75d409)
- docs: remove deprecated API_SPECS.md file (3b0f352)
- lint: configure ESLint and Prettier (275da79)
- config: add foonver.toml to enable push and changelog (5b79cbd)
- replace vite favicon and update .env.example (631e94b)
- add postversion script (33f6e6f)
- remove PLAN.md (0e4d37c)

### Misc
- Add 'web/' from commit 'fdebfc33c6424f6f97e7279f2d955110a2c8b264' (17c983a)
- Vibe coded initial version. (219d120)

### v5.42.1 (2026-09-02)

#### Bug Fixes

- db: refuse startup whenever the legacy jman.db still exists (6a00ec9)

#### Refactor

- separate jman CLI and jman-api into independent processes/data (7e63b8c)

#### Continuous Integration

- release: deploy jman-api binary to server after release build (ae7bbff)

## v5.42.0 (2026-09-02)

#### Features

- api: add ledger logging for site plugin updates (0a9ac19)

#### Bug Fixes

- api: update ledger status to partial on plugin updates (3c9ee18)

## v5.41.0 (2026-08-31)

#### Features

- db: add license key field to assets and organization assets (7ada15b)

#### Tests

- db: ensure correct yesterday date for traffic rollup test (b0b749b)
- db: add tests for asset license key inheritance logic (7d6db7f)

## v5.40.0 (2026-08-28)

#### Features

- api: implement site update ledger (345dcf2)
- commands: add setup compat command (a837e90)

## v5.39.0 (2026-08-24)

#### Features

- reports: add upcoming billing report (3be5484)
- api: implement report generation framework (64ec520)

## v5.38.0 (2026-08-24)

#### Features

- monitor: Add agent staleness detection and alerts (8b98adc)

## v5.37.0 (2026-08-21)

#### Features

- api: add payment method support to assets (3f06eec)

### v5.36.1 (2026-08-20)

#### Bug Fixes

- agent: handle nil maps when loading legacy state (4a07583)

## v5.36.0 (2026-08-20)

#### Features

- agent: track HTTP status codes and improve log filtering (3bac2a4)

## v5.35.0 (2026-08-18)

#### Features

- api: add support for monthly site traffic aggregation (ee4b2e1)

## v5.34.0 (2026-08-18)

#### Features

- agent: exclude admin and API paths from top pages list (e52c1cf)

## v5.33.0 (2026-08-18)

#### Features

- agent: aggregate historical traffic logs into daily entries (d248d05)

## v5.32.0 (2026-08-18)

#### Features

- logs: filter internal referrers and prune hourly traffic (263f26a)

## v5.31.0 (2026-08-18)

#### Features

- agent: rotate site processing order to ensure fair budget usage (a20985c)

### v5.30.3 (2026-08-18)

#### Bug Fixes

- agent: correct traffic date comparison and improve logging (e4c8a90)

### v5.30.2 (2026-08-18)

#### Bug Fixes

- agent: implement backlog catch-up scheduling (4839628)

### v5.30.1 (2026-08-18)

#### Bug Fixes

- agent: apply traffic report budget to live log processing (e09660d)

## v5.30.0 (2026-08-18)

#### Features

- agent: bound log processing to prevent oversized reports (7f10a80)

## v5.29.0 (2026-08-18)

#### Features

- agent: implement log-based traffic tracking and collection (4a09098)

## v5.28.0 (2026-08-18)

#### Features

- agent: trigger fast-path self-update via manifest (c013894)

### v5.27.1 (2026-08-17)

#### Refactor

- agent: use fixed files directory for site path resolution (eb01ae6)

## v5.27.0 (2026-08-17)

#### Features

- agent: improve site path resolution and manifest filtering (f48a94d)
- agent: track agent versions and improve path resolution (1db6f18)

## v5.26.0 (2026-08-17)

#### Features

- agent: introduce jman-agent for local server monitoring (6748bc5)

#### Documentation

- readme: add documentation for jman-agent (e6f2488)

### v5.25.1 (2026-08-17)

#### Bug Fixes

- tasks: unescape plugin names in vulnerability sync (17ca993)

## v5.25.0 (2026-08-14)

#### Features

- notes: migrate parent_id to string to support slug-based plugin notes (738f259)

## v5.24.0 (2026-08-14)

#### Features

- notes: add support for Plugin parent type in Go API (37f4ee3)
- assets: include asset type in organization assets query (bc40771)

## v5.23.0 (2026-08-06)

#### Features

- api: add archived site environment type (b2e1c13)

## v5.22.0 (2026-08-06)

#### Features

- api: implement site environment classification (1d6dfcc)

## v5.21.0 (2026-08-05)

#### Features

- vuln: add WordPress core vulnerability scanning (4d01c38)

#### Bug Fixes

- cache: parse SQLite DATETIME as RFC3339 in plugin freshness check (7786443)
- cache: parse SQLite DATETIME as RFC3339 in core version freshness check (8defd77)

### v5.20.5 (2026-08-04)

#### Bug Fixes

- config: rename UsersConfig lock methods to avoid implementing sync.Locker (4184062)
- commands: don't treat non-interactive EOF as update confirmation (e5c2709)
- cache: guard cache/data file paths against traversal (d332fef)
- config: enforce file permissions on config.toml (b371513)
- db: make CompleteTask's completion check atomic (4e62b3e)
- cmd: run deferred cleanup before process exit (211095e)
- api: only trust X-Forwarded-For/X-Real-IP from configured trusted proxies (9fc63bd)
- wpcli: shell-quote arguments passed to RunSSH (952f9e7)
- update: enforce signature verification and harden downloads (79fa498)
- config: initialize lock when constructing UsersConfig outside LoadUsersConfig (044f459)
- wpcli: force refresh of WordPress update transient before plugin updates (7f06b83)

### v5.20.4 (2026-08-04)

#### Bug Fixes

- api: return completed task object in CompleteTask handler (b8c57fa)

### v5.20.3 (2026-08-03)

#### Continuous Integration

- github: allow AUR publication steps to fail (544a14a)

### v5.20.2 (2026-08-03)

#### Maintenance

- api: add air configuration for live-reloading (bf37394)

### v5.20.1 (2026-06-04)

#### Bug Fixes

- api: verify plugin version against remote state when update check reports up to date (5f664bb)

## v5.20.0 (2026-06-04)

#### Features

- api: include error details in plugin update responses (cad8a37)

#### Bug Fixes

- api: improve error handling and response consistency for plugin updates (d54dff8)

## v5.19.0 (2026-06-04)

#### Features

- api: trigger slack notification upon task assignment (20c3671)

#### Bug Fixes

- api: prevent data races when notifying task assignees (690067a)
- tasks: improve reminder time parsing and validation (c1842b7)
- api: implement configurable Slack reminder times and reset notification status on reassignment (87352eb)

#### Performance Improvements

- tasks: implement caching for user reminder times to avoid N+1 queries (2d85854)

## v5.18.0 (2026-05-26)

#### Features

- commands: add major version update support for wordpress core (9bb786b)

## v5.17.0 (2026-05-22)

#### Features

- vuln: refactor vulnerability reporting to support multiple vulnerabilities per plugin (be828a0)

#### Bug Fixes

- vuln: ignore suppressed vulnerabilities and fix CVSS pointer assignment (f78b64a)
- vuln: recompute CVSS score after filtering vulnerabilities (6b3768e)

## v5.16.0 (2026-05-22)

#### Features

- cache: add suppressed field to plugin site data structure (cf1b153)
- api: track user who completed tasks (b890156)

#### Refactor

- task: move completion logic to database repository (d65c9cb)

## v5.15.0 (2026-05-21)

#### Features

- api: add granular vulnerability suppression logic and documentation (d1e6443)

#### Bug Fixes

- api: stop forcing capitalization on plugin update error messages (254bd34)
- api: clean up plugin update error messages (1db7337)

## v5.14.0 (2026-05-18)

#### Features

- db: implement unified ignore system for monitor and vulnerabilities (896077b)

#### Bug Fixes

- api: force new ID on ignore entry creation (0fbf0df)

#### Refactor

- ignore: optimize ignore matching with in-memory matchers (ea2b087)

#### Documentation

- ignore: document the unified ignore list system (c02203e)

### v5.13.1 (2026-05-18)

#### Documentation

- tasks: add task system specification documentation (82a9790)

## v5.13.0 (2026-05-18)

#### Features

- config: add SlackTasksChannel support for task notifications (5729e43)

#### Bug Fixes

- tasks: only update last notified timestamp if slack message is sent successfully (81ef046)
- api: implement strict field validation for task updates (6726af0)
- api: allow partial task updates by checking field presence in request body (53c8873)
- api: allow basic access to user list and make task description nullable (47091fd)

#### Documentation

- project: add AGENTS.md documentation file (03222f4)
- api: update documentation for ListUsersHandler functionality (c956765)
- api: remove duplicate user creation documentation (ed10896)

#### Continuous Integration

- github: add CI workflow and rename release workflow (3a47d0a)

## v5.12.0 (2026-05-14)

#### Features

- task: add last_notified_at field and prevent duplicate reminders (340624b)
- tasks: implement task management system (4ebce87)

#### Bug Fixes

- scheduler: handle errors when fetching cache during orphaned task cleanup (05ebd31)
- db: prevent redundant update when completing already completed task (7d49ce6)
- scheduler: use correct site version when grouping vulnerabilities (d09ed96)

### v5.11.2 (2026-05-13)

#### Bug Fixes

- db: improve table migration robustness (fa3ca21)

### v5.11.1 (2026-05-13)

#### Bug Fixes

- wpcli: use json.Decoder for robust output parsing (e8ae39a)
- wpcli: sanitize output before JSON unmarshaling to handle stray notices (95ecc88)

## v5.11.0 (2026-05-12)

#### Features

- api: add endpoints to list and update site plugins (4d5aaa1)

#### Bug Fixes

- api: handle plugin save errors and improve cache persistence logic (2441e22)
- db: handle iteration errors in GetSitePluginLastUpdates (7d5eb93)
- api: refresh plugin cache after site update (5688003)
- api: validate plugin slug format in SitePluginUpdateHandler (8683ff0)
- cache: prevent unnecessary plugin re-fetches when timestamps are missing (36ff4c4)
- cache: implement TTL-based plugin fetching logic (4b83e5e)

## v5.10.0 (2026-05-08)

#### Features

- api: implement user settings management endpoints (ebd2e3a)

#### Bug Fixes

- api: return 404 for missing settings on PATCH requests (e6337bf)
- db: enforce non-null value constraint and handle optional settings safely (f08b986)

#### Tests

- api: add unit tests for settings handlers (f7c7ffd)

### v5.9.1 (2026-05-07)

#### Bug Fixes

- db: correct column existence check during table migration (55b71cb)
- db: add primary keys to schema and handle potential errors in plugin checks (542f4e1)

#### Refactor

- db: migrate plugin cache from JSON files to SQLite (8064bae)

## v5.9.0 (2026-05-07)

#### Features

- backup: include duration in success log message (639cff0)
- backup: implement automated hourly database backup scheduler (a19a269)

## v5.8.0 (2026-05-07)

#### Features

- api: add proxy support and enhance security (253cd8e)
- auth: introduce admin user level (53c8eea)
- api: allow admins to update user passwords (23fa4be)

#### Bug Fixes

- api: security across authentication and database layers (abf4fae)
- api: secure 2FA setup and implement rate limiting on password changes (1423687)
- auth: implement token revocation and harden user management (4cb10b2)

## v5.7.0 (2026-05-06)

#### Features

- api: add username normalization and validation (eff3f35)
- api: implement user level validation and update CORS policies (7b3ad38)
- api: implement get profile endpoint (65c8345)
- api: enforce password strength requirements (f0358de)
- api: implement user management and self-service features (d362aac)

#### Bug Fixes

- auth: increase minimum password entropy requirement (451b2ac)
- api: require TOTP code for 2FA deactivation (1254376)

#### Refactor

- config: implement thread-safe access and atomic file writes (bc2da41)

## v5.6.0 (2026-05-05)

#### Features

- api: implement role-based access control with user levels (d9a9243)
- api: automate next_billing updates and add explicit override (0a57c06)
- api: add endpoint to list all organization assets (5689010)
- asset: add active status and lifecycle tracking to assets (948e6e5)
- asset: implement asset management system (cc0e718)

## v5.5.0 (2026-05-04)

#### Features

- plugin: add summary report for multi-site plugin updates (f853d54)

## v5.4.0 (2026-05-04)

#### Features

- api: add endpoint to list users (4d3d9f2)

## v5.3.0 (2026-05-03)

#### Features

- api: add endpoint to list sites by company (287faa2)
- api: implement company, contact, and note management (6709e02)

#### Bug Fixes

- api: improve cache handling and strengthen input validation (17b228b)

#### Refactor

- api: rename company entities to organization (f19d01a)

### v5.2.1 (2026-04-30)

#### Bug Fixes

- slack: strip ANSI escape codes from messages before sending (2b2d4b4)

## v5.2.0 (2026-04-29)

#### Features

- vuln: add ignore list functionality (bbe686a)

#### Refactor

- vuln: update search logic and enhance terminal output styling (2129a6b)

### v5.1.1 (2026-04-28)

#### Refactor

- completions: optimize and improve site completion logic (6daeb31)

## v5.1.0 (2026-04-28)

#### Features

- setup: add command to install or update bojaco mu-plugin (a06de8b)

#### Refactor

- plugin: split plugin command into subcommands and add concurrency safety (055015a)

#### Maintenance

- aur: remove .SRCINFO file (1097d62)

### v5.0.6 (2026-04-28)

#### Continuous Integration

- aur: switch to KSXGitHub/github-actions-deploy-aur action (52ee39c)

### v5.0.5 (2026-04-28)

#### Documentation

- update documentation for CLI changes and remove MainWP (80dd831)

#### Continuous Integration

- aur: update deployment action and add .SRCINFO (7e2604a)
- github: switch to shmew/aur-deploy action (f68c6c8)

### v5.0.4 (2026-04-28)

#### Continuous Integration

- aur: fix deployment action version and improve update script (c4120c5)

### v5.0.3 (2026-04-28)

#### Build System

- completions: move output directory to project root (37ca32b)

#### Continuous Integration

- aur: automate publishing to AUR (b6016c6)

### v5.0.2 (2026-04-28)

#### Refactor

- commands: unify site completions and rework vuln command (c11319e)

### v5.0.1 (2026-04-28)

#### Continuous Integration

- github: prefix release tag with v (d878287)

## v5.0.0 (2026-04-28)

## 5.0.0 (2026-04-28)

#### Features

- Release new version (66362a2)
- add shell completions (BREAKING CHANGE) (590dec9)

#### Documentation

- readme: add Security & Updates section and update command description (befde72)

#### Build System

- makefile: set JMAN_TOKENSPINUP placeholder for shell completions (35d7e0e)

#### Continuous Integration

- github: update foonver action version to v0.9.1 (a6766b3)
- migrate release pipeline to foonver (0ab53be)

## v4.26.0 (2026-04-27)

#### Features

- commands: add shell completion for monitor and wp commands (48d846b)
- search: add fast cache-backed site/plugin search and fast cache readers (4f0883f)
- plugin: suggest cached plugin names for subcommand argument completion (8604125)
- commands: accept action before target and add shell completion for mods command (7fa885e)
- fetch: add shell completion for fetch command and rename target to operation (6ba12bd)
- commands: add shell completion, reorder args, prefer exact site matches and prompt (603c8cc)

#### Bug Fixes

- commands: silence cobra usage and add operation validation (67dec17)

#### Performance Improvements

- wp: cache command dump and add timeout for completions (c364171)

## v4.25.0 (2026-04-24)

#### Features

- update: add signed releases and client-side signature verification (3b6e8b1)

## v4.24.0 (2026-04-23)

#### Features

- monitor: add monitorCacheBypass option to bypass frontend caches (3322275)

### v4.23.1 (2026-04-23)

#### Bug Fixes

- db: enforce case-insensitive domain handling (0e3f8f5)

#### Maintenance

- systemd: add jman-api.service systemd unit (f98a48a)

## v4.23.0 (2026-04-23)

#### Features

- monitor: notify Slack when ignoring a site in alert mode (15d36c5)
- monitor: add stateful monitoring engine, scheduler, and systemd service (c1ef84e)

#### Bug Fixes

- monitor: Log error on failed slack send. (2a4f4ea)
- monitor: normalize mode to Alert for sites marked down on load (abeddae)
- monitor: add synchronization and in-flight tracking for site checks and DB writes (8934a61)
- db: limit SQLite connections and serialize writes to avoid SQLITE_BUSY (84fdf62)

#### Refactor

- models: remove duplicate IgnoredSite struct (71d1566)

## v4.22.0 (2026-04-21)

#### Features

- monitor: add DB-backed monitoring API, ignore list, and CLI commands (30624b3)

#### Bug Fixes

- monitor: return pending status for unchecked sites and clean up stale statuses (869563c)

### v4.21.1 (2026-04-20)

#### Bug Fixes

- update: treat empty response as yes and show [Y/n] prompts (77b3d75)

#### Tests

- internal: add unit tests for auth, users config, and http utils (5fb6c1d)

## v4.21.0 (2026-04-20)

#### Features

- add configurable CORS, HTTP client utils, and SQL identifier validation (1b7376c)

#### Bug Fixes

- api: add security headers middleware and add timeouts to HTTP clients (dacfa29)

### v4.20.2 (2026-04-16)

#### Bug Fixes

- set SQLite pragmas and avoid holding lock while sending Slack alerts (cfa6a66)

### v4.20.1 (2026-04-15)

#### Bug Fixes

- fetch: handle API errors and non-JSON responses when fetching vulnerabilities (99832a2)

#### Performance Improvements

- cache: reduce concurrency limit for plugin cache refresh to 12 (21c8b26)

## v4.20.0 (2026-04-15)

#### Features

- plugin: support installing local .zip plugins by uploading to remote via scp (a529d1d)

#### Bug Fixes

- wpcli: add --force when installing zip plugins (cef2aed)

### v4.19.1 (2026-04-15)

#### Bug Fixes

- wpvuln: return Error 0 for invalid plugin slug (7716ba3)
- fetch: validate plugin slugs before fetching (c29f830)

## v4.19.0 (2026-04-15)

#### Features

- api: Add slug and name fields to the vuln API. (8162c75)
- vuln: enrich and filter vulnerability reports by affected sites (c56c906)

## v4.18.0 (2026-04-01)

#### Features

- plugin: Resolve Satispress aliases for plugin installation (1c11f3f)

### v4.17.1 (2026-03-30)

#### Bug Fixes

- fake commit to trigger release. (9a9b374)

#### Build System

- makefile: Simplify the build-pkg target. (b7bf87c)
- makefile: Add build-pkg target for package managers (053316c)

## v4.17.0 (2026-03-27)

#### Features

- plugin: Add info subcommand to get plugin details (fc78e7d)

## v4.16.0 (2026-03-26)

#### Features

- db: Implement robust database schema migration (c06423c)
- db: Add monitor and slack tables and migration (18dea2e)
- cache: Sanitize plugin info on all reads and writes (424b86c)
- db: Introduce SQLite database for plugin information (bd337b4)

#### Continuous Integration

- github: Remove release commit collection from workflow (c0c76d7)
- github: Update CI to use #jman_dev channel (1b4d00c)

## v4.15.0 (2026-03-23)

#### Features

- mods: Add ability to enable/disable file mods (4669dd5)

#### Bug Fixes

- cache: sanitize plugin metadata by decoding entities and stripping tags (1596077)

#### Refactor

- cache: make JSON cache TTL configurable (e100418)

#### Maintenance

- fixed version oopsie. (94cff62)

### v4.14.1 (2026-03-23)

#### Bug Fixes

- wpcli: Pass skip parameter to GetPlugins (230b8c6)
- admin: Use normal verbosity for user creation messages (d9219bc)

#### Refactor

- wpcli: Introduce CliOptions struct (a7378d5)

#### Documentation

- Add prerequisites to README (0d6a432)
- Update installation instructions (371a014)

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

## v4.13.0 (2026-03-20)

#### Features

- cmd: Add CLI tools for user and credential management (ebcd5c7)
- api: Implement JWT authentication and rate limiting (5f14d56)

### v4.12.1 (2026-03-20)

#### Bug Fixes

- update: Use AppVersion for current version check (218093b)

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

## v4.11.0 (2026-03-19)

#### Features

- wpcli: Return new version and language from UpdateCore (a6fc4bb)

#### Bug Fixes

- wpcli: Return structured data from UpdateCore (b7769c1)

#### Refactor

- verb: Rename ansi to verb and move ANSI color functions (ec587e5)
- verbosity: Rename verbosity package to verb (ca2481f)

### v4.10.1 (2026-03-19)

#### Bug Fixes

- wpcli: Disable skip in RunWP for plugin actions (bbededb)

## v4.10.0 (2026-03-19)

#### Features

- wpcli: Enhance core update check output (10a8406)
- core: improve core check, update, and version commands (31fb77a)
- core: Command to check core for updates and to update. (cde24be)

#### Bug Fixes

- wpcli: Use strings.SplitSeq for error splitting (13de961)
- RunWP better error handling (c602704)
- verbosity: Use verbosity.Println for cancelled operation message (6d6409e)
- wpcli: Make update regex multiline aware (6bff80b)
- wpcli: Print update core output verbosely (30c9aac)

#### Build System

- makefile: Inject app version into LDFLAGS via config (55640a7)

## v4.9.0 (2026-03-10)

#### Features

- plugin: batch updates and implement removal (f7557b4)
- search: allow selecting specific sites by index in results prompt (42c5c21)
- plugin: improve plugin list output formatting (f506f78)
- plugin: add list, update, and remove subcommands (e892e30)

#### Bug Fixes

- mods: show status messages at normal verbosity (22eae0b)

#### Refactor

- fetch: remove redundant pointer indirection (7954c9c)

## v4.8.0 (2026-03-09)

#### Features

- config: integrate viper for configuration and environment support (358d962)

## v4.7.0 (2026-03-03)

#### Features

- api: add main entry point (7f8d242)
- vuln: enhance version matching and reporting (2c1aee2)

#### Bug Fixes

- vuln: return error for unknown operators in versionCompare (914cc79)

#### Refactor

- vuln: remove version comparison fallback and simplify filtering (a09e359)

#### Maintenance

- remove accidental copy of the entrypoint. (f0f8a28)

## v4.6.0 (2026-02-27)

#### Features

- monitor: set custom User-Agent header for monitoring requests (ede2b9b)

## v4.5.0 (2026-02-27)

#### Features

- monitor: log duration of monitoring check (761f3e4)

#### Refactor

- monitor: use debug log level for ignored sites (3567e12)
- verbosity: migrate standard logging to level-aware LogPrintf (13efe7c)
- monitor: move monitoring logic to internal package (330d0d7)
- api: move middleware into api package (4d9cd6f)
- api: move route handlers to internal/api package (7d7c892)

#### Documentation

- readme: reorganize sidecar utility documentation (f90ad4b)

#### Maintenance

- rename internal/api package to internal/fetch (a9cfc43)

## v4.4.0 (2026-02-26)

#### Features

- update: add support for updating api and monitor components (99985b9)
- monitor: add site health monitoring tool (b15ad11)

#### Bug Fixes

- update: use verbosity levels for output messages (a7835b5)

#### Refactor

- monitor: use cached sites and remove PLAN.md (c1bf600)

#### Continuous Integration

- github: add jman-monitor to release artifacts (8832a8b)

## v4.3.0 (2026-02-26)

#### Features

- search: add plugin search and case-insensitive site matching (383789c)
- api: add jman-api REST service (c024dac)

#### Bug Fixes

- cache: filter out non-WordPress sites from site list (3b2baa8)

#### Refactor

- api: move middleware to internal package and update health route (588fa2d)
- api: move middleware to internal package and update health route (2351f6d)
- api: update fetch command references and health endpoint (2608c51)

#### Continuous Integration

- include jman-api in release artifacts (76c8c44)

#### Maintenance

- remove MainWP integration and rename Slack config (60c2936)
- remove MainWP integration and rename Slack config (13f5fed)

## v4.2.0 (2026-02-24)

#### Features

- fetch: add support for fetching plugin vulnerabilities (de08dc5)

#### Continuous Integration

- github: skip release steps when no version change is detected (3bb6ad2)

#### Maintenance

- zed: remove editor settings (fffcde4)

## v4.1.0 (2026-02-24)

#### Features

- fetch: support targeting specific resources for cache update (4116051)

#### Bug Fixes

- wpcli: improve error reporting (82a887d)

#### Continuous Integration

- remove build and release workflow (a21ecbd)
- github: update release workflow with build and notifications (e5824d4)

### v4.0.1 (2026-02-24)

#### Bug Fixes

- separate stdout and stderror to allow piping to files. (6ee0ebd)

#### Refactor

- cache: use verbosity package for logging (62d1039)
- verbosity: use verbosity package for output instead of fmt (5931c49)

#### Continuous Integration

- github: update version-file path to version.json (7fbe45a)
- add automated release workflow and version file (e36ad4c)

### Misc
- Refactor verbosity API and improve output handling (ced9cec)

## v4.0.0 (2026-02-24)

#### Features

- update: show download progress during updates (51396b6)
- update: implement automatic self-updating (473ad52)
- update: add command to check for latest version (c93affd)
- cmd: add verbosity flags and level management (94a59ab)
- verbosity: implement verbosity control and conditional printing (5b375df)

#### Bug Fixes

- vuln: fix typo in CVSS score label (00eb324)
- update: require valid download URL for update notification (4392dbc)
- root: show version only in verbose mode (f74ab69)
- vuln: sanitize HTML tags and entities in reports (85f4eb8)

#### Performance Improvements

- cache: limit concurrent plugin fetching to 24 (560aa8b)
- cache: fetch plugins concurrently (4dbe3b9)

#### Refactor

- cache: use verbosity level for plugin vulnerability logging (bf34026)
- vuln: use verbosity package for plugin processing output (f439f50)
- vuln: use slices.Contains and fmt.Fprintf (199764a)
- alias: replace interface{} with any (b4fddc6)

#### Build System

- ci: migrate build workflow from Bun to Go (685e4a9)

#### Continuous Integration

- github: update Go version to 1.25.x (35e9783)

#### Maintenance

- inactive: remove unused comments (5e0eeb6)
- remove PLAN.md and add dev target to Makefile (d319724)
- cleanup and documentation (dda9541)
- rewrite the whole thing in Go (be2b5f1)

### v3.4.8 (2026-02-17)

#### Continuous Integration

- Github: fix commit range and skip tag commit when generating changelog (452e8cd)

### v3.4.7 (2026-02-17)

#### Continuous Integration

- workflow: exclude commit pointed to by CURRENT_TAG when counting commits (01703e4)
- workflows: use env-based Slack message and fix commit summary formatting (4fff1a6)

### v3.4.6 (2026-02-17)

#### Continuous Integration

- slack: send Slack notifications as JSON payload using toJSON and format (18bf0c3)

### v3.4.5 (2026-02-17)

#### Continuous Integration

- workflow: update Slack action to use method/token/payload format (0a4b12c)

### v3.4.4 (2026-02-17)

#### Continuous Integration

- github: use explicit newline escape when truncating commit list (04fd398)

### v3.4.3 (2026-02-17)

#### Continuous Integration

- workflow: limit and format release commit list, add totals, upgrade Slack action to v2 (b8c9395)

### v3.4.2 (2026-02-17)

#### Continuous Integration

- github: fetch full history and include release commit list in Slack notifications (033dbbc)

### v3.4.1 (2026-02-17)

#### Continuous Integration

- workflow: add Slack notification step to release job (6835679)

## v3.4.0 (2026-02-17)

#### Features

- commands: add targeted fetch option and force flag to cache getters (e0c5175)

## v3.3.0 (2026-02-11)

#### Features

- helpers: add progress indicator to release downloads (3fd55f9)
- slack: support string-based durations for message tracking (bf36e66)

## v3.2.0 (2026-02-11)

#### Features

- slack: prevent duplicate messages using CRC32 hashing (1b7a0c9)

## v3.1.0 (2026-02-10)

#### Features

- vuln: add ability to ignore specific sites (d6a875b)

#### Bug Fixes

- slack: handle missing slack token (f6de5a6)

#### Refactor

- report: change plugin version separator to hyphen (364265c)

## v3.0.0 (2026-02-10)

#### Features

- vuln: add slack notification support (5166345)
- vuln: add site-based reporting and refactor scanning logic (031c2a2)
- report formatting to use source data (3128ea7)

#### Refactor

- slack: move messaging to dedicated module and use WebClient SDK (7030ef1)
- CVSS score retrieval (55b2832)

### v2.3.2 (2026-02-02)

#### Bug Fixes

- changed to correct version number. (99d0807)
- Improve update jman reliability (339a3c0)

### v2.3.1 (2026-02-02)

#### Bug Fixes

- Add unlinkSync to updateJman command (4092cd4)

## v2.3.0 (2026-02-02)

#### Features

- Add update command and logic (7783cf4)

#### Bug Fixes

- Remove unused imports from utils (ee98d87)

#### Refactor

- createAliases command (b43d403)

#### Documentation

- Added documentation for vuln command to readme (ae2678e)
- Added comments to functions (bfa97eb)

## v2.2.0 (2026-01-27)

#### Features

- Add skip option to runWP function (594e90e)
- Add CVSS threshold filtering (22849b0)

### Misc
- Add check for wp-cli executable (b4185d3)

### v2.1.1 (2026-01-22)

#### Features

- Add CVSS threshold for Slack notifications (75df7a5)

#### Bug Fixes

- Remove autochangelog (c0529c5)
- type imports in commands.ts (1d45c46)

### v2.0.1 (2026-01-22)

#### Build System

- add binary install script to makefile (9699553)

#### Continuous Integration

- Update release artifact path (4fa14d6)

## v2.0.0 (2026-01-22)

#### Bug Fixes

- Add check for config file existence (3c4bf36)

#### Refactor

- Replace @folder/xdg with xdg-basedir (e14103a)

### Misc
- Migrate to Bun and update dependencies (3d66d0e)

## v1.5.0 (2026-01-20)

#### Features

- Refactor vulnerability scanning to prevent duplicate slack messages (4f01512)
- Add html-entities dependency (25c3a91)
- Refactor scanVulnerabilities to process vulns (404d732)
- Add vulnerability type definitions (ce7d567)
- Add slackHook to config schema (078bfd5)
- Add cache for WordPress vulnerabilities (b0692db)
- Add functions for Slack and WP vulnerabilities (a3d4279)
- Plugin list cache and processing (f725cf8)
- Enforce Node.js version in package.json (08fc5ee)
- Add dummy function for vulnerability check. WIP (196dc5e)
- Add function to fetch plugins from site. (8ffb565)

#### Bug Fixes

- Refactor plugin caching and vulnerability processing (f220571)
- Improve vulnerability caching and schema validation (5d1ea33)

### v1.4.9 (2026-01-16)

### v1.4.8 (2026-01-16)

#### Documentation

- Replace conventional-changelog with auto-changelog (22ecdaf)

### Misc
- Add description and keywords to package.json (3c5031f)

### v1.4.7 (2026-01-16)

#### Documentation

- Add README.md with project documentation (9f6a8c5)

### v1.4.6 (2026-01-16)

### Misc
- Add repository field to package.json (4af0a89)

### v1.4.5 (2026-01-16)

#### Bug Fixes

- Remove unnecessary permissions from build job (0d21c83)

### v1.4.4 (2026-01-16)

#### Continuous Integration

- Added paermission settings (c6befb3)

### v1.4.3 (2026-01-16)

#### Continuous Integration

- Configure NPM publish in CI (badbdc8)

### v1.4.2 (2026-01-16)

### v1.4.1 (2026-01-16)

#### Refactor

- build scripts to use npm run commands (6332adc)

## v1.4.0 (2026-01-15)

#### Features

- Add NPM publish to CI and bin entry (7f89d29)
- Add getErrorMessage utility function (b398e39)

### v1.3.3 (2026-01-12)

#### Refactor

- Make command functions async and update fetchData usage (e2dd188)

### v1.3.2 (2026-01-12)

#### Refactor

- command handling and add fetchData, listData helpers (c64b3ed)
- command handlers into separate commands module (8801adc)

### v1.3.1 (2025-11-21)

#### Bug Fixes

- Remove faulty import. Improve plugin install error handling and usage messages (d57891f)

## v1.3.0 (2025-11-21)

#### Features

- Add plugin install command and support for repo plugin URLs (fc1be7d)

## v1.2.0 (2025-11-06)

#### Features

- file mods command (7ec9b80)
- MainWP user password reset and refactor command handlers (c7b93e3)

### v1.1.1 (2025-11-04)

#### Refactor

- command parsing and handling logic (bf1ecfa)

## v1.1.0 (2025-11-03)

#### Features

- Add inactive command to list sites without active MainWP (f0a764d)

#### Bug Fixes

- Handle user cancellation in promptSearch without throwing (897bade)

## v1.0.0 (2025-10-31)

#### Bug Fixes

- Remove unused API_MAINWP_URL import from constants (2986f9c)

### Misc
- BREAKING CHANGE: Change config format. Add MainWP install command and update config handling (9e34ce9)
- Refactor alias creation and site search logic (60ae2a2)
- Add @topcli/prompts and implement interactive site search (783b674)
- Add WP CLI integration and site search utilities (e3153fa)

### v0.1.3 (2025-10-28)

#### Bug Fixes

- Update version script and improve error logging (1757b3d)

### v0.1.2 (2025-10-28)

#### Build System

- Update build target to use dist/jman instead of bin/jman (7ae467a)

### v0.1.1 (2025-10-28)

#### Continuous Integration

- Use make to build instead of pnpm in CI workflow (0d68d22)

## v0.1.0 (2025-10-28)

#### Features

- Implement caching for servers and sites (f9308e4)
- Implement basic site listing (efa73ad)

### Misc
- Add GitHub Actions build workflow and clean up files (15dd88b)
- Add alias generation and command parsing utilities (6c5b719)
- Basic server fetch (9f29cd8)

