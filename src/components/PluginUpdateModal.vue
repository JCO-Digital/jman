<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useDataStore } from "../stores/data";
import { usePluginUpdatesStore } from "../stores/pluginUpdates";
import { useAuthStore } from "../stores/auth";
import { BASE_URL } from "../utils/api";
import AppIcon from "./AppIcon.vue";
import type { Plugin, PluginUpdateResult } from "../types";

const props = defineProps<{
	visible: boolean;
	siteId: number;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const dataStore = useDataStore();
const pluginUpdatesStore = usePluginUpdatesStore();

const isLoading = ref(false);
const updates = ref<Plugin[]>([]);
const fetchError = ref<string | null>(null);

type UpdateStatus = "idle" | "updating" | "success" | "error";
const pluginStatus = ref<Record<string, UpdateStatus>>({});
const pluginError = ref<Record<string, string>>({});
const pluginResult = ref<Record<string, PluginUpdateResult | null>>({});
const isUpdatingAll = ref(false);

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
	pluginStatus.value = {};
	pluginError.value = {};
	pluginResult.value = {};

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

async function updatePlugin(
	pluginName: string,
	skipLedger?: boolean,
): Promise<boolean> {
	pluginStatus.value[pluginName] = "updating";
	pluginError.value[pluginName] = "";

	try {
		const result = await pluginUpdatesStore.updatePlugin(
			props.siteId,
			pluginName,
			skipLedger,
		);
		pluginResult.value[pluginName] = result;
		pluginStatus.value[pluginName] = "success";
		return true;
	} catch (e: any) {
		pluginStatus.value[pluginName] = "error";
		pluginError.value[pluginName] = e.message || "Update failed";
		return false;
	}
}

async function runUpdates(entries: Plugin[]) {
	isUpdatingAll.value = true;

	const totalAvailableBefore = updates.value.filter(isPending).length;

	const attempted: {
		name: string;
		oldVersion: string;
		newVersion?: string;
		error?: string;
		success: boolean;
	}[] = [];

	for (const plugin of entries) {
		await updatePlugin(plugin.name, true);

		const status = pluginStatus.value[plugin.name];
		const res = pluginResult.value[plugin.name];
		const errMessage = pluginError.value[plugin.name];

		attempted.push({
			name: plugin.name,
			oldVersion: plugin.version,
			newVersion:
				status === "success" && res ? res.new_version : plugin.version,
			error: status === "error" ? errMessage : undefined,
			success: status === "success",
		});
	}

	if (attempted.length > 0) {
		const successes = attempted.filter((a) => a.success);
		const failures = attempted.filter((a) => !a.success);

		let status: "full" | "partial" | "failed" = "failed";
		if (failures.length > 0) {
			status = "failed";
		} else if (successes.length === totalAvailableBefore) {
			status = "full";
		} else {
			status = "partial";
		}

		const ledgerData = {
			updates: attempted.map((a) => ({
				plugin: a.name,
				old_version: a.oldVersion,
				new_version: a.newVersion,
				status: a.success ? "success" : "failed",
				error: a.error,
			})),
			summary: `Bulk update of ${attempted.length} plugin(s): ${successes.length} succeeded, ${failures.length} failed.`,
		};

		try {
			const authStore = useAuthStore();
			await fetch(`${BASE_URL}/sites/${props.siteId}/update-ledger`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...authStore.authHeader,
				},
				body: JSON.stringify({
					update_type: "plugin",
					status: status,
					data_json: JSON.stringify(ledgerData),
				}),
			});
		} catch (e) {
			console.error("Failed to write bulk update ledger entry", e);
		}
	}

	isUpdatingAll.value = false;
}

async function updateAll() {
	await runUpdates(updates.value.filter(isPending));
}

async function updateVulnerable() {
	await runUpdates(
		updates.value.filter((p) => isVulnerable(p) && isPending(p)),
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
									<th class="hide-mobile">Vuln</th>
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
									<td class="hide-mobile">
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
							{{ isUpdatingAll ? "Updating…" : "Update All" }}
						</button>
					</footer>
				</div>
			</div>
		</div>
	</Teleport>
</template>

<style scoped></style>
