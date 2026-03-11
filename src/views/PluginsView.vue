<script setup lang="ts">
import { ref, computed } from "vue";
import { useRouter } from "vue-router";
import { useData } from "../composables/useData";

const router = useRouter();
const { plugins, isLoading, error, refreshData } = useData();

const searchQuery = ref("");
const sortKey = ref<"name" | "count">("name");
const sortOrder = ref<"asc" | "desc">("asc");
const currentPage = ref(1);
const rowsPerPage = ref(50);

const handleSort = (key: "name" | "count") => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
};

const uniquePlugins = computed(() => {
	const pluginMap = new Map<string, number>();

	plugins.value.forEach((p) => {
		const count = pluginMap.get(p.name) || 0;
		pluginMap.set(p.name, count + 1);
	});

	let result = Array.from(pluginMap.entries()).map(([name, count]) => ({
		name,
		count,
	}));

	if (searchQuery.value) {
		const query = searchQuery.value.toLowerCase();
		result = result.filter((p) => p.name.toLowerCase().includes(query));
	}

	result.sort((a, b) => {
		let valA: any = a[sortKey.value];
		let valB: any = b[sortKey.value];

		if (typeof valA === "string") {
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
	() => Math.ceil(uniquePlugins.value.length / rowsPerPage.value) || 1,
);

const paginatedPlugins = computed(() => {
	const start = (currentPage.value - 1) * rowsPerPage.value;
	const end = start + rowsPerPage.value;
	return uniquePlugins.value.slice(start, end);
});

const prevPage = () => {
	if (currentPage.value > 1) currentPage.value--;
};

const nextPage = () => {
	if (currentPage.value < totalPages.value) currentPage.value++;
};

const goToPlugin = (name: string) => {
	router.push({
		name: "plugin-detail",
		params: { name: encodeURIComponent(name) },
	});
};
</script>

<template>
	<div class="view-container">
		<header class="header">
			<h1>Plugins Management</h1>
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
				placeholder="Search plugins by name..."
				class="search-input"
				v-model="searchQuery"
				@input="currentPage = 1"
			/>
		</div>

		<main class="table-container">
			<table class="data-table sortable">
				<thead>
					<tr>
						<th @click="handleSort('name')">
							Plugin Name
							<span v-if="sortKey === 'name'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('count')">
							Installed on Sites
							<span v-if="sortKey === 'count'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
					</tr>
				</thead>
				<tbody>
					<tr v-if="isLoading && plugins.length === 0">
						<td colspan="2" class="empty-state">
							<div class="spinner" style="margin-bottom: 12px"></div>
							<div>Loading data...</div>
						</td>
					</tr>
					<tr v-else-if="paginatedPlugins.length === 0">
						<td colspan="2" class="empty-state">
							<span v-if="searchQuery"
								>No plugins found matching "{{ searchQuery }}".</span
							>
							<span v-else>No plugins available.</span>
						</td>
					</tr>
					<tr
						v-for="plugin in paginatedPlugins"
						:key="plugin.name"
						class="clickable-row"
						@click="goToPlugin(plugin.name)"
					>
						<td>{{ plugin.name }}</td>
						<td>{{ plugin.count }}</td>
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
