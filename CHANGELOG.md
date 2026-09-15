# Changelog

## v5.52.0 (2026-09-14)

#### Features

- task: add on_hold and blocked task statuses (dc3cd3e)

### v5.51.1 (2026-09-14)

#### Refactor

- web: simplify site filters and button styling in SitesView (aa6efb2)

## v5.51.0 (2026-09-14)

#### Features

- web: add site filtering options for multisite and file mods (ae619a1)

#### Refactor

- api: upgrade agent tokens to sha256 and enhance validation (c530efd)

## v5.50.0 (2026-09-11)

#### Features

- agent: support non-WordPress sites in agent collection and manifest (e443256)

### v5.49.1 (2026-09-11)

#### Bug Fixes

- ui: format WordPress core entries in the update ledger (f6680f3)
- ui: keep WordPress Core pills on one line (69784a2)

#### Refactor

- web: replace inline SVG traffic chart with Chart.js (0324a8d)

## v5.49.0 (2026-09-11)

#### Features

- sites: add WordPress Core update card to site detail page (702104f)

### v5.48.1 (2026-09-11)

#### Bug Fixes

- ui: handle undefined task.metadata in vulnerability status check (5a467de)

## v5.48.0 (2026-09-11)

#### Features

- ui: show vulnerability pill and status on dashboard task list (9ae4e1a)

#### Bug Fixes

- ui: reflect task changes immediately without a full refetch (64ae440)
- ui: right-align vulnerability pill in task title column (d084804)

#### Maintenance

- git: ignore web package-lock.json (87a39ef)

### v5.47.1 (2026-09-11)

#### Bug Fixes

- ui: close task modal on any quick action (ddaf34c)
- vuln: prune orphaned plugin cache rows for deleted sites (533dae2)

## v5.47.0 (2026-09-11)

#### Features

- wpcli: cache and pass admin user ID to WP-CLI commands (428f2f1)

## v5.46.0 (2026-09-07)

#### Features

- monitor: add support for PagerDuty EU region endpoint (57e064d)
- monitor: add PagerDuty integration for uptime alerts (6956f40)

### v5.45.1 (2026-09-05)

#### Bug Fixes

- tasks: stabilize vuln report ordering and improve diff formatting (d07d8a5)

## v5.45.0 (2026-09-04)

#### Features

- tasks: notify assignee on task create/change/assign (6b0a1c4)

## v5.44.0 (2026-09-04)

#### Features

- tasks: add configurable default assignee for vulnerability tasks (94bebc5)
- api: trigger vuln task sync + Slack report from data refresh (63fe20b)

#### Bug Fixes

- release: keep web/package.json version in sync with version.json (ab14967)

## v5.43.0 (2026-09-04)

#### Features

- components: add secondary copy functionality to InfoCard (1d40969)
- site: display system user in site detail view (d8def71)
- assets: add license key support (1565b7b)
- SiteDetailView: add expand/collapse toggle for ledger entries (4e5713f)
- site: implement site update ledger (9f46aa4)
- reports: add support for enddate parameter type (72767f5)
- reports: add report list and runner views (ee69ffc)
- components: increase hourly traffic data retention window to 7 days (2567f2e)
- ui: add copy-and-extract functionality to asset description (71a85bd)
- assets: add payment methods management (bd4b93d)
- ui: display status codes in site traffic card (e11f085)
- traffic: add monthly view to site analytics (36fb72f)
- traffic: add interactive site traffic chart (e0d63d1)
- components: implement site traffic analytics card (91eb244)
- settings: display agent version in token list (09a04e8)
- settings: switch server ID input to selection list (bbc7b28)
- settings: add agent token management and site metrics (3b03567)
- views: display disk space usage for servers (d3f6f0c)
- task: add vulnerability status tracking to tasks (346b722)
- assets: standardize list view with sorting, pagination, and metadata (b04b8c4)
- notes: support string parent IDs and link plugin notes by slug (ca446a0)
- notes: focus note input on add and integrate notes in plugin detail view (739967e)
- notes: position Add Note button in card header (63bafb7)
- notes: collapse add-note form and truncate note list to recent 3 items (2dc777b)
- notes: implement reusable notes widget on organization and site views (5f42a5a)
- assets: add asset type filter and optimize column density (9ca839f)
- sites: add organization name tooltip to sites table (659f7fe)
- site: add archived environment support (e5346ea)
- site: add environment classification and batch editing (b920193)
- plugin: add support for updating vulnerable plugins (632e20b)
- view: update header actions in TemplatesView (674c7fa)
- asset: add organization selection and linking to asset management (fa148e5)
- components: implement reusable AssetEditModal component (34d515f)
- assets: add edit modal to asset list (81ec8cf)
- task: display completion details in task info modal (48e436c)
- settings: add slack reminder time configuration (1014405)
- ui: implement asynchronous confirmation modal (f016219)
- dashboard: implement modular and reorderable widget system (7c3fd1a)
- style: add table column width and text alignment utility classes (a1c7198)
- vulnerability: implement suppression status for ignored vulnerabilities (5221e0c)
- vulnerability: add copy to clipboard functionality for vulnerability UUIDs (deb6e08)
- store: optimize ignore lookup logic using computed index (b1e1a8d)
- settings: add edit functionality for ignore rules (a54db7f)
- ignore: implement unified ignore management system (52d8fc3)
- site: add database table prefix to site info details (45dfd11)
- settings: add slack integration and replace modal close buttons with icons (d007a41)
- components: implement AppIcon component and replace inline SVGs (0e2cd36)
- task: improve task management and user assignment (170d13a)
- tasks: implement task management system (0f371be)
- ui: display update result status in modal badges (e1819dd)
- PluginSiteUpdateModal: add support for updating only vulnerable plugin instances (cc31d3e)
- plugin: add modal to update plugin across sites (e8bd1f6)
- data: update local plugin version after successful update request (878e2da)
- plugin: add update modal for site plugins (772450e)
- settings: synchronize settings with API (78e8dba)
- dashboard: add vulnerability dashboard widget (90b0b43)
- auth: add admin user level and restrict user management access (29aec0b)
- settings: allow password resets in user edit modal (12f75d0)
- user: add loading and error states for profile fetching (463b755)
- settings: add user management and 2FA configuration (931f129)
- ui: implement toast notifications for error handling (4c0d74c)
- auth: implement role-based access control with level-based permissions (766e82e)
- assets: implement asset and subscription management module (371f925)
- audit: add audit trail to organization view (c927c55)
- organization: sync site links on organization fetch (268fe35)
- organization: add debounce and request cancellation to search functionality (7227e01)
- EditableInfoCard: add async save support and loading state (65a8817)
- organization: display linked sites in organizations table (8bd6bf5)
- data: implement local caching for site-company associations (a6e61f1)
- site: implement site and company linking functionality (7d2c59e)
- company: implement company and contact management system (a644ac6)
- monitor: render 24h proportional status timeline and update uptime calc (e8c35a4)
- monitor: display ignored domains and pending live status (0d7e525)
- settings: add settings view and store with refresh interval controls (44e99b1)
- monitor: add monitor store and history UI component (568d2f2)
- components: add refresh button to AppNav and copy-to-clipboard in InfoCard (3193be5)
- components: add reusable UI components and integrate into views (7ccfb62)
- data: add vulnerabilities to enriched types and surface them in UI (f1cb32d)
- vulns: add vulnerability support to store and UI (75b09aa)
- router: Introduce Dashboard and Sites views (8f11c61)
- router: add query params for pagination (e1c544f)
- data: Add enrichedSites computed property (34a9d5a)
- store: centralize plugin enrichment and formatting logic (e96cecb)
- data: Fetch and cache plugin metadata (2d057cd)
- auth: Implement authentication flow (d210830)
- state: Integrate Pinia for global state management (35a55b7)
- plugins: add plugin listing and detail views (94f6a00)
- app: add footer to display application version (e241bd0)

