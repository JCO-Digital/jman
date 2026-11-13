<script setup lang="ts">
import { onMounted } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useIgnoreStore } from "../stores/ignore";
import ViewHeader from "../components/ViewHeader.vue";
import ListToolbar from "../components/ListToolbar.vue";
import SortableTh from "../components/SortableTh.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import { useListState } from "../composables/useListState";
import { useClientList } from "../composables/useClientList";

const router = useRouter();
const dataStore = useDataStore();
const ignoreStore = useIgnoreStore();

onMounted(() => {
	ignoreStore.fetchIgnoreEntries();
});

// /plugins?q=&sort=&dir=&page=&per=
const list = useListState({
	sortKeys: ["name", "version", "author", "sites", "vulns"],
	defaultSort: "name",
	descFirst: ["sites", "vulns"],
});

const plugins = useClientList(() => dataStore.enrichedPlugins, list, {
	search: (p, q) =>
		p.name.toLowerCase().includes(q) ||
		p.slug.toLowerCase().includes(q) ||
		p.author.toLowerCase().includes(q),
	sortValue: (p, key) => {
		switch (key) {
			case "name":
				return p.name;
			case "version":
				return p.version;
			case "author":
				return p.author;
			case "sites":
				return p.count;
			case "vulns":
				return p.vulnerabilities.length;
		}
	},
});

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

		<ListToolbar
			:list="list"
			placeholder="Search plugins by name, slug or author"
		/>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<SortableTh
							:list="list"
							sort-key="name"
							class="col-expand"
						>
							Plugin Name
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="version"
							class="col-version"
						>
							Version
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="author"
							class="col-wide"
						>
							Author
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="sites"
							class="col-narrow text-center"
						>
							Sites
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="vulns"
							class="col-narrow text-center"
						>
							Vulns
						</SortableTh>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							dataStore.isLoading &&
							dataStore.pluginInfo.length === 0
						"
					>
						<td colspan="5">
							<LoadingSpinner message="Loading data..." />
						</td>
					</tr>
					<tr v-else-if="plugins.pageItems.value.length === 0">
						<td colspan="5" class="empty-state">
							<span v-if="list.query.value"
								>No plugins found matching "{{
									list.query.value
								}}".</span
							>
							<span v-else>No plugins available.</span>
						</td>
					</tr>
					<tr
						v-for="plugin in plugins.pageItems.value"
						:key="plugin.slug"
						class="clickable-row"
						@click="goToPlugin(plugin.slug)"
					>
						<td class="truncate col-expand">
							<RouterLink
								:to="{
									name: 'plugin-detail',
									params: { name: plugin.slug },
								}"
								class="plugin-title row-link"
								@click.stop
							>
								{{ plugin.shortName }}
							</RouterLink>
							<div class="sub-text">
								{{ plugin.slug }}
							</div>
						</td>
						<td class="col-version truncate">
							{{ plugin.version }}
						</td>
						<td class="col-wide truncate">
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
										(v) => v.suppressed,
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
				:list="list"
				:page="plugins.page.value"
				:total="plugins.total.value"
			/>
		</main>
	</div>
</template>
