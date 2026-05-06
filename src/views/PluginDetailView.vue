<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useAssetStore } from "../stores/assetStore";
import { useAuthStore } from "../stores/auth";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import PluginInfoCard from "../components/PluginInfoCard.vue";
import PluginVulnerabilityList from "../components/PluginVulnerabilityList.vue";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const dataStore = useDataStore();
const assetStore = useAssetStore();
const authStore = useAuthStore();

onMounted(() => {
	assetStore.fetchAssets();
});

const assetTemplate = computed(() => {
	return assetStore.assets.find(
		(a) => a.identifier === props.name && a.type === "Plugin",
	);
});

const info = computed(() => {
	return dataStore.enrichedPlugins.find((i) => i.slug === props.name);
});

const sitesWithPlugin = computed(() => {
	const vulnerableSites = new Set(
		info.value?.vulnerabilities.flatMap((v) =>
			v.sites.map((s) => s.site_id),
		) || [],
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

const manageAssetTemplate = () => {
	router.push({
		name: "asset-templates",
		query: !assetTemplate.value
			? {
					create: "true",
					type: "Plugin",
					identifier: props.name,
					name: info.value?.name || props.name,
				}
			: {
					search: props.name,
				},
	});
};
</script>

<template>
	<div class="view-container">
		<ViewHeader
			title="Plugin Details"
			:back-button="{ text: 'Back to Plugins', onClick: goBack }"
		>
			<template v-if="authStore.canEdit" #actions>
				<button
					class="btn"
					:class="assetTemplate ? 'btn-outline' : 'btn-primary'"
					@click="manageAssetTemplate"
				>
					<svg
						v-if="!assetTemplate"
						xmlns="http://www.w3.org/2000/svg"
						width="18"
						height="18"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<circle cx="12" cy="12" r="10"></circle>
						<line x1="12" y1="8" x2="12" y2="16"></line>
						<line x1="8" y1="12" x2="16" y2="12"></line>
					</svg>
					<svg
						v-else
						xmlns="http://www.w3.org/2000/svg"
						width="18"
						height="18"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path
							d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"
						></path>
						<line x1="7" y1="7" x2="7.01" y2="7"></line>
					</svg>
					{{
						assetTemplate
							? "View Asset Template"
							: "Create Asset Template"
					}}
				</button>
			</template>
		</ViewHeader>

		<main v-if="sitesWithPlugin.length > 0 || info" class="content">
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
									<span
										:class="[
											'status-badge',
											item.status.toLowerCase(),
										]"
									>
										{{ item.status }}
									</span>
								</td>
								<td>
									<span
										v-if="item.isVulnerable"
										class="status-badge error"
									>
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

		<main v-else class="content">
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

.btn {
	display: flex;
	align-items: center;
	gap: 8px;
}

.btn-outline {
	background-color: transparent;
	border: 1px solid var(--border-input);
	color: var(--text-main);
}

.btn-outline:hover {
	background-color: var(--bg-hover);
}
</style>