#### Bug Fixes

- ci: pin pnpm version for pnpm/action-setup (38b57f3)
- plugin: allow update modal interaction for installed plugins (7dea2c3)
- ui: update asset identifier helper text (a08cdca)
- assets: improve loading states and add asset management details (da8d679)
- components: adjust traffic window based on data retention policy (de9bb69)
- ui: update status badge logic in update modals (a3a9510)
- notes: resolve color readability issues in dark mode (ca98980)
- dashboard: wire up edit action in task reminder widget (3095bab)
- organization: fix plugin audit visibility and prefill convert-to-asset modal (d053d46)
- auth: log server response body on login failure (9e321c6)
- dashboard: prevent null item insertion during drag and drop reordering (531795d)
- vulnerability: exclude suppressed vulnerabilities from display and data processing (cc29b6d)
- task: improve task filtering and vulnerability suppression naming (5701c1d)
- components: add domain-based lookup fallback for monitor history ignore status (ad9934d)
- site: remove public folder field from site details (e8b22df)
- ui: improve date parsing logic in TaskFormModal (d01da9a)
- tasks: update current date reference and improve code formatting (03e09fd)
- settings: validate password strength in user form submission (4241840)
- navigation: rename assets route to inventory (27e5315)
- organization: update phone placeholder format (57ba4ab)
- component: allow falsy values in EditableInfoCard (cb948fa)
- monitor: prevent marking history as fetched on error (39bf54d)
- timeline: render history blocks using duration-based flex-grow and ensure visibility (aa6dbfd)
- components: preserve fractional uptime percentage and format to two decimals (ab93a5f)
- monitor-history-card: treat error_code 0 as unknown and add status-unknown style (bba045d)
- monitor: show tiny/zero-duration timeline items and make status checks case-insensitive (ff916a1)
- components: avoid accessing first_seen when sortedHistory is empty (ca81f6c)
- data: Display server name or "Unknown Server" (2d5000e)
- types: remove unused vue-router import (14d3690)
- safer JSON parsing (a4a5340)
- more specific error handling (17d74b1)
- correct favicon type and remove manual plugin name encoding (6a48e18)

#### Performance Improvements

- data: add computed lookup maps and replace repeated filters with map lookups (0c03fda)

#### Refactor

- views: improve layout of asset form fields (5a1119e)
- components: improve null safety in TrafficChart path calculations (d6c1744)
- settings: remove v-prefix from agent version display (1731221)
- ui: manage asset price as string input (b2c7080)
- organization: extract modal components from detail view (abb4a19)
- assets: decouple asset management logic from organization details (4046c5c)
- style: move component-specific styles to global CSS (d146876)
- icons: convert SVG assets to Vue components (74c5f09)
- api: centralize BASE_URL definition (a8c899d)
- api: use centralized error handler in stores (d49e5bd)
- data: update vulnerability enrichment logic in store (d5678fd)
- data: normalize vulnerability data structures (a1e2f5a)
- data: use backend-provided suppression status for vulnerabilities (b15ce68)
- components: reorder script blocks for consistency (08d47ac)
- sites: reorder reactive state declarations in SitesView (0de2cdf)
- view: type InfoItem for site info items in SiteDetailView (fd1e5d4)
- ui: update organization tables to use standardized components and utilities (0907b23)
- style: modularize and standardize CSS architecture (046c413)
- dashboard: move vulnerability widget position (b1d0837)
- types: replace PluginUpdate with Plugin interface (2acac78)
- plugin: move update logic to Pinia store (4e7da50)
- settings: move ignored domains to separate component and tab (4a35301)
- style: remove inline styles in favor of scoped CSS classes (8622884)
- organization: rename companies to organizations (0ef0f68)
- vulnerabilities: move inline styles into scoped CSS classes in PluginVulnerabilityList.vue (d10346d)
- components: rename types, compress CSS, move vulnerability list in detail view (024527e)
- ui: move styles to global CSS and update cache storage (a66146f)

#### Documentation

- AGENTS: update icon implementation details and document intentional design decisions (bbcc892)
- add project styling guidelines documentation (a06b60f)
- add CHANGELOG.md (00b354a)
- readme: update with project overview and setup instructions (c2476eb)

#### Styles

