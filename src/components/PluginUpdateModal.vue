<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { usePluginUpdatesStore } from "../stores/pluginUpdates";
import type { Plugin, PluginUpdateResult } from "../types";

const props = defineProps<{
	visible: boolean;
	siteId: number;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

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
const hasUpdatesRemaining = computed(() =>
	updates.value.some((p) => {
		const s = pluginStatus.value[p.name];
		return s !== "success" && s !== "updating";
	}),
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

async function updatePlugin(pluginName: string): Promise<boolean> {
	pluginStatus.value[pluginName] = "updating";
	pluginError.value[pluginName] = "";

	try {
		const result = await pluginUpdatesStore.updatePlugin(
			props.siteId,
			pluginName,
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

async function updateAll() {
	isUpdatingAll.value = true;
	const pending = updates.value.filter((p) => {
		const s = pluginStatus.value[p.name];
		return s !== "success" && s !== "updating";
	});
	for (const plugin of pending) {
		await updatePlugin(plugin.name);
	}
	isUpdatingAll.value = false;
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
						&times;
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
									<td class="text-right">
										<span
											v-if="
												pluginStatus[plugin.name] ===
												'success'
											"
											:class="[
												'status-badge',
												'badge-sm',
												pluginResult[plugin.name]
													? 'active'
													: 'default',
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
											:disabled="isUpdatingAll"
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

<style scoped>
.version-display {
	display: flex;
	align-items: center;
	white-space: nowrap;
}
</style>
