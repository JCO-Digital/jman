<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useMonitorStore } from "../stores/monitor";
import { useOrganizationStore } from "../stores/organization";
import { useAuthStore } from "../stores/auth";
import { useIgnoreStore } from "../stores/ignore";
import { useToastStore } from "../stores/toast";
import type { Organization, Contact } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import InfoCard, { type InfoItem } from "../components/InfoCard.vue";
import MonitorHistoryCard from "../components/MonitorHistoryCard.vue";
import PluginUpdateModal from "../components/PluginUpdateModal.vue";

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

const siteId = parseInt(props.id, 10);
const organization = ref<Organization | null>(null);
const contacts = ref<Contact[]>([]);
const site = computed(() => dataStore.getSiteById(siteId));

// Fetch initial data
onMounted(async () => {
	await Promise.all([dataStore.initData(), ignoreStore.fetchIgnoreEntries()]);
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
			isSuppressed: vulns.length > 0 && vulns.every((v) => v.suppressed),
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

	if (site.value.database?.table_prefix) {
		items.push({
			label: "Table Prefix",
			value: site.value.database.table_prefix,
			copyable: true,
		});
	}

	items.push({ label: "Status", value: site.value.status });
	return items;
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

		<main v-if="site" class="content mt-4">
			<div class="grid-2-cols">
				<InfoCard title="Site Information" :items="siteInfoItems" />
				<InfoCard
					v-if="server"
					title="Server Information"
					:items="serverInfoItems"
				/>
			</div>

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
											plugin.isSuppressed
												? 'warning'
												: 'error'
										"
										:title="`${plugin.vulnerabilities.length} vulnerabilities detected${plugin.isSuppressed ? ' (Suppressed)' : ''}`"
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
			@close="showPluginUpdateModal = false"
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
	</div>
</template>
