<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useMonitorStore } from "../stores/monitor";
import { useOrganizationStore } from "../stores/organization";
import { useAuthStore } from "../stores/auth";
import { useIgnoreStore } from "../stores/ignore";
import { useToastStore } from "../stores/toast";
import type { Organization, Contact, SiteEnvironment } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import InfoCard, { type InfoItem } from "../components/InfoCard.vue";
import MonitorHistoryCard from "../components/MonitorHistoryCard.vue";
import SiteTrafficCard from "../components/SiteTrafficCard.vue";
import PluginUpdateModal from "../components/PluginUpdateModal.vue";
import { useConfirm } from "../composables/useConfirm";
import NotesWidget from "../components/NotesWidget.vue";
import { formatBytes } from "../utils/format";
import type { SiteUpdateLedgerEntry } from "../types";
import { BASE_URL } from "../utils/api";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const dataStore = useDataStore();
const monitorStore = useMonitorStore();
const organizationStore = useOrganizationStore();
const authStore = useAuthStore();
const ignoreStore = useIgnoreStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const siteId = parseInt(props.id, 10);
if (isNaN(siteId)) router.replace({ name: "sites" });
const organization = ref<Organization | null>(null);
const contacts = ref<Contact[]>([]);
const site = computed(() => dataStore.getSiteById(siteId));

// Update Ledger state
const ledgerEntries = ref<SiteUpdateLedgerEntry[]>([]);
const showAddLedgerModal = ref(false);
const newLedgerType = ref<"core" | "plugin" | "theme">("plugin");
const newLedgerStatus = ref<"full" | "partial" | "failed">("full");
const newLedgerDetails = ref("");
const isSavingLedger = ref(false);