- refactor: format codebase with Prettier (30dda00)
- ui: standardize AppIcon rendering and asset action button (70ff2a8)
- components: remove text-transform from badge-sm (8710b56)
- site: add visual separator to plugins header (a4daeb5)
- ui: implement responsive design for mobile screens (b4b29c8)
- views: format code and update container responsiveness (4818fb6)

#### Build System

- specify pnpm as the package manager (8664d5f)

#### Continuous Integration

- release: update SSH key secret name (bd8a850)
- release: use variables for known_hosts instead of secrets (16ac7a9)
- add release workflow and Makefile (bc81ef4)

#### Maintenance

- wire up web/ (jman-ui) in the monorepo CI/build (55b61ec)
- store: update vulnerabilities cache key (5907325)
- docs: remove deprecated API_SPECS.md file (4837f1c)
- lint: configure ESLint and Prettier (7f17126)
- config: add foonver.toml to enable push and changelog (b55c70b)
- replace vite favicon and update .env.example (c785d12)
- add postversion script (33f6e6f)
- remove PLAN.md (0e4d37c)

#### Misc

- Add 'web/' from commit '329e29ae8edaed7c064b659ef1145a7a290949a4' (55fd685)
- Vibe coded initial version. (219d120)

### v5.42.1 (2026-09-02)

#### Bug Fixes

- db: refuse startup whenever the legacy jman.db still exists (3a69c88)

#### Refactor

- separate jman CLI and jman-api into independent processes/data (7c12dc1)

#### Continuous Integration

- release: deploy jman-api binary to server after release build (00de0e2)

## v5.42.0 (2026-09-02)

#### Features

- api: add ledger logging for site plugin updates (b7a11b0)

#### Bug Fixes

- api: update ledger status to partial on plugin updates (fca368e)

## v5.41.0 (2026-08-31)

#### Features

- db: add license key field to assets and organization assets (e00bcc2)

#### Tests

- db: ensure correct yesterday date for traffic rollup test (87eaa3d)
- db: add tests for asset license key inheritance logic (6cd989f)

## v5.40.0 (2026-08-28)

#### Features

- api: implement site update ledger (3ea8b81)
- commands: add setup compat command (f5886d1)

## v5.39.0 (2026-08-24)

#### Features

- reports: add upcoming billing report (0310609)
- api: implement report generation framework (194041b)

## v5.38.0 (2026-08-24)

#### Features

- monitor: Add agent staleness detection and alerts (62c45ce)

## v5.37.0 (2026-08-21)

#### Features

- api: add payment method support to assets (5d495fb)

### v5.36.1 (2026-08-20)

#### Bug Fixes

- agent: handle nil maps when loading legacy state (f95148e)

## v5.36.0 (2026-08-20)

#### Features

- agent: track HTTP status codes and improve log filtering (45e8305)

## v5.35.0 (2026-08-18)

#### Features

- api: add support for monthly site traffic aggregation (dcf7b26)

## v5.34.0 (2026-08-18)

#### Features

- agent: exclude admin and API paths from top pages list (5a2f18c)

## v5.33.0 (2026-08-18)

#### Features

- agent: aggregate historical traffic logs into daily entries (8a52e6a)

## v5.32.0 (2026-08-18)

#### Features

- logs: filter internal referrers and prune hourly traffic (5ccc19b)

## v5.31.0 (2026-08-18)

#### Features

- agent: rotate site processing order to ensure fair budget usage (92227b4)

### v5.30.3 (2026-08-18)

#### Bug Fixes

- agent: correct traffic date comparison and improve logging (54237fa)

### v5.30.2 (2026-08-18)

#### Bug Fixes

- agent: implement backlog catch-up scheduling (2cad626)

### v5.30.1 (2026-08-18)

#### Bug Fixes

- agent: apply traffic report budget to live log processing (8c8fe0e)

## v5.30.0 (2026-08-18)

#### Features

- agent: bound log processing to prevent oversized reports (3d71c74)

## v5.29.0 (2026-08-18)

#### Features

- agent: implement log-based traffic tracking and collection (2908ce5)

## v5.28.0 (2026-08-18)

#### Features

- agent: trigger fast-path self-update via manifest (65de878)

### v5.27.1 (2026-08-17)

#### Refactor

- agent: use fixed files directory for site path resolution (adac19f)

## v5.27.0 (2026-08-17)

#### Features

- agent: improve site path resolution and manifest filtering (c6e7065)
- agent: track agent versions and improve path resolution (c4e74be)

## v5.26.0 (2026-08-17)

#### Features

- agent: introduce jman-agent for local server monitoring (34f2c74)

#### Documentation

- readme: add documentation for jman-agent (984f548)

### v5.25.1 (2026-08-17)

#### Bug Fixes

- tasks: unescape plugin names in vulnerability sync (6b238af)

## v5.25.0 (2026-08-14)

#### Features

- notes: migrate parent_id to string to support slug-based plugin notes (b7d4bb4)

## v5.24.0 (2026-08-14)

#### Features

- notes: add support for Plugin parent type in Go API (671ff83)
- assets: include asset type in organization assets query (a5470e6)

## v5.23.0 (2026-08-06)

#### Features

- api: add archived site environment type (0e9b4b4)

## v5.22.0 (2026-08-06)

#### Features

- api: implement site environment classification (fe4fcf6)

## v5.21.0 (2026-08-05)

#### Features

- vuln: add WordPress core vulnerability scanning (8817a43)

#### Bug Fixes

- cache: parse SQLite DATETIME as RFC3339 in plugin freshness check (4d46513)
- cache: parse SQLite DATETIME as RFC3339 in core version freshness check (d22a896)

### v5.20.5 (2026-08-04)

#### Bug Fixes

- config: rename UsersConfig lock methods to avoid implementing sync.Locker (a5d7fe0)
- commands: don't treat non-interactive EOF as update confirmation (92fb1d9)
- cache: guard cache/data file paths against traversal (f47d7d7)
- config: enforce file permissions on config.toml (45ca44e)
- db: make CompleteTask's completion check atomic (1d77865)
- cmd: run deferred cleanup before process exit (62c906c)
- api: only trust X-Forwarded-For/X-Real-IP from configured trusted proxies (4718881)
- wpcli: shell-quote arguments passed to RunSSH (474444f)
- update: enforce signature verification and harden downloads (49c139c)
- config: initialize lock when constructing UsersConfig outside LoadUsersConfig (e48986a)
- wpcli: force refresh of WordPress update transient before plugin updates (74513a9)

