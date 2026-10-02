<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useDataStore } from "../stores/data";
import { usePluginUpdateJobsStore } from "../stores/pluginUpdateJobs";
import { useToastStore } from "../stores/toast";
import AppIcon from "./AppIcon.vue";
import type { Plugin, PluginUpdateResult } from "../types";

interface UpdateEntry extends Plugin {
	site_domain: string;
	isVulnerable: boolean;
	/** Plugin updates run over WP-CLI; sites without it can't be updated here. */
	canUpdate: boolean;
}

const props = defineProps<{
	visible: boolean;
	pluginSlug: string;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const dataStore = useDataStore();
const jobsStore = usePluginUpdateJobsStore();
const toastStore = useToastStore();

const updates = ref<UpdateEntry[]>([]);

type UpdateStatus = "idle" | "updating" | "success" | "error";
// Jobs queued from this modal since it was opened; only their results are
// shown, while spinners show every queued or running update of the plugin.
const myJobIds = ref(new Set<number>());
// Errors from queueing itself (the request failed before a job existed).
const queueError = ref<Record<string, string>>({});
// True while the enqueue request is in flight.
const isUpdatingAll = ref(false);
const confirmMode = ref<"all" | "vulnerable" | null>(null);

const siteResult = computed(() => {
	const map: Record<string, PluginUpdateResult | null> = {};
	for (const u of updates.value) {
		map[u.site_id] =
			jobsStore.resultFor(u.site_id, u.name, myJobIds.value) ?? null;
	}
	return map;
});

const siteStatus = computed(() => {
	const map: Record<string, UpdateStatus> = {};
	for (const u of updates.value) {
		const result = siteResult.value[u.site_id];
		if (jobsStore.isUpdating(u.site_id, u.name)) {
			map[u.site_id] = "updating";
		} else if (result) {
			map[u.site_id] = result.status === "failed" ? "error" : "success";
		} else {
			map[u.site_id] = queueError.value[u.site_id] ? "error" : "idle";
		}
	}
	return map;
});

const siteError = computed(() => {
	const map: Record<string, string> = {};
	for (const u of updates.value) {
		map[u.site_id] =
			siteResult.value[u.site_id]?.error ??
			queueError.value[u.site_id] ??
			"";
	}
	return map;
});

const isAnyUpdating = computed(() =>
	Object.values(siteStatus.value).some((s) => s === "updating"),
);

const isPending = (u: UpdateEntry) => {
	if (!u.canUpdate) return false;
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
		.map((p) => {
			const site = dataStore.getSiteById(p.site_id);
			return {
				...p,
				site_domain: site?.domain ?? "Unknown Site",
				isVulnerable: vulnerableSiteIds.has(p.site_id),
				canUpdate: !!site?.can_wp_cli,
			};
		})
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
	myJobIds.value = new Set();
	queueError.value = {};
	confirmMode.value = null;
	isUpdatingAll.value = false;
}

/**
 * Queues one background job per site. Sites update concurrently on the
 * server; jman-api writes each site's update ledger entry.
 */
async function runUpdates(entries: UpdateEntry[]) {
	confirmMode.value = null;
	if (entries.length === 0) return;
	isUpdatingAll.value = true;
	for (const e of entries) delete queueError.value[e.site_id];
	try {
		const jobs = await jobsStore.enqueue(
			entries.map((e) => ({ site_id: e.site_id, plugins: [e.name] })),
		);
		myJobIds.value = new Set([...myJobIds.value, ...jobs.map((j) => j.id)]);
	} catch (e: any) {
		const message = e.message || "Failed to queue update";
		for (const entry of entries) queueError.value[entry.site_id] = message;
		toastStore.addToast(
			`Failed to queue plugin update: ${message}`,
			"error",
		);
	} finally {
		isUpdatingAll.value = false;
	}
}

async function updateSite(entry: UpdateEntry) {
	await runUpdates([entry]);
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
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div class="content">
					<div v-if="updates.length === 0" class="loading-state">
						<p class="text-muted">
							This plugin is not installed on any sites.
						</p>
					</div>

					<template v-else>
						<div class="table-container">
							<table class="data-table">
								<thead>
									<tr>
										<th>Site</th>
										<th>Version</th>
										<th>Vuln</th>
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
												<template v-if="entry.update">
													<span
														class="text-muted px-2"
														>→</span
													>
													<span class="font-medium">{{
														entry.update
													}}</span>
												</template>
											</div>
										</td>
										<td>
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
													siteResult[entry.site_id] &&
													!siteResult[
														entry.site_id
													]?.status
														.toLowerCase()
														.includes('up to date')
														? 'active'
														: 'warning',
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
											<span
												v-else-if="!entry.canUpdate"
												class="text-muted font-xs"
												title="This site has no WP-CLI access"
											>
												No WP-CLI
											</span>
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
							{{
								isUpdatingAll || isAnyUpdating
									? "Updating…"
									: "Update All"
							}}
						</button>
					</footer>
					<p v-if="isAnyUpdating" class="text-muted font-sm mt-2">
						Updates run in the background on the server; you can
						close this window.
					</p>
				</div>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
/* Scoped styles removed in favor of global modal, table, and confirmation classes in components.css */
</style>
