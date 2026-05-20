<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useIgnoreStore } from "../stores/ignore";
import { useAssetStore } from "../stores/assetStore";
import { useAuthStore } from "../stores/auth";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import PluginInfoCard from "../components/PluginInfoCard.vue";
import PluginVulnerabilityList from "../components/PluginVulnerabilityList.vue";
import PluginSiteUpdateModal from "../components/PluginSiteUpdateModal.vue";
import AppIcon from "../components/AppIcon.vue";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const dataStore = useDataStore();
const ignoreStore = useIgnoreStore();
const assetStore = useAssetStore();
const authStore = useAuthStore();

onMounted(() => {
	assetStore.fetchAssets();
	ignoreStore.fetchIgnoreEntries();
});

const assetTemplate = computed(() => {
	return assetStore.assets.find(
		(a) => a.identifier === props.name && a.type === "Plugin",
	);
});

const info = computed(() => {
	const p = dataStore.enrichedPlugins.find((i) => i.slug === props.name);
	if (!p) return undefined;

	// Check if this specific plugin is ignored globally for vulnerabilities
	const isPluginIgnored = ignoreStore.isIgnored({
		pluginSlug: props.name,
		purpose: "vuln",
	});

	// ALWAYS Filter out specifically ignored vulnerability UUIDs
	const vulns = p.vulnerabilities
		.filter((v) => {
			return !ignoreStore.isIgnored({
				pluginSlug: props.name,
				vulnUuid: v.vulnerability.uuid,
				purpose: "vuln",
			});
		})
		.map((v) => ({
			...v,
			isSuppressed: isPluginIgnored,
		}));

	return { ...p, vulnerabilities: vulns };
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

			// Check if this site specifically ignores vulnerabilities (either site or server level)
			let isVulnerable = vulnerableSites.has(p.site_id);
			let isSuppressed = false;

			if (isVulnerable && site) {
				const isSiteIgnored = ignoreStore.isIgnored({
					siteId: site.id,
					serverId: site.server_id,
					purpose: "vuln",
				});
				if (isSiteIgnored) {
					isSuppressed = true;
				}
			}

			// If plugin itself is ignored, it's suppressed too
			const isPluginIgnored = ignoreStore.isIgnored({
				pluginSlug: props.name,
				purpose: "vuln",
			});
			if (isPluginIgnored) {
				isSuppressed = true;
			}

			return {
				...p,
				site_domain: site ? site.domain : "Unknown Site",
				site_id: p.site_id,
				isVulnerable,
				isSuppressed,
			};
		})
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
});

const showUpdateModal = ref(false);

const sitesWithUpdates = computed(() =>
	(dataStore.pluginsBySlugMap.get(props.name) || []).some(
		(p) => p.update !== "",
	),
);

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
					<AppIcon
						v-if="!assetTemplate"
						name="plus-circle"
						size="18"
					/>
					<AppIcon v-else name="tag" size="18" />
					{{
						assetTemplate
							? "View Asset Template"
							: "Create Asset Template"
					}}
				</button>
			</template>
		</ViewHeader>

		<main v-if="sitesWithPlugin.length > 0 || info" class="content mt-4">
			<PluginInfoCard
				:info="info"
				:installation-count="sitesWithPlugin.length"
			/>

			<PluginVulnerabilityList
				v-if="info?.vulnerabilities && info.vulnerabilities.length > 0"
				:vulnerabilities="info.vulnerabilities"
			/>

			<section class="card">
				<div class="card-header">
					<h2>Installed on Sites</h2>
					<button
						v-if="authStore.canExecute && sitesWithUpdates"
						class="btn btn-primary btn-sm"
						@click="showUpdateModal = true"
					>
						Update Available
					</button>
				</div>
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
								<td class="font-medium">
									{{ item.site_domain }}
								</td>
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
										class="status-badge"
										:class="
											item.isSuppressed
												? 'warning'
												: 'error'
										"
									>
										{{
											item.isSuppressed
												? "Suppressed"
												: "Yes"
										}}
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
					message="Loading plugin details..."
				/>
				<div v-else class="empty-state">
					<p>Plugin details not found.</p>
					<button class="back-btn mt-4" @click="goBack">
						Go back to plugins
					</button>
				</div>
			</div>
		</main>
		<PluginSiteUpdateModal
			:visible="showUpdateModal"
			:plugin-slug="name"
			@close="showUpdateModal = false"
		/>
	</div>
</template>

<style scoped>
.btn {
	gap: 8px;
}
</style>