### v5.20.4 (2026-08-04)

#### Bug Fixes

- api: return completed task object in CompleteTask handler (aac2b1c)

### v5.20.3 (2026-08-03)

#### Continuous Integration

- github: allow AUR publication steps to fail (4068873)

### v5.20.2 (2026-08-03)

#### Maintenance

- api: add air configuration for live-reloading (23b8d56)

### v5.20.1 (2026-06-04)

#### Bug Fixes

- api: verify plugin version against remote state when update check reports up to date (b9abc18)

## v5.20.0 (2026-06-04)

#### Features

- api: include error details in plugin update responses (2c24e90)

#### Bug Fixes

- api: improve error handling and response consistency for plugin updates (d9cbed1)

## v5.19.0 (2026-06-04)

#### Features

- api: trigger slack notification upon task assignment (acbf43b)

#### Bug Fixes

- api: prevent data races when notifying task assignees (bed9013)
- tasks: improve reminder time parsing and validation (b2efc2b)
- api: implement configurable Slack reminder times and reset notification status on reassignment (8670a6c)

#### Performance Improvements

- tasks: implement caching for user reminder times to avoid N+1 queries (62665eb)

## v5.18.0 (2026-05-26)

#### Features

- commands: add major version update support for wordpress core (126d902)

## v5.17.0 (2026-05-22)

#### Features

- vuln: refactor vulnerability reporting to support multiple vulnerabilities per plugin (970d654)

#### Bug Fixes

- vuln: ignore suppressed vulnerabilities and fix CVSS pointer assignment (ca44493)
- vuln: recompute CVSS score after filtering vulnerabilities (de6eed0)

## v5.16.0 (2026-05-22)

#### Features

- cache: add suppressed field to plugin site data structure (6bfd802)
- api: track user who completed tasks (4b85b40)

#### Refactor

- task: move completion logic to database repository (bd2c42f)

## v5.15.0 (2026-05-21)

#### Features

- api: add granular vulnerability suppression logic and documentation (40ff9cd)

#### Bug Fixes

- api: stop forcing capitalization on plugin update error messages (500eaf6)
- api: clean up plugin update error messages (a704978)

## v5.14.0 (2026-05-18)

#### Features

- db: implement unified ignore system for monitor and vulnerabilities (25ced53)

#### Bug Fixes

- api: force new ID on ignore entry creation (523f505)

#### Refactor

- ignore: optimize ignore matching with in-memory matchers (d035899)

#### Documentation

- ignore: document the unified ignore list system (6da779b)

### v5.13.1 (2026-05-18)

#### Documentation

- tasks: add task system specification documentation (fb0c9d6)

## v5.13.0 (2026-05-18)

#### Features

- config: add SlackTasksChannel support for task notifications (9654aef)

#### Bug Fixes

- tasks: only update last notified timestamp if slack message is sent successfully (cbfdb04)
- api: implement strict field validation for task updates (f2e89e2)
- api: allow partial task updates by checking field presence in request body (375a89e)
- api: allow basic access to user list and make task description nullable (169b136)

#### Documentation

- project: add AGENTS.md documentation file (c8d528f)
- api: update documentation for ListUsersHandler functionality (6459f3e)
- api: remove duplicate user creation documentation (4fc1ce6)

#### Continuous Integration

- github: add CI workflow and rename release workflow (4b2be89)

## v5.12.0 (2026-05-14)

#### Features

- task: add last_notified_at field and prevent duplicate reminders (37f2964)
- tasks: implement task management system (02cb53a)

#### Bug Fixes

- scheduler: handle errors when fetching cache during orphaned task cleanup (7499b06)
- db: prevent redundant update when completing already completed task (4b9b041)
- scheduler: use correct site version when grouping vulnerabilities (f1dba32)

### v5.11.2 (2026-05-13)

#### Bug Fixes

- db: improve table migration robustness (b873716)

### v5.11.1 (2026-05-13)

#### Bug Fixes

- wpcli: use json.Decoder for robust output parsing (4fea081)
- wpcli: sanitize output before JSON unmarshaling to handle stray notices (ff0874f)

## v5.11.0 (2026-05-12)

#### Features

- api: add endpoints to list and update site plugins (c116e25)

#### Bug Fixes

- api: handle plugin save errors and improve cache persistence logic (9725459)
- db: handle iteration errors in GetSitePluginLastUpdates (012ddf8)
- api: refresh plugin cache after site update (998311f)
- api: validate plugin slug format in SitePluginUpdateHandler (393adca)
- cache: prevent unnecessary plugin re-fetches when timestamps are missing (123b3c2)
- cache: implement TTL-based plugin fetching logic (3ec102e)

## v5.10.0 (2026-05-08)

#### Features

- api: implement user settings management endpoints (3054b72)

#### Bug Fixes

- api: return 404 for missing settings on PATCH requests (9bef8b5)
- db: enforce non-null value constraint and handle optional settings safely (bdda58e)

#### Tests

- api: add unit tests for settings handlers (ce692bc)

### v5.9.1 (2026-05-07)

#### Bug Fixes

- db: correct column existence check during table migration (834911c)
- db: add primary keys to schema and handle potential errors in plugin checks (ec351fe)

#### Refactor

- db: migrate plugin cache from JSON files to SQLite (11e32bf)

## v5.9.0 (2026-05-07)

#### Features

- backup: include duration in success log message (c90c5cc)
- backup: implement automated hourly database backup scheduler (4cf0874)

## v5.8.0 (2026-05-07)

#### Features

- api: add proxy support and enhance security (a96eb4a)
- auth: introduce admin user level (0e5a85f)
- api: allow admins to update user passwords (f5491a0)

#### Bug Fixes

- api: security across authentication and database layers (bdf79e6)
- api: secure 2FA setup and implement rate limiting on password changes (4cf2cfe)
- auth: implement token revocation and harden user management (312d28d)

