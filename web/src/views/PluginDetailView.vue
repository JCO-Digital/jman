<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useIgnoreStore } from "../stores/ignore";
import { useAssetStore } from "../stores/assetStore";
import { useAuthStore } from "../stores/auth";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { usePluginUpdatesStore } from "../stores/pluginUpdates";
import { useToastStore } from "../stores/toast";
import { useConfirm } from "../composables/useConfirm";
import { availablePluginUpdate } from "../utils/format";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import PluginInfoCard from "../components/PluginInfoCard.vue";
import PluginVulnerabilityList from "../components/PluginVulnerabilityList.vue";
import PluginRowActions from "../components/PluginRowActions.vue";
import AppIcon from "../components/AppIcon.vue";
import NotesWidget from "../components/NotesWidget.vue";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const dataStore = useDataStore();
const ignoreStore = useIgnoreStore();
const assetStore = useAssetStore();
const authStore = useAuthStore();
const jobsStore = useUpdateJobsStore();
const pluginUpdatesStore = usePluginUpdatesStore();
const toast = useToastStore();
const { confirm } = useConfirm();

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
			const enrichedSite = dataStore.enrichedSites.find(
				(s) => s.id === p.site_id,
			);

			// Check if this site specifically suppresses these vulnerabilities
			let isVulnerable = vulnerableSites.has(p.site_id);
			let suppressed = false;

			if (isVulnerable && enrichedSite) {
				const siteVulns = enrichedSite.vulnerabilities.filter(
					(v) => v.slug === props.name,
				);
				if (
					siteVulns.length > 0 &&
					siteVulns.every((v) => v.suppressed)
				) {
					suppressed = true;
				}
			}

			return {
				...p,
				site_domain: site ? site.domain : "Unknown Site",
				site_id: p.site_id,
				isVulnerable,
				suppressed,
				// Plugin changes run over WP-CLI.
				canManage: authStore.canExecute && !!site?.can_wp_cli,
			};
		})
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
});

// --- Plugin management ---
const manageableSites = computed(() =>
	sitesWithPlugin.value.filter((s) => s.canManage),
);
const showActions = computed(() => manageableSites.value.length > 0);

/** Sites with a cached update and no job already queued on the plugin. */
const updatableSites = computed(() =>
	manageableSites.value.filter(
		(s) =>
			availablePluginUpdate(s) &&
			!jobsStore.pendingAction(s.site_id, s.name),
	),
);
const vulnerableUpdatable = computed(() =>
	updatableSites.value.filter((s) => s.isVulnerable && !s.suppressed),
);

// "done/total" while a check across sites runs.
const checkProgress = ref<string | null>(null);

async function checkForUpdates() {
	const siteIds = manageableSites.value.map((s) => s.site_id);
	if (siteIds.length === 0) return;
	checkProgress.value = `0/${siteIds.length}`;
	try {
		const { failed } = await pluginUpdatesStore.checkSites(
			siteIds,
			(done, total) => (checkProgress.value = `${done}/${total}`),
		);
		const available = updatableSites.value.length;
		if (failed.length) {
			toast.addToast(
				`Checked ${props.name} on ${siteIds.length - failed.length} of ${siteIds.length} sites; ${failed.length} failed. ${available} update${available === 1 ? "" : "s"} available.`,
				"error",
			);
		} else {
			toast.addToast(
				available
					? `${props.name}: ${available} site${available === 1 ? "" : "s"} can be updated.`
					: `${props.name} is up to date on all sites.`,
				available ? "info" : "success",
			);
		}
	} finally {
		checkProgress.value = null;
	}
}

/** Queues one background job per site. */
async function updateSites(
	sites: { site_id: string; name: string }[],
	vulnerable: boolean,
) {
	if (sites.length === 0) return;
	const what = vulnerable ? "vulnerable site" : "site";
	const ok = await confirm(
		`Update ${props.name} on ${sites.length} ${what}${sites.length === 1 ? "" : "s"}?`,
		{ confirmLabel: "Update" },
	);
	if (!ok) return;
	try {
		await jobsStore.enqueue(
			sites.map((s) => ({ site_id: s.site_id, plugins: [s.name] })),
		);
	} catch (e: any) {
		toast.addToast(`Failed to queue plugin updates: ${e.message}`, "error");
	}
}

const goBack = () => {
	router.push({ name: "plugins" });
};

const goToSite = (siteId: string) => {
	router.push({ name: "site-detail", params: { id: siteId } });
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

			<NotesWidget parent-type="Plugin" :parent-id="name" />

			<PluginVulnerabilityList
				v-if="info?.vulnerabilities && info.vulnerabilities.length > 0"
				:vulnerabilities="info.vulnerabilities"
			/>

			<section class="card">
				<div class="card-header">
					<h2>Installed on Sites</h2>
					<div v-if="showActions" class="plugin-card-actions">
						<button
							class="btn btn-outline btn-sm"
							:disabled="!!checkProgress"
							@click="checkForUpdates"
						>
							<span
								v-if="checkProgress"
								class="spinner spinner-small"
							></span>
							{{
								checkProgress
									? `Checking ${checkProgress}…`
									: "Check for updates"
							}}
						</button>
						<button
							v-if="vulnerableUpdatable.length"
							class="btn btn-outline btn-sm"
							@click="updateSites(vulnerableUpdatable, true)"
						>
							Update vulnerable ({{ vulnerableUpdatable.length }})
						</button>
						<button
							v-if="updatableSites.length"
							class="btn btn-primary btn-sm"
							@click="updateSites(updatableSites, false)"
						>
							Update all ({{ updatableSites.length }})
						</button>
					</div>
				</div>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Site Domain</th>
								<th>Version</th>
								<th>Status</th>
								<th>Vuln</th>
								<th v-if="showActions" class="text-right">
									Actions
								</th>
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
								<td>
									{{ item.version }}
									<span
										v-if="availablePluginUpdate(item)"
										class="plugin-update-available"
										:title="`Update available: ${availablePluginUpdate(item)}`"
									>
										→ {{ availablePluginUpdate(item) }}
									</span>
								</td>
								<td>
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
											item.suppressed
												? 'warning'
												: 'error'
										"
									>
										{{
											item.suppressed
												? "Suppressed"
												: "Yes"
										}}
									</span>
									<span v-else class="text-muted">—</span>
								</td>
								<td v-if="showActions">
									<PluginRowActions
										v-if="item.canManage"
										:site-id="item.site_id"
										:site-name="item.site_domain"
										:plugin="item"
									/>
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
	</div>
</template>

<style scoped>
.btn {
	gap: 8px;
}
</style>
