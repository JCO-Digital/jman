<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useDataStore } from "../stores/data";
import { usePluginUpdatesStore } from "../stores/pluginUpdates";
import type { Plugin, PluginUpdateResult } from "../types";

interface UpdateEntry extends Plugin {
	site_domain: string;
	isVulnerable: boolean;
}

const props = defineProps<{
	visible: boolean;
	pluginSlug: string;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const dataStore = useDataStore();
const pluginUpdatesStore = usePluginUpdatesStore();

const updates = ref<UpdateEntry[]>([]);

type UpdateStatus = "idle" | "updating" | "success" | "error";
const siteStatus = ref<Record<number, UpdateStatus>>({});
const siteError = ref<Record<number, string>>({});
const siteResult = ref<Record<number, PluginUpdateResult | null>>({});
const isUpdatingAll = ref(false);
const confirmMode = ref<"all" | "vulnerable" | null>(null);

const isAnyUpdating = computed(() =>
	Object.values(siteStatus.value).some((s) => s === "updating"),
);

const isPending = (u: UpdateEntry) => {
	const s = siteStatus.value[u.site_id];
	return s !== "success" && s !== "updating";
};

const hasUpdatesRemaining = computed(() => updates.value.some(isPending));

const hasVulnerableUpdatesRemaining = computed(() =>
	updates.value.some((u) => u.isVulnerable && isPending(u)),
);

function snapshot() {
	const instances = dataStore.pluginsBySlugMap.get(props.pluginSlug) || [];
	const enriched = dataStore.enrichedPlugins.find(
		(p) => p.slug === props.pluginSlug,
	);
	const vulnerableSiteIds = new Set(
		enriched?.vulnerabilities.flatMap((v) =>
			v.sites.map((s) => s.site_id),
		) ?? [],
	);
	updates.value = instances
		.filter((p) => p.update !== "")
		.map((p) => ({
			...p,
			site_domain:
				dataStore.getSiteById(p.site_id)?.domain ?? "Unknown Site",
			isVulnerable: vulnerableSiteIds.has(p.site_id),
		}))
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
	siteStatus.value = {};
	siteError.value = {};
	siteResult.value = {};
	confirmMode.value = null;
	isUpdatingAll.value = false;
}

async function updateSite(entry: UpdateEntry): Promise<void> {
	siteStatus.value[entry.site_id] = "updating";
	siteError.value[entry.site_id] = "";
	try {
		const result = await pluginUpdatesStore.updatePlugin(
			entry.site_id,
			entry.name,
		);
		siteResult.value[entry.site_id] = result;
		siteStatus.value[entry.site_id] = "success";
	} catch (e: any) {
		siteStatus.value[entry.site_id] = "error";
		siteError.value[entry.site_id] = e.message || "Update failed";
	}
}

async function runUpdates(entries: UpdateEntry[]) {
	confirmMode.value = null;
	isUpdatingAll.value = true;
	for (const entry of entries) {
		await updateSite(entry);
	}
	isUpdatingAll.value = false;
}

async function updateAll() {
	await runUpdates(updates.value.filter(isPending));
}

async function updateVulnerable() {
	await runUpdates(
		updates.value.filter((u) => u.isVulnerable && isPending(u)),
	);
}

watch(
	() => props.visible,
	(val) => {
		if (val) snapshot();
	},
);
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card">
				<header class="modal-header">
					<h2>Update Plugin on Sites</h2>
					<button class="modal-close" @click="emit('close')">
						&times;
					</button>
				</header>

				<div class="content">
					<div v-if="updates.length === 0" class="loading-state">
						<p class="text-muted">
							No updates available for this plugin.
						</p>
					</div>

					<template v-else>
						<div class="table-container">
							<table class="data-table">
								<thead>
									<tr>
										<th>Site</th>
										<th>Version</th>
										<th class="hide-mobile">Vuln</th>
										<th class="text-right">Action</th>
									</tr>
								</thead>
								<tbody>
									<tr
										v-for="entry in updates"
										:key="entry.site_id"
									>
										<td class="font-medium text-main">
											{{ entry.site_domain }}
										</td>
										<td>
											<div class="version-display">
												<span
													class="text-muted font-xs"
													>{{ entry.version }}</span
												>
												<span class="text-muted px-2"
													>→</span
												>
												<span class="font-medium">{{
													entry.update
												}}</span>
											</div>
										</td>
										<td class="hide-mobile">
											<span
												v-if="entry.isVulnerable"
												class="status-badge error badge-sm"
											>
												Yes
											</span>
											<span v-else class="text-muted"
												>—</span
											>
										</td>
										<td class="text-right">
											<span
												v-if="
													siteStatus[
														entry.site_id
													] === 'success'
												"
												:class="[
													'status-badge',
													'badge-sm',
													siteResult[entry.site_id]
														? 'active'
														: 'default',
												]"
											>
												{{
													siteResult[entry.site_id]
														? siteResult[
																entry.site_id
															]!.status
														: "Up to date"
												}}
											</span>
											<span
												v-else-if="
													siteStatus[
														entry.site_id
													] === 'error'
												"
												class="status-badge error badge-sm"
												:title="
													siteError[entry.site_id]
												"
											>
												Failed
											</span>
											<span
												v-else-if="
													siteStatus[
														entry.site_id
													] === 'updating'
												"
												class="spinner spinner-small"
											/>
											<button
												v-else
												class="btn btn-primary btn-sm"
												:disabled="
													isUpdatingAll ||
													isAnyUpdating
												"
												@click="updateSite(entry)"
											>
												Update
											</button>
										</td>
									</tr>
								</tbody>
							</table>
						</div>

						<div v-if="confirmMode" class="confirm-banner">
							<p v-if="confirmMode === 'all'">
								<strong>Are you sure?</strong> Updating the
								plugin on all sites should only be used as an
								emergency measure in case of vulnerabilities.
							</p>
							<p v-else>
								<strong>Are you sure?</strong> This will update
								the plugin on all sites with vulnerable
								versions.
							</p>
							<div class="confirm-actions">
								<button
									class="btn btn-outline"
									@click="confirmMode = null"
								>
									Cancel
								</button>
								<button
									class="btn btn-danger"
									@click="
										confirmMode === 'all'
											? updateAll()
											: updateVulnerable()
									"
								>
									{{
										confirmMode === "all"
											? "Confirm Update All"
											: "Confirm Update Vulnerable"
									}}
								</button>
							</div>
						</div>
					</template>

					<footer class="form-actions mt-4">
						<button class="btn btn-outline" @click="emit('close')">
							Close
						</button>
						<button
							v-if="hasVulnerableUpdatesRemaining"
							class="btn btn-danger"
							:disabled="
								isUpdatingAll ||
								isAnyUpdating ||
								confirmMode !== null
							"
							@click="confirmMode = 'vulnerable'"
						>
							Update Vulnerable
						</button>
						<button
							v-if="updates.length > 0"
							class="btn btn-danger"
							:disabled="
								isUpdatingAll ||
								isAnyUpdating ||
								!hasUpdatesRemaining ||
								confirmMode !== null
							"
							@click="confirmMode = 'all'"
						>
							{{ isUpdatingAll ? "Updating…" : "Update All" }}
						</button>
					</footer>
				</div>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
/* Scoped styles removed in favor of global modal, table, and confirmation classes in components.css */
</style>
