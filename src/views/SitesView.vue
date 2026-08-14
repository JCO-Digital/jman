<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useIgnoreStore } from "../stores/ignore";
import { useOrganizationStore } from "../stores/organization";
import type { EnrichedSite, SiteEnvironment, Organization } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";

const props = defineProps<{
	page?: number;
	rowsPerPage?: number;
}>();

const router = useRouter();
const dataStore = useDataStore();
const ignoreStore = useIgnoreStore();
const organizationStore = useOrganizationStore();

const searchQuery = ref("");
const filterEnvironment = ref<SiteEnvironment | "unclassified" | "">("");
const sortKey = ref<keyof EnrichedSite>("domain");
const sortOrder = ref<"asc" | "desc">("asc");
const currentPage = ref(props.page || 1);
const rowsPerPage = ref(props.rowsPerPage || 50);

const batchMode = ref(false);
const selectedSiteIds = ref<Set<number>>(new Set());
const batchEnvironment = ref<SiteEnvironment | "">("");
const isApplyingBatch = ref(false);

watch(
	() => props.page,
	(newVal) => {
		currentPage.value = newVal || 1;
	},
);

watch(
	() => props.rowsPerPage,
	(newVal) => {
		rowsPerPage.value = newVal || 50;
	},
);

onMounted(() => {
	ignoreStore.fetchIgnoreEntries();
	organizationStore.fetchOrganizations();
});

const organizationsById = computed(() => {
	const map = new Map<number, Organization>();
	for (const org of organizationStore.organizations) {
		map.set(org.id, org);
	}
	return map;
});

const getOrganizationName = (orgId?: number) => {
	if (!orgId) return "";
	return organizationsById.value.get(orgId)?.name || "";
};

const updateRoute = (page: number, rpp: number) => {
	router.push({
		name: "sites",
		params: {
			page: page.toString(),
			rowsPerPage: rpp.toString(),
		},
	});
};

const handleSort = (key: keyof EnrichedSite) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
};

const filteredAndSortedSites = computed(() => {
	let result = dataStore.enrichedSites;

	if (searchQuery.value) {
		const query = searchQuery.value.toLowerCase();
		result = result.filter((site) => {
			return site.domain.toLowerCase().includes(query);
		});
	}

	if (filterEnvironment.value) {
		result = result.filter((site) =>
			filterEnvironment.value === "unclassified"
				? !site.environment
				: site.environment === filterEnvironment.value,
		);
	}

	result = [...result].sort((a, b) => {
		let valA: any = a[sortKey.value];
		let valB: any = b[sortKey.value];

		if (sortKey.value === "organization_id") {
			valA = getOrganizationName(a.organization_id).toLowerCase();
			valB = getOrganizationName(b.organization_id).toLowerCase();
		} else if (sortKey.value === "server") {
			valA = a.server.toLowerCase();
			valB = b.server.toLowerCase();
		} else if (sortKey.value === "plugins") {
			valA = a.plugins.length;
			valB = b.plugins.length;
		} else if (sortKey.value === "vulnerabilities") {
			valA = a.vulnerabilities.length;
			valB = b.vulnerabilities.length;
		} else if (typeof valA === "string") {
			valA = valA.toLowerCase();
			valB = valB.toLowerCase();
		}

		if (valA < valB) return sortOrder.value === "asc" ? -1 : 1;
		if (valA > valB) return sortOrder.value === "asc" ? 1 : -1;
		return 0;
	});

	return result;
});

const totalPages = computed(
	() =>
		Math.ceil(filteredAndSortedSites.value.length / rowsPerPage.value) || 1,
);

const paginatedSites = computed(() => {
	const start = (currentPage.value - 1) * rowsPerPage.value;
	const end = start + rowsPerPage.value;
	return filteredAndSortedSites.value.slice(start, end);
});

const allOnPageSelected = computed(
	() =>
		paginatedSites.value.length > 0 &&
		paginatedSites.value.every((site) =>
			selectedSiteIds.value.has(site.id),
		),
);

const toggleSelectAllOnPage = () => {
	if (allOnPageSelected.value) {
		for (const site of paginatedSites.value) {
			selectedSiteIds.value.delete(site.id);
		}
	} else {
		for (const site of paginatedSites.value) {
			selectedSiteIds.value.add(site.id);
		}
	}
};

const toggleSiteSelected = (id: number) => {
	if (selectedSiteIds.value.has(id)) {
		selectedSiteIds.value.delete(id);
	} else {
		selectedSiteIds.value.add(id);
	}
};

const toggleBatchMode = () => {
	batchMode.value = !batchMode.value;
	selectedSiteIds.value.clear();
	batchEnvironment.value = "";
};

const applyBatchEnvironment = async () => {
	if (selectedSiteIds.value.size === 0) return;
	isApplyingBatch.value = true;
	try {
		await Promise.all(
			Array.from(selectedSiteIds.value).map((id) =>
				dataStore.setSiteEnvironment(id, batchEnvironment.value),
			),
		);
		selectedSiteIds.value.clear();
	} finally {
		isApplyingBatch.value = false;
	}
};

const prevPage = () => {
	if (currentPage.value > 1) {
		updateRoute(currentPage.value - 1, rowsPerPage.value);
	}
};

const nextPage = () => {
	if (currentPage.value < totalPages.value) {
		updateRoute(currentPage.value + 1, rowsPerPage.value);
	}
};

const handleRowsPerPageUpdate = (newRpp: number) => {
	updateRoute(1, newRpp);
};