## v5.7.0 (2026-05-06)

#### Features

- api: add username normalization and validation (31414b3)
- api: implement user level validation and update CORS policies (e0723fe)
- api: implement get profile endpoint (5bb2233)
- api: enforce password strength requirements (0400240)
- api: implement user management and self-service features (00b11cf)

#### Bug Fixes

- auth: increase minimum password entropy requirement (c7bd726)
- api: require TOTP code for 2FA deactivation (763fb7d)

#### Refactor

- config: implement thread-safe access and atomic file writes (c0aedec)

## v5.6.0 (2026-05-05)

#### Features

- api: implement role-based access control with user levels (c303930)
- api: automate next_billing updates and add explicit override (5a2b09c)
- api: add endpoint to list all organization assets (961ce01)
- asset: add active status and lifecycle tracking to assets (85bedde)
- asset: implement asset management system (d7ec4f7)

## v5.5.0 (2026-05-04)

#### Features

- plugin: add summary report for multi-site plugin updates (ae00bae)

## v5.4.0 (2026-05-04)

#### Features

- api: add endpoint to list users (8e48a3d)

## v5.3.0 (2026-05-03)

#### Features

- api: add endpoint to list sites by company (d7e675e)
- api: implement company, contact, and note management (047ce30)

#### Bug Fixes

- api: improve cache handling and strengthen input validation (9a9242f)

#### Refactor

- api: rename company entities to organization (e16eb66)

### v5.2.1 (2026-04-30)

#### Bug Fixes

- slack: strip ANSI escape codes from messages before sending (6ad0ae7)

## v5.2.0 (2026-04-29)

#### Features

- vuln: add ignore list functionality (f5ed51d)

#### Refactor

- vuln: update search logic and enhance terminal output styling (be56abf)

### v5.1.1 (2026-04-28)

#### Refactor

- completions: optimize and improve site completion logic (24c10ec)

## v5.1.0 (2026-04-28)

#### Features

- setup: add command to install or update bojaco mu-plugin (f17a502)

#### Refactor

- plugin: split plugin command into subcommands and add concurrency safety (bb13b1b)

#### Maintenance

- aur: remove .SRCINFO file (7c60575)

### v5.0.6 (2026-04-28)

#### Continuous Integration

- aur: switch to KSXGitHub/github-actions-deploy-aur action (19dca01)

### v5.0.5 (2026-04-28)

#### Documentation

- update documentation for CLI changes and remove MainWP (2989336)

#### Continuous Integration

- aur: update deployment action and add .SRCINFO (999ed79)
- github: switch to shmew/aur-deploy action (c6e1b94)

### v5.0.4 (2026-04-28)

#### Continuous Integration

- aur: fix deployment action version and improve update script (9d40f14)

### v5.0.3 (2026-04-28)

#### Build System

- completions: move output directory to project root (dd602f1)

#### Continuous Integration

- aur: automate publishing to AUR (09131e8)

### v5.0.2 (2026-04-28)

#### Refactor

- commands: unify site completions and rework vuln command (92756e1)

### v5.0.1 (2026-04-28)

#### Continuous Integration

- github: prefix release tag with v (b263800)

## v5.0.0 (2026-04-28)

## 5.0.0 (2026-04-28)

#### Features

- Release new version (167572a)
- add shell completions (BREAKING CHANGE) (45d6f41)

#### Documentation

- readme: add Security & Updates section and update command description (2ba9332)

#### Build System

- makefile: set JMAN_TOKENSPINUP placeholder for shell completions (9951997)

#### Continuous Integration

- github: update foonver action version to v0.9.1 (9a32675)
- migrate release pipeline to foonver (5f71062)

## v4.26.0 (2026-04-27)

#### Features

- commands: add shell completion for monitor and wp commands (d74963b)
- search: add fast cache-backed site/plugin search and fast cache readers (b82babf)
- plugin: suggest cached plugin names for subcommand argument completion (3ae41cb)
- commands: accept action before target and add shell completion for mods command (151e547)
- fetch: add shell completion for fetch command and rename target to operation (50e1c62)
- commands: add shell completion, reorder args, prefer exact site matches and prompt (d2af36f)

#### Bug Fixes

- commands: silence cobra usage and add operation validation (5ea19f5)

#### Performance Improvements

- wp: cache command dump and add timeout for completions (8f595ff)

## v4.25.0 (2026-04-24)

#### Features

- update: add signed releases and client-side signature verification (7b858cd)

## v4.24.0 (2026-04-23)

#### Features

- monitor: add monitorCacheBypass option to bypass frontend caches (c37bc8b)

### v4.23.1 (2026-04-23)

#### Bug Fixes

- db: enforce case-insensitive domain handling (134e0c9)

#### Maintenance

- systemd: add jman-api.service systemd unit (38975f5)

## v4.23.0 (2026-04-23)

#### Features

- monitor: notify Slack when ignoring a site in alert mode (2b23e06)
- monitor: add stateful monitoring engine, scheduler, and systemd service (084761e)

#### Bug Fixes

- monitor: Log error on failed slack send. (f0c99eb)
- monitor: normalize mode to Alert for sites marked down on load (946de51)
- monitor: add synchronization and in-flight tracking for site checks and DB writes (db454f9)
- db: limit SQLite connections and serialize writes to avoid SQLITE_BUSY (215f8c5)

#### Refactor

- models: remove duplicate IgnoredSite struct (1b27af0)

## v4.22.0 (2026-04-21)

#### Features

- monitor: add DB-backed monitoring API, ignore list, and CLI commands (734af86)

#### Bug Fixes

- monitor: return pending status for unchecked sites and clean up stale statuses (68fb12c)

### v4.21.1 (2026-04-20)

#### Bug Fixes

- update: treat empty response as yes and show [Y/n] prompts (7484354)

#### Tests

- internal: add unit tests for auth, users config, and http utils (7516642)

## v4.21.0 (2026-04-20)

#### Features

- add configurable CORS, HTTP client utils, and SQL identifier validation (dcb4a24)

#### Bug Fixes

