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

export interface Server {
	id: number;
	name: string;
	provider_name: string;
	ubuntu_version: string;
	ip_address: string;
	ssh_port: number;
	timezone: string;
	region: string;
	size: string;
	disk_space: DiskSpace;
	database: ServerDatabase;
	ssh_publickey: string;
	git_publickey: string;
	connection_status: string;
	reboot_required: boolean;
	upgrade_required: boolean;
	install_notes: string;
	created_at: string;
	status: string;
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

export interface SiteWpFlags {
	is_multisite: boolean;
	disallow_file_mods: boolean;
	updated_at: string;
}

export interface Site {
	id: number;
	server_id: number;
	organization_id?: number;
	domain: string;
	additional_domains: AdditionalDomain[];
	site_user: string;
	user_auth: string;
	php_version: string;
	public_folder: string;
	is_wordpress: boolean;
	page_cache: {
		enabled: boolean;
	};
	https: {
		enabled: boolean;
		certificate_expires: string | null;
		certificate_renews: string | null;
	};
	nginx: {
		uploads_directory_protected: boolean;
		xmlrpc_protected: boolean;
		subdirectory_rewrite_in_place: boolean;
	};
	database: SiteDatabase;
	backups: Backups;
	wp_core_update: boolean | number;
	wp_theme_updates: boolean | number;
	wp_plugin_updates: boolean | number;
	basic_auth: {
		enabled: boolean;
		username: string;
	};
	created_at: string;
	created_by: string;
	updated_at: string;
	updated_by: string;
	status: string;
	environment?: SiteEnvironment;
	disk_usage?: SiteDiskUsage;
	wp_flags?: SiteWpFlags;
	last_update?: SiteUpdateLedgerEntry;
}

export interface Plugin {
	site_id: number;
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
	site_id: number;
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

export interface SiteUpdateLedgerEntry {
	id: number;
	site_id: number;
	update_type: "core" | "plugin" | "theme";
	status: "full" | "partial" | "failed";
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
	negated_site_ids: number[] | null;
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
	negated_site_ids?: number[];
	use_for_monitor?: boolean;
	use_for_vuln?: boolean;
}

export type UpdateIgnorePayload = Partial<CreateIgnorePayload>;

export interface EnrichedSite extends Site {
	server: string;
	plugins: Plugin[];
	vulnerabilities: EnrichedVulnerability[];
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
	site_id: number | null;
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

export interface PluginUpdateResult {
	name: string;
	old_version: string;
	new_version: string;
	status: string;
	error?: string;
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

export interface Task {
	id: number;
	type: TaskType;
	status: TaskStatus;
	priority: TaskPriority;
	title: string;
	description: string | null;
	site_id: number | null;
	server_id: number | null;
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
	site_id?: number | null;
	server_id?: number | null;
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
	site_id?: number;
	organization_id?: number;
	server_id?: number;
	search?: string;
}

export interface AgentToken {
	id: number;
	server_id: number;
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
	server_id: number;
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