const fetchLedger = async () => {
	try {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/update-ledger`, {
			headers: authStore.authHeader,
		});
		if (res.ok) {
			const data = await res.json();
			ledgerEntries.value = data || [];
		}
	} catch (e) {
		console.error("Failed to fetch update ledger", e);
	}
};

const saveManualLedgerEntry = async () => {
	isSavingLedger.value = true;
	try {
		let dataJSON = "";
		if (newLedgerDetails.value.trim()) {
			dataJSON = JSON.stringify({ note: newLedgerDetails.value.trim() });
		}

		const res = await fetch(`${BASE_URL}/sites/${siteId}/update-ledger`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({
				update_type: newLedgerType.value,
				status: newLedgerStatus.value,
				data_json: dataJSON,
			}),
		});

		if (res.ok) {
			toast.addToast("Manual update logged successfully.", "success");
			showAddLedgerModal.value = false;
			newLedgerDetails.value = "";
			newLedgerType.value = "plugin";
			newLedgerStatus.value = "full";
			await fetchLedger();
			await dataStore.refreshData();
		} else {
			toast.addToast("Failed to log manual update.", "error");
		}
	} catch (e: any) {
		toast.addToast("Error: " + e.message, "error");
	} finally {
		isSavingLedger.value = false;
	}
};

function formatLedgerDetails(entry: SiteUpdateLedgerEntry) {
	if (!entry.data_json) return "—";
	try {
		const data = JSON.parse(entry.data_json);
		if (data.summary) {
			let txt = data.summary;
			if (data.updates && data.updates.length > 0) {
				const pluginsList = data.updates
					.map(
						(u: any) =>
							`${u.plugin} (${u.status === "success" ? "✓" : "✗"})`,
					)
					.join(", ");
				txt += `\nPlugins: ${pluginsList}`;
			}
			return txt;
		}
		if (data.plugin) {
			let txt = `Plugin: ${data.plugin}`;
			if (data.old_version || data.new_version) {
				txt += ` (${data.old_version || "?"} → ${data.new_version || "?"})`;
			}
			if (data.error) {
				txt += ` [Error: ${data.error}]`;
			}
			return txt;
		}
		if (data.note) {
			return data.note;
		}
		return JSON.stringify(data, null, 2);
	} catch (e) {
		return entry.data_json;
	}
}

function formatDate(d: string | Date | null) {
	if (!d) return "—";
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "short",
		timeStyle: "short",
	});
}

const expandedLedgerEntries = ref<Set<number>>(new Set());

const toggleExpandLedgerEntry = (id: number) => {
	if (expandedLedgerEntries.value.has(id)) {
		expandedLedgerEntries.value.delete(id);
	} else {
		expandedLedgerEntries.value.add(id);
	}
};

const isLedgerEntryExpanded = (id: number) => {
	return expandedLedgerEntries.value.has(id);
};

const getFirstLine = (entry: SiteUpdateLedgerEntry) => {
	const details = formatLedgerDetails(entry);
	return details.split("\n")[0];
};

const hasMultipleLines = (entry: SiteUpdateLedgerEntry) => {
	const details = formatLedgerDetails(entry);
	return details.includes("\n") || details.length > 55;
};

// Fetch initial data
onMounted(async () => {
	await Promise.all([
		dataStore.initData(),
		ignoreStore.fetchIgnoreEntries(),
		fetchLedger(),
	]);
});

// Watch for domain changes to fetch monitor status
watch(
	() => site.value?.domain,
	(domain) => {
		if (domain) {
			monitorStore.fetchStatus(domain);
		}
	},
	{ immediate: true },
);

// Watch for site ID to fetch linked organization
watch(
	() => site.value?.id,
	async (id) => {
		if (id) {
			try {
				organization.value =
					await organizationStore.getOrganizationForSite(id);
				dataStore.setSiteOrganizationLink(id, organization.value?.id);
				if (organization.value) {
					contacts.value =
						await organizationStore.fetchOrganizationContacts(
							organization.value.id,
						);
				}
			} catch (e) {
				console.error("Failed to fetch organization for site", e);
			}
		}
	},
	{ immediate: true },
);

const server = computed(() =>
	site.value ? dataStore.getServerById(site.value.server_id) : null,
);

const environmentDraft = ref<SiteEnvironment | "">("");
watch(
	() => site.value?.environment,
	(environment) => {
		environmentDraft.value = environment ?? "";
	},
	{ immediate: true },
);

const onEnvironmentChange = async () => {
	if (!site.value) return;
	const previous = site.value.environment ?? "";
	try {
		await dataStore.setSiteEnvironment(
			site.value.id,
			environmentDraft.value,
		);
		toast.addToast("Environment updated.", "success");
	} catch (e: any) {
		environmentDraft.value = previous;
		toast.addToast("Failed to update environment: " + e.message, "error");
	}
};

const history = computed(() =>
	site.value ? monitorStore.historyByDomain.get(site.value.domain) || [] : [],
);

const sitePlugins = computed(() => {
	const enrichedSite = dataStore.enrichedSites.find((s) => s.id === siteId);
	if (!enrichedSite) return [];

	return enrichedSite.plugins.map((plugin) => {
		// Filter relevant vulnerabilities for this plugin on this site
		const vulns = enrichedSite.vulnerabilities.filter(
			(v) => v.slug === plugin.name,
		);

		return {
			...plugin,
			vulnerabilities: vulns,
			suppressed: vulns.length > 0 && vulns.every((v) => v.suppressed),
		};
	});
});

const siteInfoItems = computed(() => {
	if (!site.value) return [];
	const items: InfoItem[] = [
		{ label: "Site ID", value: site.value.id },
		{
			label: "Domain",
			value: site.value.domain,
			copyable: true,
			isLink: true,
			href: site.value.domain.startsWith("http")
				? site.value.domain
				: `https://${site.value.domain}`,
		},
		{ label: "PHP Version", value: site.value.php_version },
		{ label: "WordPress", value: site.value.is_wordpress ? "Yes" : "No" },
	];

	if (site.value.site_user) {
		items.push({
			label: "System User",
			value: site.value.site_user,
			copyable: true,
		});
	}

	if (site.value.wp_flags) {
		items.push({
			label: "Multisite",
			value: site.value.wp_flags.is_multisite ? "Yes" : "No",
		});
		items.push({
			label: "File Mods Allowed",
			value: !site.value.wp_flags.disallow_file_mods ? "Yes" : "No",
		});
	}

	if (site.value.database?.table_prefix) {
		items.push({
			label: "Table Prefix",
			value: site.value.database.table_prefix,
			copyable: true,
		});
	}

	if (site.value.disk_usage) {
		items.push({
			label: "Disk Usage",
			value: formatBytes(site.value.disk_usage.bytes_used),
		});
	}

	items.push({ label: "Status", value: site.value.status });
	return items;
});

const serverInfoItems = computed(() => {
	if (!server.value) return [];
	const items = [
		{ label: "Server Name", value: server.value.name, copyable: true },
		{ label: "IP Address", value: server.value.ip_address, copyable: true },
		{ label: "Ubuntu", value: server.value.ubuntu_version },
		{ label: "Provider", value: server.value.provider_name },
	];

	if (server.value.disk_space && server.value.disk_space.total > 0) {
		const used = server.value.disk_space.used;
		const total = server.value.disk_space.total;
		const percent = Math.round((used / total) * 100);
		items.push({
			label: "Disk Space",
			value: `${formatBytes(used)} / ${formatBytes(total)} (${percent}% used)`,
		});
	}

	return items;
});