- api: add security headers middleware and add timeouts to HTTP clients (e457009)

### v4.20.2 (2026-04-16)

#### Bug Fixes

- set SQLite pragmas and avoid holding lock while sending Slack alerts (17bb76a)

### v4.20.1 (2026-04-15)

#### Bug Fixes

- fetch: handle API errors and non-JSON responses when fetching vulnerabilities (ee3c19d)

#### Performance Improvements

- cache: reduce concurrency limit for plugin cache refresh to 12 (c42474b)

## v4.20.0 (2026-04-15)

#### Features

- plugin: support installing local .zip plugins by uploading to remote via scp (c9de192)

#### Bug Fixes

- wpcli: add --force when installing zip plugins (b10e7b6)

### v4.19.1 (2026-04-15)

#### Bug Fixes

- wpvuln: return Error 0 for invalid plugin slug (0103c1c)
- fetch: validate plugin slugs before fetching (380723d)

## v4.19.0 (2026-04-15)

#### Features

- api: Add slug and name fields to the vuln API. (89a15d0)
- vuln: enrich and filter vulnerability reports by affected sites (92175cc)

## v4.18.0 (2026-04-01)

#### Features

- plugin: Resolve Satispress aliases for plugin installation (9c0099a)

### v4.17.1 (2026-03-30)

#### Bug Fixes

- fake commit to trigger release. (3ead521)

#### Build System

- makefile: Simplify the build-pkg target. (744f818)
- makefile: Add build-pkg target for package managers (c3d8ab0)

## v4.17.0 (2026-03-27)

#### Features

- plugin: Add info subcommand to get plugin details (4f7084f)

## v4.16.0 (2026-03-26)

#### Features

- db: Implement robust database schema migration (af13a33)
- db: Add monitor and slack tables and migration (e6278e8)
- cache: Sanitize plugin info on all reads and writes (3f4795c)
- db: Introduce SQLite database for plugin information (2407962)

#### Continuous Integration

- github: Remove release commit collection from workflow (d9650a5)
- github: Update CI to use #jman_dev channel (89aa5dc)

## v4.15.0 (2026-03-23)

#### Features

- mods: Add ability to enable/disable file mods (5fc3fbb)

#### Bug Fixes

- cache: sanitize plugin metadata by decoding entities and stripping tags (2dbf6cf)

#### Refactor

- cache: make JSON cache TTL configurable (821f0e4)

#### Maintenance

- fixed version oopsie. (fc9119a)

### v4.14.1 (2026-03-23)

#### Bug Fixes

- wpcli: Pass skip parameter to GetPlugins (a69cd59)
- admin: Use normal verbosity for user creation messages (95a76ad)

#### Refactor

- wpcli: Introduce CliOptions struct (5b4f5af)

#### Documentation

- Add prerequisites to README (e70376a)
- Update installation instructions (dd4cf7d)

## v4.14.0 (2026-03-22)

#### Features

- cache: Refactor plugin info update logic (18cf2f1)
- api: Refactor response handling and add plugin info endpoint (f528bdf)
- cache: Add version comparison for plugin updates (aae3c3d)
- cache: Add plugin info caching and fetching (d421f32)

#### Bug Fixes

- cache: sanitize plugin metadata by decoding entities and stripping tags (b1ee971)
- cache: Remove Latest field update from WPVuln (284ca81)

#### Refactor

- cache: make JSON cache TTL configurable (9aa3275)

## v4.13.0 (2026-03-20)

#### Features

- cmd: Add CLI tools for user and credential management (a587b1b)
- api: Implement JWT authentication and rate limiting (0f9f84d)

### v4.12.1 (2026-03-20)

#### Bug Fixes

- update: Use AppVersion for current version check (0ff1310)

## v4.12.0 (2026-03-20)

#### Features

- plugin: colorize site names in output (c01362d)
- plugin: Add plugin alias support (62453fd)

#### Bug Fixes

- plugin: Correct site name formatting in remove output (6b18263)
- wpcli: Ensure error is returned from RunWP (92272c5)
- cache: Improve plugin fetch error reporting (f42dc82)
- wpcli: Improve error handling for WP-CLI commands (4746710)
- plugin: Improve error handling for plugin operations (0e52537)

## v4.11.0 (2026-03-19)

#### Features

- wpcli: Return new version and language from UpdateCore (45b45df)

#### Bug Fixes

- wpcli: Return structured data from UpdateCore (01185b5)

#### Refactor

- verb: Rename ansi to verb and move ANSI color functions (6723f7d)
- verbosity: Rename verbosity package to verb (53e1f89)

### v4.10.1 (2026-03-19)

#### Bug Fixes

- wpcli: Disable skip in RunWP for plugin actions (4d8c0d6)

## v4.10.0 (2026-03-19)

#### Features

- wpcli: Enhance core update check output (8bd26a2)
- core: improve core check, update, and version commands (8ae5c56)
- core: Command to check core for updates and to update. (8b505b8)

#### Bug Fixes

- wpcli: Use strings.SplitSeq for error splitting (4cf3eac)
- RunWP better error handling (f529788)
- verbosity: Use verbosity.Println for cancelled operation message (5373e8f)
- wpcli: Make update regex multiline aware (30bb762)
- wpcli: Print update core output verbosely (e87b73f)

#### Build System

- makefile: Inject app version into LDFLAGS via config (35848d9)

## v4.9.0 (2026-03-10)

#### Features

- plugin: batch updates and implement removal (a3b696b)
- search: allow selecting specific sites by index in results prompt (677316e)
- plugin: improve plugin list output formatting (b300503)
- plugin: add list, update, and remove subcommands (cc6797b)

#### Bug Fixes

- mods: show status messages at normal verbosity (a466a6f)

#### Refactor

- fetch: remove redundant pointer indirection (b455e7d)

## v4.8.0 (2026-03-09)

#### Features

- config: integrate viper for configuration and environment support (303dbac)

## v4.7.0 (2026-03-03)

#### Features

- api: add main entry point (1ae656a)
- vuln: enhance version matching and reporting (8ddfa04)

#### Bug Fixes

- vuln: return error for unknown operators in versionCompare (4367114)

