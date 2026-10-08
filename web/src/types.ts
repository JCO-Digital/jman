export interface DiskSpace {
	total: number;
	available: number;
	used: number;
	updated_at: string;
}

export interface ServerDatabase {
	server: string;
	host: string;
	port: number;
}

/** Site/server provider. "spinupwp" entities carry extra SpinupWP-only detail fields. */
export type Provider = "spinupwp" | "manual" | "wpengine" | (string & {});

/**
 * A server from GET /api/servers. `id` is always a UUID; the legacy SpinupWP
 * integer id (if any) lives in `provider_server_id`. SpinupWP-only detail
 * fields are present only when `provider === "spinupwp"`.
 */
export interface Server {
	id: string;
	provider: Provider;
	provider_server_id?: string;
	name: string;
	is_logical: boolean;
	ip_address?: string;
	ssh_port?: number;
	// SpinupWP-only
	provider_name?: string;
	ubuntu_version?: string;
	timezone?: string;
	region?: string;
	size?: string;
	disk_space?: DiskSpace;
	database?: ServerDatabase;
	ssh_publickey?: string;
	git_publickey?: string;
	connection_status?: string;
	reboot_required?: boolean;
	upgrade_required?: boolean;
	install_notes?: string;
	created_at?: string;
	status?: string;
}

export interface AdditionalDomain {
	domain: string;
	redirect: {
		enabled: boolean;
	};
	created_at: string;
}

export interface SiteDatabase {
	id?: number;
	user_id?: number;
	table_prefix?: string;
}

export interface StorageProvider {
	id: number;
	region: string;
	bucket: string;
}

export interface Backups {
	files: boolean;
	database: boolean;
	paths_to_exclude: string;
	is_backups_retention_period_enabled: boolean;
	retention_period: number;
	next_run_time: string | null;
	storage_provider: StorageProvider;
}

export type SiteEnvironment =
	| "production"
	| "staging"
	| "development"
	| "archived";

export interface SiteDiskUsage {
	bytes_used: number;
	measured_at: string;
}

/**
 * WordPress's core auto-update setting: "true" (major updates too),
 * "minor", "false", "disabled" (all automatic updates off), "default"
 * (minor only), or "" if unknown (an agent too old to report it).
 */
export type AutoUpdateCore =
	| "true"
	| "minor"
	| "false"
	| "disabled"
	| "default"
	| "";

export interface SiteWpFlags {
	is_multisite: boolean;
	disallow_file_mods: boolean;
	auto_update_core: AutoUpdateCore;
	updated_at: string;
}

/**
 * Restricts updates on a site (empty plugin: core and every plugin) or of
 * one plugin to fix releases. Bulk and ordinary updates of locked items
 * only install patch versions; a bigger update needs an explicit,
 * confirmed major update, and the lock stays in place.
 */
export interface UpdateLock {
	id: number;
	site_id: string;
	/** Plugin slug, or "" for a site lock. */
	plugin: string;
	comment: string;
	created_by: string;
	created_at: string;
}

/**
 * A site from GET /api/sites. `id` and `server_id` are UUIDs; the legacy
 * SpinupWP integer id (if any) lives in `provider_site_id`. SpinupWP-only
 * detail fields are present only when `provider === "spinupwp"`.
 */
export interface Site {
	id: string;
	provider: Provider;
	provider_site_id?: string;
	server_id: string | null;
	/** "" when the site has no server. */
	server_name: string;
	organization_id?: number;
	domain: string;
	site_user: string;
	/**
	 * SSH target jman connects to for WP-CLI ("user@host", or
	 * "user@host:port" for a non-default port). Absent when the site has no
	 * SSH host.
	 */
	ssh?: string;
	php_version: string;
	is_wordpress: boolean;
	status: string;
	// Capabilities
	connection_type: ManagedConnectionType;
	can_wp_cli: boolean;
	has_agent: boolean;
	has_monitoring: boolean;
	// SpinupWP-only
	additional_domains?: AdditionalDomain[];
	user_auth?: string;
	public_folder?: string;
	page_cache?: {
		enabled: boolean;
	};
	https?: {
		enabled: boolean;
		certificate_expires: string | null;
		certificate_renews: string | null;
	};
	nginx?: {
		uploads_directory_protected: boolean;
		xmlrpc_protected: boolean;
		subdirectory_rewrite_in_place: boolean;
	};
	database?: SiteDatabase;
	backups?: Backups;
	wp_core_update?: boolean | number;
	wp_theme_updates?: boolean | number;
	wp_plugin_updates?: boolean | number;
	basic_auth?: {
		enabled: boolean;
		username: string;
	};
	created_at?: string;
	created_by?: string;
	updated_at?: string;
	updated_by?: string;
	environment?: SiteEnvironment;
	disk_usage?: SiteDiskUsage;
	wp_flags?: SiteWpFlags;
	last_update?: SiteUpdateLedgerEntry;
	wp_core?: SiteCore;
	update_locks?: UpdateLock[];
	/** jman-agent's WordPress data collection, once it has tried one. */
	agent_wp?: AgentWPStatus;
}