const goBack = () => {
	router.push({ name: "sites" });
};

const goToPlugin = (name: string) => {
	router.push({
		name: "plugin-detail",
		params: { name },
	});
};

const goToOrganization = () => {
	if (organization.value) {
		router.push({
			name: "organization-detail",
			params: { id: organization.value.id.toString() },
		});
	}
};

const showPluginUpdateModal = ref(false);

const showLinkModal = ref(false);
const organizationSearchQuery = ref("");
const searchResults = ref<Organization[]>([]);
const isSearching = ref(false);
let searchTimeout: number | null = null;
let abortController: AbortController | null = null;

const handleSearch = () => {
	if (searchTimeout) {
		clearTimeout(searchTimeout);
	}

	if (organizationSearchQuery.value.length < 2) {
		searchResults.value = [];
		isSearching.value = false;
		if (abortController) {
			abortController.abort();
			abortController = null;
		}
		return;
	}

	searchTimeout = window.setTimeout(async () => {
		if (abortController) {
			abortController.abort();
		}
		abortController = new AbortController();
		isSearching.value = true;

		try {
			await organizationStore.fetchOrganizations(
				organizationSearchQuery.value,
				abortController.signal,
			);
			searchResults.value = organizationStore.organizations;
		} catch (e: any) {
			if (e.name !== "AbortError") {
				console.error("Search failed", e);
			}
		} finally {
			if (!abortController?.signal.aborted) {
				isSearching.value = false;
			}
		}
	}, 300);
};

const linkOrganization = async (organizationId: number) => {
	try {
		await organizationStore.linkSiteToOrganization(siteId, organizationId);
		dataStore.setSiteOrganizationLink(siteId, organizationId);
		await dataStore.refreshData();
		organization.value =
			await organizationStore.getOrganizationForSite(siteId);
		if (organization.value) {
			contacts.value = await organizationStore.fetchOrganizationContacts(
				organization.value.id,
			);
		}
		showLinkModal.value = false;
	} catch (e: any) {
		toast.addToast("Failed to link organization: " + e.message, "error");
	}
};

const unlinkOrganization = async () => {
	if (!(await confirm("Are you sure you want to unlink this organization?")))
		return;
	try {
		await organizationStore.unlinkSite(siteId);
		dataStore.setSiteOrganizationLink(siteId, undefined);
		await dataStore.refreshData();
		organization.value = null;
		contacts.value = [];
	} catch (e: any) {
		toast.addToast("Failed to unlink organization: " + e.message, "error");
	}
};
</script>

