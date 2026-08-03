# Changelog

### 1.31.1 (2026-08-03)

#### Bug Fixes

- organization: fix plugin audit visibility and prefill convert-to-asset modal (fe96449)

#### Refactor

- ui: manage asset price as string input (4422ac3)

## v1.31.0 (2026-06-04)

#### Features

- plugin: add support for updating vulnerable plugins (ef6a8ae)

#### Refactor

- organization: extract modal components from detail view (8e3c54a)

### v1.30.1 (2026-06-04)

#### Refactor

- assets: decouple asset management logic from organization details (1bb781c)

## v1.30.0 (2026-06-04)

#### Features

- view: update header actions in TemplatesView (74d20ea)
- asset: add organization selection and linking to asset management (070e7a4)
- components: implement reusable AssetEditModal component (0dccdcc)
- assets: add edit modal to asset list (de52cc0)
- task: display completion details in task info modal (f5412bf)
- settings: add slack reminder time configuration (a9749d0)

#### Refactor

- style: move component-specific styles to global CSS (47b63f7)

#### Styles

- ui: standardize AppIcon rendering and asset action button (4bd87c3)

## v1.29.0 (2026-05-25)

#### Features

- ui: implement asynchronous confirmation modal (d4be1e7)

#### Bug Fixes

- auth: log server response body on login failure (9a0b944)

#### Refactor

- icons: convert SVG assets to Vue components (1cc60fb)
- api: centralize BASE_URL definition (482306f)
- api: use centralized error handler in stores (385da54)

#### Documentation

- AGENTS: update icon implementation details and document intentional design decisions (9281562)

### v1.28.1 (2026-05-22)

#### Bug Fixes

- dashboard: prevent null item insertion during drag and drop reordering (4c0f9fc)

## v1.28.0 (2026-05-22)

#### Features

- dashboard: implement modular and reorderable widget system (4bea0bd)

### v1.27.2 (2026-05-22)

#### Refactor

- data: update vulnerability enrichment logic in store (315ab31)
- data: normalize vulnerability data structures (cbe2580)

#### Maintenance

- store: update vulnerabilities cache key (a75d409)

### v1.27.1 (2026-05-21)

#### Styles

- components: remove text-transform from badge-sm (d73b4c9)

## v1.27.0 (2026-05-21)

#### Features

- style: add table column width and text alignment utility classes (4e9e16e)
- vulnerability: implement suppression status for ignored vulnerabilities (94f0e3b)

#### Bug Fixes

- vulnerability: exclude suppressed vulnerabilities from display and data processing (6493610)
- task: improve task filtering and vulnerability suppression naming (119ae13)

#### Refactor

- data: use backend-provided suppression status for vulnerabilities (e587d3c)

## v1.26.0 (2026-05-19)

#### Features

- vulnerability: add copy to clipboard functionality for vulnerability UUIDs (63b3313)

## v1.25.0 (2026-05-19)

#### Features

- store: optimize ignore lookup logic using computed index (06c22cd)
- settings: add edit functionality for ignore rules (b46a964)
- ignore: implement unified ignore management system (b191593)

#### Bug Fixes

- components: add domain-based lookup fallback for monitor history ignore status (af1d51c)

#### Refactor

- components: reorder script blocks for consistency (d325282)
- sites: reorder reactive state declarations in SitesView (8c91278)

### v1.24.1 (2026-05-18)

#### Refactor

- view: type InfoItem for site info items in SiteDetailView (3054990)

## v1.24.0 (2026-05-18)

#### Features

- site: add database table prefix to site info details (ec8ae39)
- settings: add slack integration and replace modal close buttons with icons (3dfe80e)
- components: implement AppIcon component and replace inline SVGs (2963145)

#### Bug Fixes

- site: remove public folder field from site details (990df09)

#### Refactor

- ui: update organization tables to use standardized components and utilities (1752638)

### v1.23.2 (2026-05-14)

#### Refactor

- style: modularize and standardize CSS architecture (511f415)
- dashboard: move vulnerability widget position (7047a7d)

#### Documentation

- add project styling guidelines documentation (3baab7b)

### v1.23.1 (2026-05-14)

#### Bug Fixes

- ui: improve date parsing logic in TaskFormModal (de2cfff)

## v1.23.0 (2026-05-14)

#### Features

- task: improve task management and user assignment (9a4e7d0)
- tasks: implement task management system (9336b04)

#### Bug Fixes

- tasks: update current date reference and improve code formatting (84beac5)

## v1.22.0 (2026-05-13)

#### Features

- ui: display update result status in modal badges (0cd7b68)

## v1.21.0 (2026-05-13)

#### Features

- PluginSiteUpdateModal: add support for updating only vulnerable plugin instances (40cc4bc)

## v1.20.0 (2026-05-13)

#### Features

- plugin: add modal to update plugin across sites (d3dba88)

## v1.19.0 (2026-05-13)

#### Features

- data: update local plugin version after successful update request (b668783)

## v1.18.0 (2026-05-13)

#### Features