const goToSite = (id: number) => {
	if (batchMode.value) {
		toggleSiteSelected(id);
		return;
	}
	router.push({ name: "site-detail", params: { id: id.toString() } });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Site Management" />

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search sites by name or URL..."
				class="search-input"
				@input="updateRoute(1, rowsPerPage)"
			/>

			<select
				v-model="filterEnvironment"
				@change="updateRoute(1, rowsPerPage)"
			>
				<option value="">All Environments</option>
				<option value="production">Production</option>
				<option value="staging">Staging</option>
				<option value="development">Development</option>
				<option value="archived">Archived</option>
				<option value="unclassified">Unclassified</option>
			</select>

			<button
				class="btn btn-outline"
				:class="{ 'btn-primary': batchMode }"
				@click="toggleBatchMode"
			>
				{{ batchMode ? "Cancel Batch Edit" : "Batch Edit" }}
			</button>
		</div>

		<div v-if="batchMode" class="card-muted batch-action-bar">
			<span class="font-medium"
				>{{ selectedSiteIds.size }} site(s) selected</span
			>
			<select v-model="batchEnvironment">
				<option value="">Unclassified</option>
				<option value="production">Production</option>
				<option value="staging">Staging</option>
				<option value="development">Development</option>
				<option value="archived">Archived</option>
			</select>
			<button
				class="btn btn-primary btn-sm"
				:disabled="selectedSiteIds.size === 0 || isApplyingBatch"
				@click="applyBatchEnvironment"
			>
				{{ isApplyingBatch ? "Applying…" : "Apply" }}
			</button>
		</div>

		<main class="table-container">
			<table class="data-table sortable">
				<thead>
					<tr>
						<th v-if="batchMode" class="col-narrow text-center">
							<input
								type="checkbox"
								:checked="allOnPageSelected"
								@click="toggleSelectAllOnPage"
							/>
						</th>
						<th class="col-expand" @click="handleSort('domain')">
							Site Name
							<span v-if="sortKey === 'domain'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="hide-mobile col-medium"
							@click="handleSort('organization_id')"
						>
							Organization
							<span v-if="sortKey === 'organization_id'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="hide-mobile col-wide"
							@click="handleSort('server')"
						>
							Server
							<span v-if="sortKey === 'server'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="col-narrow text-center"
							@click="handleSort('environment')"
						>
							Environment
							<span v-if="sortKey === 'environment'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="col-narrow text-center"
							@click="handleSort('plugins')"
						>
							Plugins
							<span v-if="sortKey === 'plugins'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="col-narrow text-center"
							@click="handleSort('vulnerabilities')"
						>
							Vulns
							<span v-if="sortKey === 'vulnerabilities'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							dataStore.isLoading && dataStore.sites.length === 0
						"
					>
						<td colspan="6" class="hide-mobile">
							<LoadingSpinner message="Loading data..." />
						</td>
						<td colspan="4" class="show-mobile">
							<LoadingSpinner message="Loading data..." />
						</td>
					</tr>
					<tr v-else-if="paginatedSites.length === 0">
						<td colspan="6" class="empty-state hide-mobile">
							<span v-if="searchQuery"
								>No sites found matching "{{
									searchQuery
								}}".</span
							>
							<span v-else>No sites available.</span>
						</td>
						<td colspan="4" class="empty-state show-mobile">
							<span v-if="searchQuery"
								>No sites found matching "{{
									searchQuery
								}}".</span
							>
							<span v-else>No sites available.</span>
						</td>
					</tr>
					<tr
						v-for="site in paginatedSites"
						:key="site.id"
						class="clickable-row"
						@click="goToSite(site.id)"
					>
						<td v-if="batchMode" class="col-narrow text-center">
							<input
								type="checkbox"
								:checked="selectedSiteIds.has(site.id)"
								@click.stop="toggleSiteSelected(site.id)"
							/>
						</td>
						<td class="font-medium truncate col-expand">
							{{ site.domain }}
						</td>
						<td
							class="hide-mobile col-medium truncate"
							:title="
								site.organization_id
									? getOrganizationName(site.organization_id)
									: ''
							"
						>
							<span v-if="site.organization_id">
								{{ getOrganizationName(site.organization_id) }}
							</span>
							<span v-else class="text-muted">—</span>
						</td>
						<td class="hide-mobile col-wide truncate">
							{{ site.server }}
						</td>
						<td class="col-narrow text-center">
							<span
								v-if="site.environment"
								class="status-badge badge-sm"
								:class="site.environment"
							>
								{{ site.environment }}
							</span>
							<span v-else class="text-muted">—</span>
						</td>
						<td class="col-narrow text-center">
							{{
								site.is_wordpress
									? site.plugins.length
									: "Not WP"
							}}
						</td>
						<td class="col-narrow text-center">
							<span
								v-if="site.vulnerabilities.length > 0"
								class="status-badge"
								:class="
									site.vulnerabilities.every(
										(v) => v.suppressed,
									)
										? 'warning'
										: 'error'
								"
								:title="
									site.vulnerabilities.every(
										(v) => v.suppressed,
									)
										? 'All vulnerabilities are suppressed via ignore rules'
										: `${site.vulnerabilities.length} vulnerabilities detected`
								"
							>
								{{ site.vulnerabilities.length }}
							</span>
							<span v-else class="text-muted">—</span>
						</td>
					</tr>
				</tbody>
			</table>

			<Pagination
				:current-page="currentPage"
				:total-pages="totalPages"
				:rows-per-page="rowsPerPage"
				@update:rows-per-page="handleRowsPerPageUpdate"
				@prev="prevPage"
				@next="nextPage"
			/>
		</main>
	</div>
</template>

<style scoped>
.batch-action-bar {
	display: flex;
	align-items: center;
	gap: 16px;
	margin-bottom: 24px;
}
</style>