/**
 * jman-agent's collection of a site's plugins and core version. Until the
 * first successful collection the site stays on the periodic SSH refresh.
 */
export interface AgentWPStatus {
	collected_at?: string;
	/** The latest failure, cleared by the next successful collection. */
	error?: string;
	error_at?: string;
}

export interface SiteCore {
	site_id: string;
	version: string;
	minor_update?: string;
	major_update?: string;
}

export interface Plugin {
	site_id: string;
	name: string;
	slug?: string;
	status: string;
	version: string;
	update: string;
	autoUpdate: string | boolean;
}

export interface PluginInfo {
	name: string;
	slug: string;
	version: string;
	author: string;
	author_profile: string;
	requires: string;
	tested: string;
	last_updated: string;
	homepage: string;
}

export interface VulnerabilitySource {
	id: string;
	name: string;
	link: string;
	description: string;
	date: string | null;
}

export interface VulnerabilityImpact {
	cvss?: {
		version: string;
		vector: string;
		score: string;
		severity: string;
	};
	cwe?: Array<{
		cwe: string;
		name: string;
		description: string;
	}>;
}

export interface VulnerabilitySite {
	site_id: string;
	site_name: string;
	version: string;
	suppressed: boolean;
}

export interface Vulnerability {
	uuid: string;
	name: string;
	description: string | null;
	operator: {
		max_version: string | null;
		max_operator: string | null;
		unfixed: string;
		closed: string;
	};
	source: VulnerabilitySource[];
	impact: VulnerabilityImpact;
	sites: VulnerabilitySite[];
	suppressed: boolean;
}

export interface PluginVulnerability {
	plugin: string;
	slug: string;
	plugin_name: string;
	suppressed: boolean;
	vulnerabilities: Vulnerability[];
}

export interface EnrichedVulnerability extends Vulnerability {
	slug: string;
	plugin_name: string;
	plugin_suppressed: boolean;
}

// Vulnerabilities affecting a WordPress core version, as reported by
// GET /api/vulns/core. The wpvulnerability.net core endpoint already scopes
// results to the requested version, so every entry applies to every listed site.
export interface CoreVulnReport {
	version: string;
	vulnerabilities: Vulnerability[];
	suppressed: boolean;
}

export interface EnrichedCoreVulnerability extends Vulnerability {
	core_version: string;
}

export interface MonitorHistory {
	id: number;
	domain: string;
	status: string;
	error_code: number;
	first_seen: string;
	last_seen: string;
	count: number;
}

export interface MonitorStatus {
	domain: string;
	is_down: boolean;
	failure_count: number;
	last_checked: string | null;
	status_message?: string;
}

/**
 * Update ledger statuses. "full": the site was brought up to date; "vuln":
 * its vulnerable plugins were updated but other updates remain; "partial":
 * updates remain or an action worked for only some plugins. The rest record
 * plugin management actions.
 */
export type LedgerStatus =
	| "full"
	| "vuln"
	| "partial"
	| "failed"
	| "activated"
	| "deactivated"
	| "deleted"
	| "installed"
	/** A change jman found on the site but didn't make itself. */
	| "detected";

export interface SiteUpdateLedgerEntry {
	id: number;
	site_id: string;
	update_type: "core" | "plugin" | "theme";
	status: LedgerStatus;
	data_json?: string;
	updated_by: string;
	updated_at: string;
}

