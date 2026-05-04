<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import PluginInfoCard from "../components/PluginInfoCard.vue";
import PluginVulnerabilityList from "../components/PluginVulnerabilityList.vue";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const dataStore = useDataStore();

const info = computed(() => {
	return dataStore.enrichedPlugins.find((i) => i.slug === props.name);
});

const sitesWithPlugin = computed(() => {
	const vulnerableSites = new Set(
		info.value?.vulnerabilities.flatMap((v) => v.sites.map((s) => s.site_id)) ||
			[],
	);

	const instances = dataStore.pluginsBySlugMap.get(props.name) || [];

	return instances
		.map((p) => {
			const site = dataStore.getSiteById(p.site_id);
			return {
				...p,
				site_domain: site ? site.domain : "Unknown Site",
				site_id: p.site_id,
				isVulnerable: vulnerableSites.has(p.site_id),
			};
		})
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
});

const goBack = () => {
	router.push({ name: "plugins" });
};

const goToSite = (siteId: number) => {
	router.push({ name: "site-detail", params: { id: siteId.toString() } });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader
			title="Plugin Details"
			:back-button="{ text: 'Back to Plugins', onClick: goBack }"
		/>

		<main class="content" v-if="sitesWithPlugin.length > 0 || info">
			<PluginInfoCard
				:info="info"
				:installation-count="sitesWithPlugin.length"
			/>

			<PluginVulnerabilityList
				v-if="info?.vulnerabilities && info.vulnerabilities.length > 0"
				:vulnerabilities="info.vulnerabilities"
			/>

			<section class="card">
				<h2>Installed on Sites</h2>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Site Domain</th>
								<th class="hide-mobile">Version</th>
								<th class="hide-mobile">Status</th>
								<th>Vuln</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-for="item in sitesWithPlugin"
								:key="item.site_id"
								class="clickable-row"
								@click="goToSite(item.site_id)"
							>
								<td>{{ item.site_domain }}</td>
								<td class="hide-mobile">{{ item.version }}</td>
								<td class="hide-mobile">
									<span :class="['status-badge', item.status.toLowerCase()]">
										{{ item.status }}
									</span>
								</td>
								<td>
									<span v-if="item.isVulnerable" class="status-badge error">
										Yes
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
					message="Loading plugin details..."
				/>
				<div v-else class="empty-state">
					<p>Plugin details not found.</p>
					<button class="back-btn not-found-back-btn" @click="goBack">
						Go back to plugins
					</button>
				</div>
			</div>
		</main>
	</div>
</template>

<style scoped>
/* All specific styles moved to components or available in style.css */
.empty-dash {
	color: #999;
}

.not-found-back-btn {
	margin-top: 16px;
}
</style>
