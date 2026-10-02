<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useDataStore } from "../stores/data";
import { usePluginUpdatesStore } from "../stores/pluginUpdates";
import { usePluginUpdateJobsStore } from "../stores/pluginUpdateJobs";
import { useToastStore } from "../stores/toast";
import AppIcon from "./AppIcon.vue";
import type { Plugin, PluginUpdateResult } from "../types";

const props = defineProps<{
	visible: boolean;
	siteId: string;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const dataStore = useDataStore();
const pluginUpdatesStore = usePluginUpdatesStore();
const jobsStore = usePluginUpdateJobsStore();
const toastStore = useToastStore();

const isLoading = ref(false);
const updates = ref<Plugin[]>([]);
const fetchError = ref<string | null>(null);

type UpdateStatus = "idle" | "updating" | "success" | "error";
// Jobs queued from this modal since it was opened; only their results are
// shown, while spinners show every queued or running update on the site.
const myJobIds = ref(new Set<number>());
// Errors from queueing itself (the request failed before a job existed).
const queueError = ref<Record<string, string>>({});
// True while the enqueue request is in flight.
const isUpdatingAll = ref(false);

const pluginResult = computed(() => {
	const map: Record<string, PluginUpdateResult | null> = {};
	for (const p of updates.value) {
		map[p.name] =
			jobsStore.resultFor(props.siteId, p.name, myJobIds.value) ?? null;
	}
	return map;
});

const pluginStatus = computed(() => {
	const map: Record<string, UpdateStatus> = {};
	for (const p of updates.value) {
		const result = pluginResult.value[p.name];
		if (jobsStore.isUpdating(props.siteId, p.name)) {
			map[p.name] = "updating";
		} else if (result) {
			map[p.name] = result.status === "failed" ? "error" : "success";
		} else {
			map[p.name] = queueError.value[p.name] ? "error" : "idle";
		}
	}
	return map;
});

const pluginError = computed(() => {
	const map: Record<string, string> = {};
	for (const p of updates.value) {
		map[p.name] =
			pluginResult.value[p.name]?.error ?? queueError.value[p.name] ?? "";
	}
	return map;
});

const isAnyUpdating = computed(() =>
	Object.values(pluginStatus.value).some((s) => s === "updating"),
);

const isPending = (p: Plugin) => {
	const s = pluginStatus.value[p.name];
	return s !== "success" && s !== "updating";
};

const hasUpdatesRemaining = computed(() => updates.value.some(isPending));

const vulnerablePluginNames = computed(() => {
	const vulns = dataStore.vulnerabilitiesBySiteId.get(props.siteId) || [];
	return new Set(
		vulns
			.filter((v) => {
				const siteSpecificVuln = v.sites.find(
					(s) => s.site_id === props.siteId,
				);
				return !(
					v.plugin_suppressed ||
					v.suppressed ||
					siteSpecificVuln?.suppressed
				);
			})
			.map((v) => v.plugin_name),
	);
});

const vulnerablePluginSlugs = computed(() => {
	const vulns = dataStore.vulnerabilitiesBySiteId.get(props.siteId) || [];
	return new Set(
		vulns
			.filter((v) => {
				const siteSpecificVuln = v.sites.find(
					(s) => s.site_id === props.siteId,
				);
				return !(
					v.plugin_suppressed ||
					v.suppressed ||
					siteSpecificVuln?.suppressed
				);
			})
			.map((v) => v.slug),
	);
});

const isVulnerable = (plugin: Plugin) =>
	vulnerablePluginNames.value.has(plugin.name) ||
	vulnerablePluginSlugs.value.has(plugin.slug || plugin.name);

const hasVulnerableUpdatesRemaining = computed(() =>
	updates.value.some((p) => isVulnerable(p) && isPending(p)),
);

async function fetchUpdates() {
	isLoading.value = true;
	fetchError.value = null;
	updates.value = [];
	myJobIds.value = new Set();
	queueError.value = {};

	try {
		updates.value = await pluginUpdatesStore.fetchPluginUpdates(
			props.siteId,
		);
	} catch (e: any) {
		fetchError.value = e.message || "Failed to fetch plugin updates";
	} finally {
		isLoading.value = false;
	}
}

/**
 * Queues one background job updating all the given plugins in a single
 * WP-CLI call. jman-api writes the update ledger entry when it finishes.
 */
async function runUpdates(names: string[]) {
	if (names.length === 0) return;
	isUpdatingAll.value = true;
	for (const name of names) delete queueError.value[name];
	try {
		const jobs = await jobsStore.enqueue([
			{ site_id: props.siteId, plugins: names },
		]);
		myJobIds.value = new Set([...myJobIds.value, ...jobs.map((j) => j.id)]);
	} catch (e: any) {
		const message = e.message || "Failed to queue update";
		for (const name of names) queueError.value[name] = message;
		toastStore.addToast(
			`Failed to queue plugin update: ${message}`,
			"error",
		);
	} finally {
		isUpdatingAll.value = false;
	}
}

async function updatePlugin(pluginName: string) {
	await runUpdates([pluginName]);
}

async function updateAll() {
	await runUpdates(updates.value.filter(isPending).map((p) => p.name));
}

async function updateVulnerable() {
	await runUpdates(
		updates.value
			.filter((p) => isVulnerable(p) && isPending(p))
			.map((p) => p.name),
	);
}

watch(
	() => props.visible,
	(val) => {
		if (val) fetchUpdates();
	},
);
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card">
				<header class="modal-header">
					<h2>Plugin Updates</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div class="content">
					<div v-if="isLoading" class="loading-state">
						<span class="spinner mb-4" />
						<p>Checking for updates…</p>
					</div>

					<div v-else-if="fetchError" class="error-banner">
						<p>{{ fetchError }}</p>
					</div>

					<div v-else-if="updates.length === 0" class="loading-state">
						<p>All plugins are up to date.</p>
					</div>

					<div v-else class="table-container">
						<table class="data-table">
							<thead>
								<tr>
									<th>Plugin</th>
									<th>Version</th>
									<th>Vuln</th>
									<th class="text-right">Action</th>
								</tr>
							</thead>
							<tbody>
								<tr
									v-for="plugin in updates"
									:key="plugin.name"
								>
									<td class="font-medium">
										{{ plugin.name }}
									</td>
									<td>
										<div class="version-display">
											<span class="text-muted font-xs">{{
												plugin.version
											}}</span>
											<span class="text-muted px-2"
												>→</span
											>
											<span class="font-medium">{{
												plugin.update
											}}</span>
										</div>
									</td>
									<td>
										<span
											v-if="isVulnerable(plugin)"
											class="status-badge error badge-sm"
										>
											Yes
										</span>
										<span v-else class="text-muted">—</span>
									</td>
									<td class="text-right">
										<span
											v-if="
												pluginStatus[plugin.name] ===
												'success'
											"
											:class="[
												'status-badge',
												'badge-sm',
												pluginResult[plugin.name] &&
												!pluginResult[
													plugin.name
												]?.status
													.toLowerCase()
													.includes('up to date')
													? 'active'
													: 'warning',
											]"
										>
											{{
												pluginResult[plugin.name]
													? pluginResult[plugin.name]!
															.status
													: "Up to date"
											}}
										</span>
										<span
											v-else-if="
												pluginStatus[plugin.name] ===
												'error'
											"
											class="status-badge error badge-sm"
											:title="pluginError[plugin.name]"
										>
											Failed
										</span>
										<span
											v-else-if="
												pluginStatus[plugin.name] ===
												'updating'
											"
											class="spinner spinner-small"
										/>
										<button
											v-else
											class="btn btn-primary btn-sm"
											:disabled="
												isUpdatingAll || isAnyUpdating
											"
											@click="updatePlugin(plugin.name)"
										>
											Update
										</button>
									</td>
								</tr>
							</tbody>
						</table>
					</div>

					<footer class="form-actions mt-4">
						<button class="btn btn-outline" @click="emit('close')">
							Close
						</button>
						<button
							v-if="hasVulnerableUpdatesRemaining"
							class="btn btn-danger"
							:disabled="isUpdatingAll || isAnyUpdating"
							@click="updateVulnerable"
						>
							Update Vulnerable
						</button>
						<button
							v-if="updates.length > 0"
							class="btn btn-primary"
							:disabled="
								isUpdatingAll ||
								isAnyUpdating ||
								isLoading ||
								!hasUpdatesRemaining
							"
							@click="updateAll"
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

<style scoped></style>