export type IgnoreType = "site" | "server" | "plugin" | "vulnerability";

export interface IgnoreEntry {
	id: number;
	type: IgnoreType;
	target: string;
	reason: string;
	negated_site_ids: string[] | null;
	use_for_monitor: boolean;
	use_for_vuln: boolean;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export interface CreateIgnorePayload {
	type: IgnoreType;
	target: string;
	reason?: string;
	negated_site_ids?: string[];
	use_for_monitor?: boolean;
	use_for_vuln?: boolean;
}

export type UpdateIgnorePayload = Partial<CreateIgnorePayload>;

export interface EnrichedSite extends Site {
	server: string;
	plugins: Plugin[];
	vulnerabilities: EnrichedVulnerability[];
	coreVulnerabilities: EnrichedCoreVulnerability[];
	monitorHistory?: MonitorHistory[];
	monitorStatus?: MonitorStatus;
}

export interface EnrichedPlugin extends PluginInfo {
	shortName: string;
	count: number;
	vulnerabilities: EnrichedVulnerability[];
}

export interface Organization {
	id: number;
	name: string;
	vat_number: string | null;
	info: string | null;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export type ContactType = "Main" | "Technical" | "Billing";

export interface Contact {
	id: number;
	organization_id: number;
	name: string;
	email: string | null;
	phone: string | null;
	type: ContactType;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export interface EnrichedOrganization extends Organization {
	contacts: Contact[];
}

export type AssetType =
	| "Plugin"
	| "Domain"
	| "Hosting Package"
	| "Service Package"
	| "General";
export type BillingFrequency = "Yearly" | "Quarterly" | "Monthly" | "One-time";

export type PaymentMethodType = "Buy" | "Sell";

export interface PaymentMethod {
	id: number;
	name: string;
	type: PaymentMethodType;
	expiry_date: string | null;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export interface CreatePaymentMethodPayload {
	name: string;
	type: PaymentMethodType;
	expiry_date?: string | null;
}

export interface Asset {
	id: number;
	type: AssetType;
	identifier: string | null;
	name: string;
	description: string | null;
	default_price: number | null;
	default_freq: BillingFrequency | null;
	active: boolean;
	payment_method_id: number | null;
	payment_method_name?: string;
	purchase_price: number | null;
	quantity: number;
	next_payment: string | null;
	management_url: string | null;
	management_account: string | null;
	license_key?: string | null;
	usage_count?: number;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export type OrganizationAssetStatus = "active" | "cancelled" | "paused";

export interface OrganizationAsset {
	id: number;
	organization_id: number;
	organization_name?: string;
	site_id: string | null;
	asset_id: number | null;
	asset_name?: string;
	asset_type?: string;
	identifier: string | null;
	price: number;
	billing_freq: BillingFrequency;
	next_billing: string | null;
	last_billed: string | null;
	status: OrganizationAssetStatus;
	description: string | null;
	payment_method_id: number | null;
	payment_method_name?: string;
	asset_purchase_price?: number;
	asset_quantity?: number;
	asset_next_payment?: string | null;
	asset_management_url?: string;
	asset_management_account?: string;
	license_key?: string | null;
	asset_license_key?: string;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export interface AssetPayment {
	id: number;
	organization_asset_id: number;
	amount: number;
	payment_date: string;
	info: string | null;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export interface EnrichedOrganizationAsset extends OrganizationAsset {
	asset?: Asset;
	payments?: AssetPayment[];
}

export type UserLevel = "basic" | "edit" | "execute" | "admin";

export type NoteParentType = "Organization" | "Site" | "Plugin";

export interface Note {
	id: number;
	parent_type: NoteParentType;
	parent_id: string | number;
	content: string;
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
}

export interface AdminUser {
	username: string;
	displayName: string;
	level: UserLevel;
	has2FA: boolean;
}

export interface CreateUserPayload {
	username: string;
	password: string;
	displayName: string;
	level?: UserLevel;
}

export interface UpdateUserPayload {
	displayName?: string;
	level?: UserLevel;
	password?: string;
}

/** The outcome of updating one plugin, or WordPress core ("WordPress"). */
export interface UpdateResult {
	name: string;
	old_version: string;
	new_version: string;
	/** "Updated", "Up to date", "failed", "Done" or "Skipped (locked)". */
	status: string;
	error?: string;
	/** Explains a skipped update, e.g. a vulnerability the lock holds back. */
	note?: string;
}

export type UpdateJobStatus =
	| "queued"
	| "running"
	/** Ran to completion; individual results may still have failed. */
	| "done"
	/** Couldn't run at all (e.g. site unreachable). */
	| "failed"
	/** jman-api restarted mid-update; the outcome on the site is unknown. */
	| "interrupted";

/** Plugin management job kinds that act on a set of installed plugins. */
export type PluginActionKind =
	| "activate"
	| "deactivate"
	| "delete"
	| "uninstall";

export type UpdateJobKind = "plugins" | "core" | PluginActionKind | "install";

/**
 * A background change on one site: `wp plugin update` for one or more
 * plugins (kind "plugins"), `wp core update`, activating, deactivating,
 * deleting or uninstalling plugins, or installing one plugin.
 */
export interface UpdateJob {
	id: number;
	kind: UpdateJobKind;
	site_id: string;
	status: UpdateJobStatus;
	/** Plugins to act on; empty for core and install jobs. */
	plugins: { name: string; old_version: string }[];
	/** Core jobs: "minor" or "major". */
	target?: "minor" | "major";
	/** Install jobs: the slug, ZIP URL or uploaded file name. */
	source?: string;
	/** Install jobs: activate after installing. */
	activate?: boolean;
	/** Plugins and core jobs: update-locked items may get major updates. */
	allow_major?: boolean;
	/**
	 * One entry per plugin (one "WordPress" entry for core jobs, one entry
	 * for the installed plugin for install jobs) once finished.
	 */
	results: UpdateResult[];
	/** Core jobs: the refreshed core state once finished. */
	core?: SiteCore;
	error?: string;
	created_by: string;
	created_at: string;
	started_at?: string;
	finished_at?: string;
}

export interface TwoFactorSetupResponse {
	secret: string;
	uri: string;
}

export interface UserProfile {
	username: string;
	displayName: string;
	level: UserLevel;
	has2FA: boolean;
}

export type TaskType = "one-time" | "repeating" | "dynamic";
export type TaskStatus =
	| "pending"
	| "in_progress"
	| "completed"
	| "skipped"
	| "on_hold"
	| "blocked"
	| "overdue";
export type TaskPriority = "low" | "medium" | "high";

export type DashboardWidgetType =
	| "stats"
	| "tasks"
	| "vulnerabilities"
	| "renewals";

export interface DashboardSettings {
	layout: DashboardWidgetType[];
}

export interface VulnSettings {
	defaultAssignee: string;
}

export interface Task {
	id: number;
	type: TaskType;
	status: TaskStatus;
	priority: TaskPriority;
	title: string;
	description: string | null;
	/** UUID; null or omitted when unset. */
	site_id?: string | null;
	/** UUID; null or omitted when unset. */
	server_id?: string | null;
	organization_id: number | null;
	plugin_slug: string | null;
	assigned_to: string | null;
	interval: string | null;
	metadata: string | null;
	due_date: string | null;
	reminder_date: string | null;
	created_at: string;
	updated_at: string;
	created_by: string;
	completed_at: string | null;
	completed_by: string | null;
}

export interface CreateTaskPayload {
	type?: TaskType;
	status?: TaskStatus;
	priority?: TaskPriority;
	title: string;
	description?: string;
	site_id?: string | null;
	server_id?: string | null;
	organization_id?: number | null;
	plugin_slug?: string | null;
	assigned_to?: string | null;
	interval?: string | null;
	due_date?: string | null;
	reminder_date?: string | null;
}

export type UpdateTaskPayload = Partial<CreateTaskPayload>;

export interface TaskFilters {
	status?: TaskStatus | "";
	priority?: TaskPriority | "";
	assigned_to?: string;
	site_id?: string;
	organization_id?: number;
	server_id?: string;
	search?: string;
}

export interface AgentToken {
	id: number;
	server_id: string;
	server_name: string;
	token_prefix: string;
	description: string | null;
	revoked: boolean;
	last_seen_at: string | null;
	agent_version: string | null;
	created_at: string;
	created_by: string;
}

export interface CreateAgentTokenPayload {
	server_id: string;
	server_name: string;
	description?: string;
}

// Returned only once, at creation time, in addition to the normal AgentToken fields.
export interface CreatedAgentToken extends AgentToken {
	token: string;
}

export interface TrafficTopEntry {
	key: string;
	count: number;
}

export interface SiteTrafficPeriod {
	period_start: string;
	requests_total: number;
	requests_human: number;
	requests_bot: number;
	// Note: for period=daily/monthly this is the SUM of each finer-grained
	// period's unique-visitor count, which over-counts visitors active
	// across multiple hours/days within the same day/month (true
	// daily/monthly-distinct isn't tracked server-side to avoid retaining
	// raw IPs). Treat as an approximation, not an exact count.
	unique_visitors: number;
	top_pages: TrafficTopEntry[];
	top_referrers: TrafficTopEntry[];
	// Raw connection count per HTTP status code (e.g. "200", "404"),
	// independent of top_pages — every request counts here, including ones
	// excluded from top_pages (non-200 or a WordPress system path).
	status_codes: TrafficTopEntry[];
}

export type ReportColumnType = "text" | "number" | "currency" | "date";

export interface ReportColumn {
	key: string;
	label: string;
	type: ReportColumnType;
}

export type ReportParamType = "daterange" | "enddate";

export interface ReportParamDef {
	key: string;
	type: ReportParamType;
	label: string;
	required: boolean;
	default?: string;
}

export interface ReportMeta {
	id: string;
	name: string;
	description: string;
	params: ReportParamDef[];
}

export interface ReportResult {
	columns: ReportColumn[];
	rows: Record<string, string | number | null>[];
}

export type IncidentStatus = "open" | "acknowledged" | "resolved" | "closed";

export interface Incident {
	id: number;
	domain: string;
	status: IncidentStatus;
	error_message: string;
	error_code: number;
	down_since: string;
	acknowledged_by?: string | null;
	acknowledged_at?: string | null;
	resolved_by?: string | null;
	resolved_at?: string | null;
	pd_triggered: boolean;
	created_at: string;
	updated_at: string;
}

export interface IncidentsResponse {
	incidents: Incident[];
	total: number;
	active_count: number;
}

// ---------------------------------------------------------------------------
// Managed (host-agnostic) sites and servers from inventory.db
// ---------------------------------------------------------------------------

export type ManagedConnectionType = "ssh" | "agent" | "none";
export type ManagedSiteStatus = "active" | "paused" | "archived";

export interface ManagedServer {
	id: string;
	provider: string;
	provider_server_id?: string;
	name: string;
	is_logical: boolean;
	ip_address?: string;
	ssh_port: number;
	created_at?: string;
	updated_at?: string;
}

export interface ManagedSite {
	id: string;
	server_id?: string | null;
	server_name?: string;
	provider: string;
	provider_site_id?: string;
	domain: string;
	environment: SiteEnvironment;
	is_wordpress: boolean;
	php_version?: string;
	connection_type: ManagedConnectionType;
	ssh_host: string;
	ssh_port: number;
	ssh_user: string;
	site_path: string;
	can_wp_cli: boolean;
	has_agent: boolean;
	has_monitoring: boolean;
	status: ManagedSiteStatus;
	created_at?: string;
	updated_at?: string;
}

export interface ManagedSitePayload {
	domain?: string;
	/** Empty string unassigns the site from its server. */
	server_id?: string;
	provider?: string;
	environment?: SiteEnvironment;
	is_wordpress?: boolean;
	php_version?: string;
	connection_type?: ManagedConnectionType;
	ssh_host?: string;
	ssh_port?: number;
	ssh_user?: string;
	site_path?: string;
	can_wp_cli?: boolean;
	has_agent?: boolean;
	has_monitoring?: boolean;
	status?: ManagedSiteStatus;
}

export interface ManagedServerPayload {
	name: string;
	provider?: string;
	is_logical: boolean;
	ip_address?: string;
	ssh_port?: number;
}

export interface TestConnectionResult {
	success: boolean;
	wp_version?: string;
	message?: string;
	error?: string;
	output?: string;
	stderr?: string;
}

export interface SpinupWPSyncResult {
	success: boolean;
	servers: number;
	sites: number;
	message: string;
}