<template>
	<div class="view-container">
		<ViewHeader
			title="Site Details"
			:back-button="{ text: 'Back to Sites', onClick: goBack }"
		/>

		<main v-if="site" class="content mt-4">
			<div class="flex-row gap-3 mb-4">
				<label for="site-environment" class="font-medium"
					>Environment</label
				>
				<select
					id="site-environment"
					v-model="environmentDraft"
					:disabled="!authStore.canEdit"
					@change="onEnvironmentChange"
				>
					<option value="">Unclassified</option>
					<option value="production">Production</option>
					<option value="staging">Staging</option>
					<option value="development">Development</option>
					<option value="archived">Archived</option>
				</select>
			</div>

			<div class="grid-2-cols">
				<InfoCard title="Site Information" :items="siteInfoItems" />
				<InfoCard
					v-if="server"
					title="Server Information"
					:items="serverInfoItems"
				/>
			</div>

			<NotesWidget parent-type="Site" :parent-id="siteId" />

			<section class="card mt-4">
				<div class="card-header">
					<h2>Organization Information</h2>
					<div class="flex-row gap-3">
						<button
							v-if="organization && authStore.canEdit"
							class="btn btn-text danger"
							@click="unlinkOrganization"
						>
							Unlink
						</button>
						<button
							v-if="organization"
							class="btn btn-outline btn-sm"
							@click="goToOrganization"
						>
							View Organization
						</button>
						<button
							v-if="!organization && authStore.canEdit"
							class="btn btn-primary btn-sm"
							@click="showLinkModal = true"
						>
							Link Organization
						</button>
					</div>
				</div>

				<div v-if="organization">
					<div class="info-grid">
						<div class="info-item">
							<span class="label">Name</span>
							<span class="value">{{ organization.name }}</span>
						</div>
						<div v-if="organization.vat_number" class="info-item">
							<span class="label">VAT Number</span>
							<span class="value">{{
								organization.vat_number
							}}</span>
						</div>
					</div>

					<div v-if="contacts.length > 0" class="mt-4">
						<h3 class="sub-text font-medium mb-4">Contacts</h3>
						<div class="table-container">
							<table class="data-table">
								<thead>
									<tr>
										<th>Name</th>
										<th>Type</th>
										<th class="hide-mobile">Email</th>
									</tr>
								</thead>
								<tbody>
									<tr
										v-for="contact in contacts"
										:key="contact.id"
									>
										<td class="font-medium">
											{{ contact.name }}
										</td>
										<td>
											<span
												:class="[
													'status-badge',
													'badge-sm',
													contact.type.toLowerCase(),
												]"
											>
												{{ contact.type }}
											</span>
										</td>
										<td class="hide-mobile text-muted">
											{{ contact.email || "—" }}
										</td>
									</tr>
								</tbody>
							</table>
						</div>
					</div>
				</div>
				<div v-else class="empty-state">
					No organization linked to this site.
				</div>
			</section>

			<div class="mt-4">
				<MonitorHistoryCard
					:history="history"
					:domain="site.domain"
					:site-id="site.id"
					:server-id="site.server_id"
				/>
			</div>

			<SiteTrafficCard :site-id="site.id" />

			<section class="card mt-4">
				<div class="card-header">
					<h2>Installed Plugins ({{ sitePlugins.length }})</h2>
					<button
						v-if="authStore.canExecute"
						class="btn btn-primary btn-sm"
						@click="showPluginUpdateModal = true"
					>
						Check Updates
					</button>
				</div>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Plugin Name</th>
								<th class="hide-mobile">Version</th>
								<th class="hide-mobile">Status</th>
								<th>Vulns</th>
							</tr>
						</thead>
						<tbody>
							<tr v-if="sitePlugins.length === 0">
								<td colspan="4" class="empty-state">
									No plugins found.
								</td>
							</tr>
							<tr
								v-for="plugin in sitePlugins"
								:key="plugin.name"
								class="clickable-row"
								@click="goToPlugin(plugin.name)"
							>
								<td class="font-medium">{{ plugin.name }}</td>
								<td class="hide-mobile text-muted">
									{{ plugin.version }}
								</td>
								<td class="hide-mobile">
									<span
										:class="[
											'status-badge',
											'badge-sm',
											plugin.status.toLowerCase(),
										]"
									>
										{{ plugin.status }}
									</span>
								</td>
								<td>
									<span
										v-if="plugin.vulnerabilities.length > 0"
										class="status-badge badge-sm"
										:class="
											plugin.suppressed
												? 'warning'
												: 'error'
										"
										:title="`${plugin.vulnerabilities.length} vulnerabilities detected${plugin.suppressed ? ' (Suppressed)' : ''}`"
									>
										{{ plugin.vulnerabilities.length }}
									</span>
									<span v-else class="text-muted">—</span>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</section>

			<section class="card mt-4">
				<div class="card-header">
					<h2>Update Ledger</h2>
					<button
						v-if="authStore.canEdit"
						class="btn btn-primary btn-sm"
						@click="showAddLedgerModal = true"
					>
						Log Manual Update
					</button>
				</div>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Type</th>
								<th>Status</th>
								<th>Details</th>
								<th>User</th>
								<th>Timestamp</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-if="
									!ledgerEntries || ledgerEntries.length === 0
								"
							>
								<td colspan="5" class="empty-state">
									No update ledger entries recorded.
								</td>
							</tr>
							<tr
								v-for="entry in ledgerEntries || []"
								:key="entry.id"
							>
								<td>
									<span class="status-badge badge-sm info">
										{{ entry.update_type.toUpperCase() }}
									</span>
								</td>
								<td>
									<span
										:class="[
											'status-badge',
											'badge-sm',
											entry.status === 'full'
												? 'active'
												: entry.status === 'partial'
													? 'warning'
													: 'error',
										]"
									>
										{{ entry.status }}
									</span>
								</td>
								<td
									class="font-xs text-muted"
									style="max-width: 350px"
								>
									<div
										style="
											display: flex;
											flex-direction: column;
										"
									>
										<div
											:style="
												isLedgerEntryExpanded(entry.id)
													? 'white-space: pre-wrap; word-break: break-word;'
													: 'white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%;'
											"
										>
											{{
												isLedgerEntryExpanded(entry.id)
													? formatLedgerDetails(entry)
													: getFirstLine(entry)
											}}
										</div>
										<button
											v-if="hasMultipleLines(entry)"
											class="btn btn-text btn-sm mt-1 p-0 text-left"
											style="
												font-size: 11px;
												height: auto;
												min-height: 0;
												align-self: flex-start;
											"
											@click.stop="
												toggleExpandLedgerEntry(
													entry.id,
												)
											"
										>
											{{
												isLedgerEntryExpanded(entry.id)
													? "Show Less"
													: "Show More"
											}}
										</button>
									</div>
								</td>
								<td class="font-medium">
									{{ entry.updated_by }}
								</td>
								<td class="text-muted">
									{{ formatDate(entry.updated_at) }}
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</section>
		</main>
		<main v-else class="content mt-4">
			<div class="card">
				<LoadingSpinner
					v-if="dataStore.isLoading"
					message="Loading site details..."
				/>
				<div v-else class="empty-state">
					<p>Site not found.</p>
					<button class="back-btn mt-4" @click="goBack">
						Go back to sites
					</button>
				</div>
			</div>
		</main>

		<!-- Plugin Update Modal -->
		<PluginUpdateModal
			:visible="showPluginUpdateModal"
			:site-id="siteId"
			@close="
				() => {
					showPluginUpdateModal = false;
					fetchLedger();
				}
			"
		/>

		<!-- Link Organization Modal -->
		<div
			v-if="showLinkModal"
			class="modal-overlay"
			@click.self="showLinkModal = false"
		>
			<div class="modal-content card">
				<h2>Link Organization to Site</h2>
				<div class="content">
					<div class="form-group">
						<label for="org-search">Search Organization</label>
						<input
							id="org-search"
							v-model="organizationSearchQuery"
							type="text"
							placeholder="Type name or VAT..."
							@input="handleSearch"
						/>
					</div>

					<div
						v-if="searchResults.length > 0"
						class="table-container"
						style="max-height: 300px"
					>
						<table class="data-table">
							<tbody>
								<tr
									v-for="res in searchResults"
									:key="res.id"
									class="clickable-row"
									@click="linkOrganization(res.id)"
								>
									<td>
										<div class="font-medium">
											{{ res.name }}
										</div>
										<div
											v-if="res.vat_number"
											class="sub-text"
										>
											{{ res.vat_number }}
										</div>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
					<div
						v-else-if="
							organizationSearchQuery.length >= 2 && !isSearching
						"
						class="empty-state"
					>
						No organizations found.
					</div>
					<div v-else-if="isSearching" class="empty-state">
						Searching...
					</div>

					<div class="form-actions">
						<button
							class="btn btn-outline"
							@click="showLinkModal = false"
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>

		<!-- Add Ledger Modal -->
		<div
			v-if="showAddLedgerModal"
			class="modal-overlay"
			@click.self="showAddLedgerModal = false"
		>
			<div class="modal-content card" style="max-width: 450px">
				<h2>Log Manual Update</h2>
				<div class="content mt-4">
					<div class="form-group mb-4">
						<label for="ledger-type">Update Type</label>
						<select id="ledger-type" v-model="newLedgerType">
							<option value="plugin">Plugin</option>
							<option value="core">WordPress Core</option>
							<option value="theme">Theme</option>
						</select>
					</div>

					<div class="form-group mb-4">
						<label for="ledger-status">Status</label>
						<select id="ledger-status" v-model="newLedgerStatus">
							<option value="full">Full Success</option>
							<option value="partial">Partial</option>
							<option value="failed">Failed</option>
						</select>
					</div>

					<div class="form-group mb-4">
						<label for="ledger-details"
							>Details / Notes (Optional)</label
						>
						<textarea
							id="ledger-details"
							v-model="newLedgerDetails"
							placeholder="E.g., Updated WooCommerce from WP Admin"
							rows="3"
						></textarea>
					</div>

					<div class="form-actions mt-4">
						<button
							class="btn btn-outline"
							@click="showAddLedgerModal = false"
						>
							Cancel
						</button>
						<button
							class="btn btn-primary"
							:disabled="isSavingLedger"
							@click="saveManualLedgerEntry"
						>
							{{ isSavingLedger ? "Saving..." : "Save Log" }}
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>
</template>
