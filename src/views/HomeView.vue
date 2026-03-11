<script setup lang="ts">
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import { useData } from "../composables/useData";
import type { Site } from "../types";

const router = useRouter();
const { sites, servers, plugins, isLoading, error, refreshData } = useData();

const searchQuery = ref("");
const sortKey = ref<keyof Site | "server" | "plugins">("domain");
const sortOrder = ref<"asc" | "desc">("asc");
const currentPage = ref(1);
const rowsPerPage = ref(50);

const getServerName = (serverId: number) => {
	const server = servers.value.find((s) => s.id === serverId);
	return server ? server.name : "Unknown";
};

const getPluginCount = (siteId: number) => {
	return plugins.value.filter((p) => p.site_id === siteId).length;
};

const handleSort = (key: string) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key as any;
		sortOrder.value = "asc";
	}
};

const filteredAndSortedSites = computed(() => {
	let result = sites.value;

	if (searchQuery.value) {
		const query = searchQuery.value.toLowerCase();
		result = result.filter((site) => {
			return site.domain.toLowerCase().includes(query);
		});
	}

	result = [...result].sort((a, b) => {
		let valA: any = a[sortKey.value as keyof Site];
		let valB: any = b[sortKey.value as keyof Site];

		if (sortKey.value === "server") {
			valA = getServerName(a.server_id).toLowerCase();
			valB = getServerName(b.server_id).toLowerCase();
		} else if (sortKey.value === "plugins") {
			valA = getPluginCount(a.id);
			valB = getPluginCount(b.id);
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
	if (currentPage.value > 1) currentPage.value--;
};

const nextPage = () => {
	if (currentPage.value < totalPages.value) currentPage.value++;
};

const goToSite = (id: number) => {
	router.push({ name: "site-detail", params: { id: id.toString() } });
};
</script>

<template>
	<div class="view-container">
		<header class="header">
			<h1>Site Management</h1>
			<button
				class="btn btn-primary"
				@click="refreshData"
				:disabled="isLoading"
			>
				<span
					v-if="isLoading"
					class="spinner spinner-small"
					style="margin-right: 8px; vertical-align: middle"
				></span>
				<span style="vertical-align: middle">{{
					isLoading ? "Refreshing..." : "Refresh Data"
				}}</span>
			</button>
		</header>

		<div v-if="error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ error }}</p>
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
					</tr>
				</thead>
				<tbody>
					<tr v-if="isLoading && sites.length === 0">
						<td colspan="3" class="empty-state">
							<div class="spinner" style="margin-bottom: 12px"></div>
							<div>Loading data...</div>
						</td>
					</tr>
					<tr v-else-if="paginatedSites.length === 0">
						<td colspan="3" class="empty-state">
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
						<td>{{ getServerName(site.server_id) }}</td>
						<td>{{ getPluginCount(site.id) }}</td>
					</tr>
				</tbody>
			</table>

			<div class="pagination">
				<div class="rows-per-page">
					<label for="per-page">Rows per page:</label>
					<select
						id="per-page"
						v-model.number="rowsPerPage"
						@change="currentPage = 1"
					>
						<option value="50">50</option>
						<option value="100">100</option>
						<option value="150">150</option>
						<option value="200">200</option>
						<option value="250">250</option>
					</select>
				</div>
				<div class="page-controls">
					<button :disabled="currentPage === 1" @click="prevPage">
						&laquo; Prev
					</button>
					<span>Page {{ currentPage }} of {{ totalPages }}</span>
					<button :disabled="currentPage === totalPages" @click="nextPage">
						Next &raquo;
					</button>
				</div>
			</div>
		</main>
	</div>
</template>

<style scoped>
/* All generic styles moved to style.css */
</style>
