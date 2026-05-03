<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import InfoCard from "../components/InfoCard.vue";
import MonitorHistoryCard from "../components/MonitorHistoryCard.vue";
import { useMonitorStore } from "../stores/monitor";
import { useCompanyStore } from "../stores/company";
import type { Company, Contact } from "../types";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const dataStore = useDataStore();
const monitorStore = useMonitorStore();
const companyStore = useCompanyStore();

const siteId = parseInt(props.id, 10);
const company = ref<Company | null>(null);
const contacts = ref<Contact[]>([]);
const site = computed(() => dataStore.getSiteById(siteId));

monitorStore.ensureHistory();

watch(
	() => site.value?.domain,
	(domain) => {
		if (domain) {
			monitorStore.fetchStatus(domain);
		}
	},
	{ immediate: true },
);

watch(
	() => site.value?.id,
	async (id) => {
		if (id) {
			try {
				company.value = await companyStore.getCompanyForSite(id);
				dataStore.setSiteCompanyLink(id, company.value?.id);
				if (company.value) {
					contacts.value = await companyStore.fetchCompanyContacts(
						company.value.id,
					);
				}
			} catch (e) {
				console.error("Failed to fetch company for site", e);
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
		{ label: "Status", value: site.value.status },
	];
});

const serverInfoItems = computed(() => {
	if (!server.value) return [];
	return [
		{ label: "Server Name", value: server.value.name, copyable: true },
		{ label: "IP Address", value: server.value.ip_address, copyable: true },
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

const goToCompany = () => {
	if (company.value) {
		router.push({
			name: "company-detail",
			params: { id: company.value.id.toString() },
		});
	}
};

const showLinkModal = ref(false);
const companySearchQuery = ref("");
const searchResults = ref<Company[]>([]);
const isSearching = ref(false);

const handleSearch = async () => {
	if (companySearchQuery.value.length < 2) {
		searchResults.value = [];
		return;
	}
	isSearching.value = true;
	try {
		await companyStore.fetchCompanies(companySearchQuery.value);
		searchResults.value = companyStore.companies;
	} catch (e) {
		console.error("Search failed", e);
	} finally {
		isSearching.value = false;
	}
};

const linkCompany = async (compId: number) => {
	try {
		await companyStore.linkSiteToCompany(siteId, compId);
		dataStore.setSiteCompanyLink(siteId, compId);
		await dataStore.refreshData();
		company.value = await companyStore.getCompanyForSite(siteId);
		if (company.value) {
			contacts.value = await companyStore.fetchCompanyContacts(
				company.value.id,
			);
		}
		showLinkModal.value = false;
	} catch (e: any) {
		alert("Failed to link company: " + e.message);
	}
};

const unlinkCompany = async () => {
	if (!confirm("Are you sure you want to unlink this company?")) return;
	try {
		await companyStore.unlinkSite(siteId);
		dataStore.setSiteCompanyLink(siteId, undefined);
		await dataStore.refreshData();
		company.value = null;
		contacts.value = [];
	} catch (e: any) {
		alert("Failed to unlink company: " + e.message);
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
			<InfoCard title="Site Information" :items="siteInfoItems" />

			<InfoCard
				v-if="server"
				title="Server Information"
				:items="serverInfoItems"
			/>

			<section class="card">
				<div
					style="
						display: flex;
						justify-content: space-between;
						align-items: center;
						margin-bottom: 16px;
						border-bottom: 1px solid var(--border-color);
						padding-bottom: 8px;
					"
				>
					<h2 style="margin: 0; border: none">Company Information</h2>
					<div class="header-actions">
						<button
							v-if="company"
							class="text-btn"
							@click="unlinkCompany"
							style="color: #ef4444"
						>
							Unlink
						</button>
						<button
							v-if="company"
							class="back-btn"
							@click="goToCompany"
							style="padding: 4px 12px; font-size: 13px"
						>
							View Company
						</button>
						<button
							v-else
							class="btn btn-primary btn-sm"
							@click="showLinkModal = true"
						>
							Link Company
						</button>
					</div>
				</div>

				<div v-if="company">
					<div class="info-grid">
						<div class="info-item">
							<span class="label">Name:</span>
							<span class="value">{{ company.name }}</span>
						</div>
						<div class="info-item" v-if="company.vat_number">
							<span class="label">VAT Number:</span>
							<span class="value">{{ company.vat_number }}</span>
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
										<th>Email</th>
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
										<td>{{ contact.email || "—" }}</td>
									</tr>
								</tbody>
							</table>
						</div>
					</div>
				</div>
				<div v-else class="empty-state">No company linked to this site.</div>
			</section>

			<MonitorHistoryCard :history="history" :domain="site.domain" />

			<section class="card">
				<h2>Installed Plugins ({{ sitePlugins.length }})</h2>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Plugin Name</th>
								<th>Version</th>
								<th>Status</th>
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
								<td>{{ plugin.version }}</td>
								<td>
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
									<span v-else style="color: #999">—</span>
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
					<button class="back-btn" @click="goBack" style="margin-top: 16px">
						Go back to sites
					</button>
				</div>
			</div>
		</main>

		<!-- Link Company Modal -->
		<div
			v-if="showLinkModal"
			class="modal-overlay"
			@click.self="showLinkModal = false"
		>
			<div class="modal-content card">
				<h2>Link Company to Site</h2>
				<div class="form-layout">
					<div class="form-group">
						<label for="comp-search">Search Company</label>
						<input
							id="comp-search"
							v-model="companySearchQuery"
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
							@click="linkCompany(res.id)"
						>
							<div class="res-name">{{ res.name }}</div>
							<div class="res-vat" v-if="res.vat_number">
								{{ res.vat_number }}
							</div>
						</div>
					</div>
					<div
						v-else-if="companySearchQuery.length >= 2 && !isSearching"
						class="empty-state"
					>
						No companies found.
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
.contacts-preview {
	margin-top: 24px;
}

.contacts-preview h3 {
	font-size: 0.95rem;
	margin-bottom: 12px;
	color: var(--text-muted);
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
