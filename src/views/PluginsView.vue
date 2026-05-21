<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useIgnoreStore } from "../stores/ignore";
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

onMounted(() => {
	ignoreStore.fetchIgnoreEntries();
});

const searchQuery = ref("");
const sortKey = ref<
	"name" | "count" | "version" | "author" | "vulnerabilities"
>("name");
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
		name: "plugins",
		params: {
			page: page.toString(),
			rowsPerPage: rpp.toString(),
		},
	});
};

const handleSort = (
	key: "name" | "count" | "version" | "author" | "vulnerabilities",
) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
};

const uniquePlugins = computed(() => {
	let filtered = [...dataStore.enrichedPlugins];

	if (searchQuery.value) {
		const query = searchQuery.value.toLowerCase();
		filtered = filtered.filter(
			(p) =>
				p.name.toLowerCase().includes(query) ||
				p.slug.toLowerCase().includes(query) ||
				p.author.toLowerCase().includes(query),
		);
	}

	filtered.sort((a, b) => {
		let valA: any = (a as any)[sortKey.value];
		let valB: any = (b as any)[sortKey.value];

		if (sortKey.value === "vulnerabilities") {
			valA = (valA as any[]).length;
			valB = (valB as any[]).length;
		}

		if (typeof valA === "string") {
			valA = valA.toLowerCase();
			valB = valB.toLowerCase();
		}

		if (valA < valB) return sortOrder.value === "asc" ? -1 : 1;
		if (valA > valB) return sortOrder.value === "asc" ? 1 : -1;
		return 0;
	});

	return filtered;
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

const goToPlugin = (name: string) => {
	router.push({
		name: "plugin-detail",
		params: { name },
	});
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Plugins Management" />

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search plugins by name..."
				class="search-input"
				@input="updateRoute(1, rowsPerPage)"
			/>
		</div>

		<main class="table-container">
			<table class="data-table sortable">
				<thead>
					<tr>
						<th class="col-expand" @click="handleSort('name')">
							Plugin Name
							<span v-if="sortKey === 'name'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="hide-mobile col-version"
							@click="handleSort('version')"
						>
							Version
							<span v-if="sortKey === 'version'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="hide-mobile col-medium"
							@click="handleSort('author')"
						>
							Author
							<span v-if="sortKey === 'author'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th
							class="col-narrow text-center"
							@click="handleSort('count')"
						>
							Sites
							<span v-if="sortKey === 'count'">{{
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
							dataStore.isLoading &&
							dataStore.pluginInfo.length === 0
						"
					>
						<td colspan="5" class="hide-mobile">
							<LoadingSpinner message="Loading data..." />
						</td>
						<td colspan="3" class="show-mobile">
							<LoadingSpinner message="Loading data..." />
						</td>
					</tr>
					<tr v-else-if="paginatedPlugins.length === 0">
						<td colspan="5" class="empty-state hide-mobile">
							<span v-if="searchQuery"
								>No plugins found matching "{{
									searchQuery
								}}".</span
							>
							<span v-else>No plugins available.</span>
						</td>
						<td colspan="3" class="empty-state show-mobile">
							<span v-if="searchQuery"
								>No plugins found matching "{{
									searchQuery
								}}".</span
							>
							<span v-else>No plugins available.</span>
						</td>
					</tr>
					<tr
						v-for="plugin in paginatedPlugins"
						:key="plugin.slug"
						class="clickable-row"
						@click="goToPlugin(plugin.slug)"
					>
						<td class="truncate col-expand">
							<div class="plugin-title">
								{{ plugin.shortName }}
							</div>
							<div class="sub-text">
								{{ plugin.slug }}
							</div>
						</td>
						<td class="hide-mobile col-version truncate">
							{{ plugin.version }}
						</td>
						<td class="hide-mobile col-medium truncate">
							{{ plugin.author }}
						</td>
						<td class="col-narrow text-center">
							{{ plugin.count }}
						</td>
						<td class="col-narrow text-center">
							<span
								v-if="plugin.vulnerabilities.length > 0"
								class="status-badge"
								:class="
									plugin.vulnerabilities.every(
										(v) => v.isSuppressed,
									)
										? 'warning'
										: 'error'
								"
								title="Vulnerabilities detected"
							>
								{{ plugin.vulnerabilities.length }}
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