#### Refactor

- vuln: remove version comparison fallback and simplify filtering (8257d23)

#### Maintenance

- remove accidental copy of the entrypoint. (18e4409)

## v4.6.0 (2026-02-27)

#### Features

- monitor: set custom User-Agent header for monitoring requests (c108fdf)

## v4.5.0 (2026-02-27)

#### Features

- monitor: log duration of monitoring check (3e6463e)

#### Refactor

- monitor: use debug log level for ignored sites (9427f90)
- verbosity: migrate standard logging to level-aware LogPrintf (fc5e59e)
- monitor: move monitoring logic to internal package (5a0a31f)
- api: move middleware into api package (bfd0c69)
- api: move route handlers to internal/api package (767bb39)

#### Documentation

- readme: reorganize sidecar utility documentation (3ceeeb4)

#### Maintenance

- rename internal/api package to internal/fetch (65328b0)

## v4.4.0 (2026-02-26)

#### Features

- update: add support for updating api and monitor components (f4d3e4f)
- monitor: add site health monitoring tool (0e34b23)

#### Bug Fixes

- update: use verbosity levels for output messages (db84baf)

#### Refactor

- monitor: use cached sites and remove PLAN.md (6b9a34e)

#### Continuous Integration

- github: add jman-monitor to release artifacts (68b7a6d)

## v4.3.0 (2026-02-26)

#### Features

- search: add plugin search and case-insensitive site matching (c3207cd)
- api: add jman-api REST service (3c47a52)

#### Bug Fixes

- cache: filter out non-WordPress sites from site list (9252780)

#### Refactor

- api: move middleware to internal package and update health route (8b1939c)
- api: move middleware to internal package and update health route (5105571)
- api: update fetch command references and health endpoint (5b06221)

#### Continuous Integration

- include jman-api in release artifacts (35e4e03)

#### Maintenance

- remove MainWP integration and rename Slack config (6dca387)
- remove MainWP integration and rename Slack config (a04c536)

## v4.2.0 (2026-02-24)

#### Features

- fetch: add support for fetching plugin vulnerabilities (a6b88d3)

#### Continuous Integration

- github: skip release steps when no version change is detected (f5f5712)

#### Maintenance

- zed: remove editor settings (6ffd734)

## v4.1.0 (2026-02-24)

#### Features

- fetch: support targeting specific resources for cache update (9f7b8ce)

#### Bug Fixes

- wpcli: improve error reporting (c13d95b)

#### Continuous Integration

- remove build and release workflow (82f823f)
- github: update release workflow with build and notifications (103aae3)

### v4.0.1 (2026-02-24)

#### Bug Fixes

- separate stdout and stderror to allow piping to files. (cb92241)

#### Refactor

- cache: use verbosity package for logging (7f606a6)
- verbosity: use verbosity package for output instead of fmt (1da6513)

#### Continuous Integration

- github: update version-file path to version.json (98423dc)
- add automated release workflow and version file (8949000)

#### Misc

- Refactor verbosity API and improve output handling (4440d11)

## v4.0.0 (2026-02-24)

#### Features

- update: show download progress during updates (64cfd46)
- update: implement automatic self-updating (bd05197)
- update: add command to check for latest version (d4a0423)
- cmd: add verbosity flags and level management (079aa87)
- verbosity: implement verbosity control and conditional printing (ed95ec5)

#### Bug Fixes

- vuln: fix typo in CVSS score label (8b6f326)
- update: require valid download URL for update notification (38f4678)
- root: show version only in verbose mode (4f203d4)
- vuln: sanitize HTML tags and entities in reports (d09e076)

#### Performance Improvements

- cache: limit concurrent plugin fetching to 24 (4465ae7)
- cache: fetch plugins concurrently (0642e3f)

#### Refactor

- cache: use verbosity level for plugin vulnerability logging (bc2ee73)
- vuln: use verbosity package for plugin processing output (d14069d)
- vuln: use slices.Contains and fmt.Fprintf (d57f2db)
- alias: replace interface{} with any (d5d6abe)

#### Build System

- ci: migrate build workflow from Bun to Go (6ec70f7)

#### Continuous Integration

- github: update Go version to 1.25.x (5d258a9)

#### Maintenance

- inactive: remove unused comments (6b85902)
- remove PLAN.md and add dev target to Makefile (f9d81b0)
- cleanup and documentation (888c4f2)
- rewrite the whole thing in Go (5811819)

### v3.4.8 (2026-02-17)

#### Continuous Integration

- Github: fix commit range and skip tag commit when generating changelog (910fabc)

### v3.4.7 (2026-02-17)

#### Continuous Integration

- workflow: exclude commit pointed to by CURRENT_TAG when counting commits (7215465)
- workflows: use env-based Slack message and fix commit summary formatting (835555c)

### v3.4.6 (2026-02-17)

#### Continuous Integration

- slack: send Slack notifications as JSON payload using toJSON and format (42044d7)

### v3.4.5 (2026-02-17)

#### Continuous Integration

- workflow: update Slack action to use method/token/payload format (d2f7a36)

### v3.4.4 (2026-02-17)

#### Continuous Integration

- github: use explicit newline escape when truncating commit list (e11af0b)

### v3.4.3 (2026-02-17)

#### Continuous Integration

- workflow: limit and format release commit list, add totals, upgrade Slack action to v2 (729e91d)

### v3.4.2 (2026-02-17)

#### Continuous Integration

- github: fetch full history and include release commit list in Slack notifications (a71264a)

### v3.4.1 (2026-02-17)

#### Continuous Integration

- workflow: add Slack notification step to release job (f2e137d)

## v3.4.0 (2026-02-17)

#### Features

- commands: add targeted fetch option and force flag to cache getters (e87c0ad)

## v3.3.0 (2026-02-11)

#### Features

- helpers: add progress indicator to release downloads (ae7db2c)
- slack: support string-based durations for message tracking (8d39658)

## v3.2.0 (2026-02-11)

#### Features

- slack: prevent duplicate messages using CRC32 hashing (15dbb4a)

## v3.1.0 (2026-02-10)

#### Features

