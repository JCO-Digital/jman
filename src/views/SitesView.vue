<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import type { EnrichedSite } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";

const props = defineProps<{
	page?: number;
	rowsPerPage?: number;
}>();

const router = useRouter();
const dataStore = useDataStore();

const searchQuery = ref("");
const sortKey = ref<keyof EnrichedSite>("domain");
const sortOrder = ref<"asc" | "desc">("asc");
const currentPage = ref(props.page || 1);
const rowsPerPage = ref(props.rowsPerPage || 50);

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

	result = [...result].sort((a, b) => {
		let valA: any = a[sortKey.value];
		let valB: any = b[sortKey.value];

		if (sortKey.value === "server") {
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
	() => Math.ceil(filteredAndSortedSites.value.length / rowsPerPage.value) || 1,
);

const paginatedSites = computed(() => {
	const start = (currentPage.value - 1) * rowsPerPage.value;
	const end = start + rowsPerPage.value;
	return filteredAndSortedSites.value.slice(start, end);
});

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
	router.push({ name: "site-detail", params: { id: id.toString() } });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Site Management" show-refresh />

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<div class="controls">
			<input
				type="text"
				placeholder="Search sites by name or URL..."
				class="search-input"
				v-model="searchQuery"
			/>
		</div>

		<main class="table-container">
			<table class="data-table sortable">
				<thead>
					<tr>
						<th @click="handleSort('domain')">
							Site Name
							<span v-if="sortKey === 'domain'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('server')">
							Server
							<span v-if="sortKey === 'server'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('plugins')">
							Plugins
							<span v-if="sortKey === 'plugins'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('vulnerabilities')">
							Vulns
							<span v-if="sortKey === 'vulnerabilities'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
					</tr>
				</thead>
				<tbody>
					<tr v-if="dataStore.isLoading && dataStore.sites.length === 0">
						<td colspan="4">
							<LoadingSpinner message="Loading data..." />
						</td>
					</tr>
					<tr v-else-if="paginatedSites.length === 0">
						<td colspan="4" class="empty-state">
							<span v-if="searchQuery"
								>No sites found matching "{{ searchQuery }}".</span
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
						<td>{{ site.domain }}</td>
						<td>{{ site.server }}</td>
						<td>
							{{ site.is_wordpress ? site.plugins.length : "Not WP" }}
						</td>
						<td>
							<span
								v-if="site.vulnerabilities.length > 0"
								class="status-badge error"
								title="Vulnerabilities detected"
							>
								{{ site.vulnerabilities.length }}
							</span>
							<span v-else style="color: #999">—</span>
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
/* All specific styles moved to components or available in style.css */
</style>
