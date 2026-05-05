<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useMonitorStore } from "../stores/monitor";
import { useOrganizationStore } from "../stores/organization";
import { useAuthStore } from "../stores/auth";
import { useToastStore } from "../stores/toast";
import type { Organization, Contact } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import InfoCard from "../components/InfoCard.vue";
import MonitorHistoryCard from "../components/MonitorHistoryCard.vue";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const dataStore = useDataStore();
const monitorStore = useMonitorStore();
const organizationStore = useOrganizationStore();
const authStore = useAuthStore();
const toast = useToastStore();

const siteId = parseInt(props.id, 10);
const organization = ref<Organization | null>(null);
const contacts = ref<Contact[]>([]);
const site = computed(() => dataStore.getSiteById(siteId));

// Fetch initial data
onMounted(async () => {
	await dataStore.initData();
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
				organization.value = await organizationStore.getOrganizationForSite(id);
				dataStore.setSiteOrganizationLink(id, organization.value?.id);
				if (organization.value) {
					contacts.value = await organizationStore.fetchOrganizationContacts(
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

const history = computed(() =>
	site.value ? monitorStore.historyByDomain.get(site.value.domain) || [] : [],
);

const sitePlugins = computed(() => {
	const siteVulns = dataStore.vulnerabilitiesBySiteId.get(siteId) || [];
	return dataStore.getPluginsBySiteId(siteId).map((plugin) => {
		const vulns = siteVulns.filter((v) => v.slug === plugin.name);
		return {
			...plugin,
			vulnerabilities: vulns,
		};
	});
});

const siteInfoItems = computed(() => {
	if (!site.value) return [];
	return [
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
		{ label: "Public Folder", value: site.value.public_folder },
		{ label: "WordPress", value: site.value.is_wordpress ? "Yes" : "No" },
		{ label: "Status", value: site.value.status },
	];
});

const serverInfoItems = computed(() => {
	if (!server.value) return [];
	return [
		{ label: "Server Name", value: server.value.name, copyable: true },
		{ label: "IP Address", value: server.value.ip_address, copyable: true },
		{ label: "Ubuntu", value: server.value.ubuntu_version },
		{ label: "Provider", value: server.value.provider_name },
	];
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
		organization.value = await organizationStore.getOrganizationForSite(siteId);
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
	if (!confirm("Are you sure you want to unlink this organization?")) return;
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

		<main class="content" v-if="site">
			<div class="grid-2-cols">
				<InfoCard title="Site Information" :items="siteInfoItems" />
				<InfoCard
					v-if="server"
					title="Server Information"
					:items="serverInfoItems"
				/>
			</div>

			<section class="card organization-section">
				<div class="organization-header">
					<h2 class="organization-title">Organization Information</h2>
					<div class="header-actions">
						<button
							v-if="organization && authStore.canEdit"
							class="text-btn unlink-btn"
							@click="unlinkOrganization"
						>
							Unlink
						</button>
						<button
							v-if="organization"
							class="back-btn view-org-btn"
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
							<span class="label">Name:</span>
							<span class="value">{{ organization.name }}</span>
						</div>
						<div class="info-item" v-if="organization.vat_number">
							<span class="label">VAT Number:</span>
							<span class="value">{{ organization.vat_number }}</span>
						</div>
					</div>

					<div class="contacts-preview" v-if="contacts.length > 0">
						<h3>Contacts</h3>
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
									<tr v-for="contact in contacts" :key="contact.id">
										<td>{{ contact.name }}</td>
										<td>
											<span
												:class="['status-badge', contact.type.toLowerCase()]"
											>
												{{ contact.type }}
											</span>
										</td>
										<td class="hide-mobile">{{ contact.email || "—" }}</td>
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

			<MonitorHistoryCard :history="history" :domain="site.domain" />

			<section class="card">
				<h2>Installed Plugins ({{ sitePlugins.length }})</h2>
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
								<td colspan="4" class="empty-state">No plugins found.</td>
							</tr>
							<tr
								v-for="plugin in sitePlugins"
								:key="plugin.name"
								class="clickable-row"
								@click="goToPlugin(plugin.name)"
							>
								<td>{{ plugin.name }}</td>
								<td class="hide-mobile">{{ plugin.version }}</td>
								<td class="hide-mobile">
									<span :class="['status-badge', plugin.status.toLowerCase()]">
										{{ plugin.status }}
									</span>
								</td>
								<td>
									<span
										v-if="plugin.vulnerabilities.length > 0"
										class="status-badge error"
										:title="`${plugin.vulnerabilities.length} vulnerabilities detected`"
									>
										{{ plugin.vulnerabilities.length }}
									</span>
									<span v-else class="empty-dash">—</span>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</section>
		</main>
		<main class="content" v-else>
			<div class="card">
				<LoadingSpinner
					v-if="dataStore.isLoading"
					message="Loading site details..."
				/>
				<div v-else class="empty-state">
					<p>Site not found.</p>
					<button class="back-btn not-found-back-btn" @click="goBack">
						Go back to sites
					</button>
				</div>
			</div>
		</main>

		<!-- Link Organization Modal -->
		<div
			v-if="showLinkModal"
			class="modal-overlay"
			@click.self="showLinkModal = false"
		>
			<div class="modal-content card">
				<h2>Link Organization to Site</h2>
				<div class="form-layout">
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

					<div class="search-results-list" v-if="searchResults.length > 0">
						<div
							v-for="res in searchResults"
							:key="res.id"
							class="search-result-item"
							@click="linkOrganization(res.id)"
						>
							<div class="res-name">{{ res.name }}</div>
							<div class="res-vat" v-if="res.vat_number">
								{{ res.vat_number }}
							</div>
						</div>
					</div>
					<div
						v-else-if="organizationSearchQuery.length >= 2 && !isSearching"
						class="empty-state"
					>
						No organizations found.
					</div>
					<div v-else-if="isSearching" class="empty-state">Searching...</div>

					<div class="form-actions">
						<button class="back-btn" @click="showLinkModal = false">
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>
</template>

<style scoped>
.grid-2-cols {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 24px;
	margin-bottom: 24px;
}

@media (max-width: 1024px) {
	.grid-2-cols {
		grid-template-columns: 1fr;
	}
}

.contacts-preview {
	margin-top: 24px;
}

.contacts-preview h3 {
	font-size: 0.95rem;
	margin-bottom: 12px;
	color: var(--text-muted);
}

.organization-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 16px;
	border-bottom: 1px solid var(--border-color);
	padding-bottom: 8px;
	flex-wrap: wrap;
	gap: 12px;
}

.organization-title {
	margin: 0;
	border: none;
}

.unlink-btn {
	color: #ef4444;
}

.view-org-btn {
	padding: 4px 12px;
	font-size: 13px;
}

.empty-dash {
	color: #999;
}

.not-found-back-btn {
	margin-top: 16px;
}

.header-actions {
	display: flex;
	gap: 12px;
	align-items: center;
}

.text-btn {
	background: none;
	border: none;
	font-weight: 600;
	cursor: pointer;
	padding: 4px 8px;
	font-size: 0.9rem;
	transition: opacity 0.2s;
}

.text-btn:hover {
	opacity: 0.8;
}

.info-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
	gap: 16px;
}

.info-item {
	display: flex;
	flex-direction: column;
}

.label {
	font-size: 0.85rem;
	color: var(--text-muted);
	font-weight: 600;
	margin-bottom: 4px;
}

.value {
	font-weight: 500;
}

/* Modal styles */
.modal-overlay {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background: rgba(0, 0, 0, 0.6);
	backdrop-filter: blur(2px);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 1000;
}

.modal-content {
	width: 100%;
	max-width: 500px;
	padding: 24px;
	box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);

	@media (max-width: 640px) {
		width: 95%;
		padding: 20px 16px;
		max-height: 90vh;
		overflow-y: auto;
	}
}

.modal-content h2 {
	margin-top: 0;
	margin-bottom: 20px;
	font-size: 1.25rem;
	border-bottom: none;
	padding-bottom: 0;
}

.search-results-list {
	margin-top: 12px;
	border: 1px solid var(--border-color);
	border-radius: 6px;
	max-height: 250px;
	overflow-y: auto;
}

.search-result-item {
	padding: 10px 16px;
	cursor: pointer;
	transition: background-color 0.2s;
	border-bottom: 1px solid var(--border-color);
}

.search-result-item:last-child {
	border-bottom: none;
}

.search-result-item:hover {
	background-color: var(--bg-hover);
}

.res-name {
	font-weight: 600;
	color: var(--text-main);
}

.res-vat {
	font-size: 0.85rem;
	color: var(--text-muted);
}

.form-layout {
	display: flex;
	flex-direction: column;
	gap: 16px;
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-group label {
	font-size: 0.85rem;
	font-weight: 600;
	color: var(--text-muted);
}

.form-group input {
	padding: 10px 12px;
	border: 1px solid var(--border-input);
	border-radius: 6px;
	background: var(--bg-body);
	color: var(--text-main);
	font-size: 0.95rem;
}

.form-actions {
	display: flex;
	justify-content: flex-end;
	margin-top: 8px;
}

.btn-sm {
	padding: 6px 12px;
	font-size: 13px;
}

/* Status badges for contact types */
.status-badge.main {
	background-color: rgba(59, 130, 246, 0.1);
	color: #3b82f6;
}

.status-badge.technical {
	background-color: rgba(16, 185, 129, 0.1);
	color: #10b981;
}

.status-badge.billing {
	background-color: rgba(245, 158, 11, 0.1);
	color: #f59e0b;
}
</style>