- vuln: add ability to ignore specific sites (616e755)

#### Bug Fixes

- slack: handle missing slack token (2ae9dc7)

#### Refactor

- report: change plugin version separator to hyphen (767fec3)

## v3.0.0 (2026-02-10)

#### Features

- vuln: add slack notification support (d3cb4ba)
- vuln: add site-based reporting and refactor scanning logic (01a9241)
- report formatting to use source data (bb70891)

#### Refactor

- slack: move messaging to dedicated module and use WebClient SDK (c3d1382)
- CVSS score retrieval (1f7b444)

### v2.3.2 (2026-02-02)

#### Bug Fixes

- changed to correct version number. (9df98e8)
- Improve update jman reliability (f3d704b)

### v2.3.1 (2026-02-02)

#### Bug Fixes

- Add unlinkSync to updateJman command (38321f7)

## v2.3.0 (2026-02-02)

#### Features

- Add update command and logic (331a3b1)

#### Bug Fixes

- Remove unused imports from utils (8cee35e)

#### Refactor

- createAliases command (ac3f4ec)

#### Documentation

- Added documentation for vuln command to readme (ae0bc8d)
- Added comments to functions (0a74c2e)

## v2.2.0 (2026-01-27)

#### Features

- Add skip option to runWP function (1bc25a2)
- Add CVSS threshold filtering (16028af)

#### Misc

- Add check for wp-cli executable (37b6c9a)

### v2.1.1 (2026-01-22)

#### Features

- Add CVSS threshold for Slack notifications (6779111)

#### Bug Fixes

- Remove autochangelog (1b64e36)
- type imports in commands.ts (5abf1ad)

### v2.0.1 (2026-01-22)

#### Build System

- add binary install script to makefile (7c914b0)

#### Continuous Integration

- Update release artifact path (1588afe)

## v2.0.0 (2026-01-22)

#### Bug Fixes

- Add check for config file existence (6095705)

#### Refactor

- Replace @folder/xdg with xdg-basedir (fb57940)

#### Misc

- Migrate to Bun and update dependencies (f0b7f09)

## v1.5.0 (2026-01-20)

#### Features

- Refactor vulnerability scanning to prevent duplicate slack messages (dd7cff1)
- Add html-entities dependency (28f0993)
- Refactor scanVulnerabilities to process vulns (4d5d8e8)
- Add vulnerability type definitions (d7afb8f)
- Add slackHook to config schema (eaa662a)
- Add cache for WordPress vulnerabilities (b51c460)
- Add functions for Slack and WP vulnerabilities (793acc6)
- Plugin list cache and processing (5af2122)
- Enforce Node.js version in package.json (602ef06)
- Add dummy function for vulnerability check. WIP (bf6bc5f)
- Add function to fetch plugins from site. (f125402)

#### Bug Fixes

- Refactor plugin caching and vulnerability processing (734eae2)
- Improve vulnerability caching and schema validation (a9e10f1)

### v1.4.9 (2026-01-16)

### v1.4.8 (2026-01-16)

#### Documentation

- Replace conventional-changelog with auto-changelog (f92fb86)

#### Misc

- Add description and keywords to package.json (46a2a47)

### v1.4.7 (2026-01-16)

#### Documentation

- Add README.md with project documentation (1277b3d)

### v1.4.6 (2026-01-16)

#### Misc

- Add repository field to package.json (90d853d)

### v1.4.5 (2026-01-16)

#### Bug Fixes

- Remove unnecessary permissions from build job (4e095fb)

### v1.4.4 (2026-01-16)

#### Continuous Integration

- Added paermission settings (08c9997)

### v1.4.3 (2026-01-16)

#### Continuous Integration

- Configure NPM publish in CI (29af36b)

### v1.4.2 (2026-01-16)

### v1.4.1 (2026-01-16)

#### Refactor

- build scripts to use npm run commands (b12e4d8)

## v1.4.0 (2026-01-15)

#### Features

- Add NPM publish to CI and bin entry (7356071)
- Add getErrorMessage utility function (131f10a)

### v1.3.3 (2026-01-12)

#### Refactor

- Make command functions async and update fetchData usage (5a94527)

### v1.3.2 (2026-01-12)

#### Refactor

- command handling and add fetchData, listData helpers (c6275f6)
- command handlers into separate commands module (b3c8491)

### v1.3.1 (2025-11-21)

#### Bug Fixes

- Remove faulty import. Improve plugin install error handling and usage messages (7a058ef)

## v1.3.0 (2025-11-21)

#### Features

- Add plugin install command and support for repo plugin URLs (d225384)

## v1.2.0 (2025-11-06)

#### Features

- file mods command (a47711e)
- MainWP user password reset and refactor command handlers (9a528d2)

### v1.1.1 (2025-11-04)

#### Refactor

- command parsing and handling logic (701fbd9)

## v1.1.0 (2025-11-03)

#### Features

- Add inactive command to list sites without active MainWP (3636840)

#### Bug Fixes

- Handle user cancellation in promptSearch without throwing (b9f19fc)

## v1.0.0 (2025-10-31)

#### Bug Fixes

- Remove unused API_MAINWP_URL import from constants (e85ce1f)

#### Misc

- BREAKING CHANGE: Change config format. Add MainWP install command and update config handling (fadef45)
- Refactor alias creation and site search logic (b010210)
- Add @topcli/prompts and implement interactive site search (0d8c794)
- Add WP CLI integration and site search utilities (24d9630)

### v0.1.3 (2025-10-28)

#### Bug Fixes

- Update version script and improve error logging (b25e5b8)

### v0.1.2 (2025-10-28)

#### Build System

- Update build target to use dist/jman instead of bin/jman (c9b06b5)

### v0.1.1 (2025-10-28)

#### Continuous Integration

- Use make to build instead of pnpm in CI workflow (41bf42f)

## v0.1.0 (2025-10-28)

#### Features

- Implement caching for servers and sites (f9308e4)
- Implement basic site listing (efa73ad)

#### Misc

- Add GitHub Actions build workflow and clean up files (341d946)
- Add alias generation and command parsing utilities (082a493)
- Basic server fetch (9f29cd8)