- plugin: add update modal for site plugins (d65431c)

#### Refactor

- types: replace PluginUpdate with Plugin interface (c0746ca)
- plugin: move update logic to Pinia store (3012723)

#### Styles

- site: add visual separator to plugins header (10c2b70)

## v1.17.0 (2026-05-08)

#### Features

- settings: synchronize settings with API (f687387)

## v1.16.0 (2026-05-08)

#### Features

- dashboard: add vulnerability dashboard widget (d3e7ad9)

## v1.15.0 (2026-05-07)

#### Features

- auth: add admin user level and restrict user management access (c5a0dca)
- settings: allow password resets in user edit modal (caa9c6b)

#### Refactor

- settings: move ignored domains to separate component and tab (e9f54b2)

#### Maintenance

- docs: remove deprecated API_SPECS.md file (3b0f352)

## v1.14.0 (2026-05-06)

#### Features

- user: add loading and error states for profile fetching (8c5f9a1)
- settings: add user management and 2FA configuration (a168fdc)

#### Bug Fixes

- settings: validate password strength in user form submission (57795ce)

#### Maintenance

- lint: configure ESLint and Prettier (275da79)

### v1.13.1 (2026-05-05)

#### Bug Fixes

- navigation: rename assets route to inventory (ef5ce4d)

## v1.13.0 (2026-05-05)

#### Features

- ui: implement toast notifications for error handling (507257a)
- auth: implement role-based access control with level-based permissions (417442f)
- assets: implement asset and subscription management module (26b4b06)

## v1.12.0 (2026-05-04)

#### Features

- audit: add audit trail to organization view (ccda9aa)

#### Bug Fixes

- organization: update phone placeholder format (c9a2ae1)

## v1.11.0 (2026-05-04)

#### Features

- organization: sync site links on organization fetch (24a6292)

#### Styles

- ui: implement responsive design for mobile screens (a5f5db7)

## v1.10.0 (2026-05-03)

#### Features

- organization: add debounce and request cancellation to search functionality (f46a89e)
- EditableInfoCard: add async save support and loading state (50d54ee)
- organization: display linked sites in organizations table (3fb85da)
- data: implement local caching for site-company associations (b494b18)
- site: implement site and company linking functionality (c80bf4e)
- company: implement company and contact management system (81cdea9)

#### Bug Fixes

- component: allow falsy values in EditableInfoCard (7d4fd54)
- monitor: prevent marking history as fetched on error (439a342)

#### Refactor

- style: remove inline styles in favor of scoped CSS classes (40daa90)
- organization: rename companies to organizations (32267b4)

### v1.9.3 (2026-04-21)

#### Bug Fixes

- timeline: render history blocks using duration-based flex-grow and ensure visibility (47fc639)

### v1.9.2 (2026-04-21)

#### Bug Fixes

- components: preserve fractional uptime percentage and format to two decimals (b532903)
- monitor-history-card: treat error_code 0 as unknown and add status-unknown style (b0213d2)

### v1.9.1 (2026-04-21)

#### Bug Fixes

- monitor: show tiny/zero-duration timeline items and make status checks case-insensitive (2b54626)

## v1.9.0 (2026-04-21)

#### Features

- monitor: render 24h proportional status timeline and update uptime calc (eeb4ae5)

### v1.8.1 (2026-04-21)

#### Bug Fixes

- components: avoid accessing first_seen when sortedHistory is empty (4878228)

## v1.8.0 (2026-04-21)

#### Features

- monitor: display ignored domains and pending live status (e6e1601)
- settings: add settings view and store with refresh interval controls (3c4ddb8)
- monitor: add monitor store and history UI component (6b6cd14)

#### Maintenance

- config: add foonver.toml to enable push and changelog (5b79cbd)

## v1.7.0 (2026-04-17)

#### Features

- components: add refresh button to AppNav and copy-to-clipboard in InfoCard (e45fa5a)

## v1.6.0 (2026-04-15)

#### Features

- components: add reusable UI components and integrate into views (7279d3f)
- data: add vulnerabilities to enriched types and surface them in UI (8b7e921)
- vulns: add vulnerability support to store and UI (5bedf9c)

#### Performance Improvements

- data: add computed lookup maps and replace repeated filters with map lookups (2313857)

#### Refactor

- vulnerabilities: move inline styles into scoped CSS classes in PluginVulnerabilityList.vue (9c5cf8e)
- components: rename types, compress CSS, move vulnerability list in detail view (be3a1cd)

## v1.5.0 (2026-03-26)

#### Features

- router: Introduce Dashboard and Sites views (7de3123)

## v1.4.0 (2026-03-23)

#### Features

- router: add query params for pagination (dd95dd4)
- data: Add enrichedSites computed property (af3ccd0)

#### Bug Fixes

- data: Display server name or "Unknown Server" (69aae28)

### v1.3.2 (2026-03-23)

#### Bug Fixes

- types: remove unused vue-router import (1a6bda5)

#### Documentation

- add CHANGELOG.md (5cc2d8d)

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

