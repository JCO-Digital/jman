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
const pluginResult = ref<Record<string, PluginUpdateResult>>({});
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
		updates.value = await pluginUpdatesStore.fetchPluginUpdates(props.siteId);
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
		<div
			v-if="visible"
			class="modal-overlay"
			@click.self="emit('close')"
		>
			<div class="modal-card">
				<header class="modal-header">
					<h3>Plugin Updates</h3>
				</header>

				<div class="modal-body">
					<div v-if="isLoading" class="center-state">
						<span class="spinner" />
						<span>Checking for updates…</span>
					</div>

					<div v-else-if="fetchError" class="error-banner">
						<p>{{ fetchError }}</p>
					</div>

					<div v-else-if="updates.length === 0" class="center-state muted">
						All plugins are up to date.
					</div>

					<table v-else class="update-table">
						<thead>
							<tr>
								<th>Plugin</th>
								<th>Version</th>
								<th></th>
							</tr>
						</thead>
						<tbody>
							<tr v-for="plugin in updates" :key="plugin.name">
								<td class="plugin-name">{{ plugin.name }}</td>
								<td>
									<span class="version-col">
										<span class="version-old">{{ plugin.version }}</span>
										<span class="arrow">→</span>
										<span class="version-new">{{ plugin.update }}</span>
									</span>
								</td>
								<td class="action-col">
									<span
										v-if="pluginStatus[plugin.name] === 'success'"
										class="status-badge active"
									>
										{{ pluginResult[plugin.name]?.status || "Updated" }}
									</span>
									<span
										v-else-if="pluginStatus[plugin.name] === 'error'"
										class="status-badge error"
										:title="pluginError[plugin.name]"
									>
										Failed
									</span>
									<span
										v-else-if="pluginStatus[plugin.name] === 'updating'"
										class="spinner spinner-sm"
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

				<footer class="modal-footer">
					<button class="btn btn-cancel" @click="emit('close')">
						Close
					</button>
					<button
						v-if="updates.length > 0"
						class="btn btn-primary"
						:disabled="isUpdatingAll || isAnyUpdating || isLoading || !hasUpdatesRemaining"
						@click="updateAll"
					>
						{{ isUpdatingAll ? "Updating…" : "Update All" }}
					</button>
				</footer>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.modal-overlay {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background: rgba(0, 0, 0, 0.5);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 1000;
	padding: 16px;
}

.modal-card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 8px;
	width: 100%;
	max-width: 640px;
	max-height: 90vh;
	overflow-y: auto;
	display: flex;
	flex-direction: column;
}

.modal-header {
	padding: 20px 24px 16px;
	border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
	margin: 0;
	font-size: 18px;
	color: var(--text-heading);
}

.modal-body {
	padding: 20px 24px;
	flex: 1;
}

.modal-footer {
	padding: 16px 24px 20px;
	border-top: 1px solid var(--border-color);
	display: flex;
	justify-content: flex-end;
	gap: 12px;
}

.center-state {
	display: flex;
	align-items: center;
	gap: 12px;
	padding: 24px 0;
}

.center-state.muted {
	color: var(--text-muted);
}

.error-banner {
	background-color: var(--error-bg);
	border-left: 4px solid var(--error-border);
	color: var(--error-text);
	padding: 10px 14px;
	border-radius: 4px;
}

.error-banner p {
	margin: 0;
	font-size: 14px;
}

.update-table {
	width: 100%;
	border-collapse: collapse;
	font-size: 14px;
}

.update-table th {
	text-align: left;
	padding: 8px 12px;
	font-size: 12px;
	font-weight: 600;
	color: var(--text-muted);
	border-bottom: 1px solid var(--border-color);
}

.update-table td {
	padding: 10px 12px;
	border-bottom: 1px solid var(--border-color);
	vertical-align: middle;
}

.update-table tr:last-child td {
	border-bottom: none;
}

.plugin-name {
	font-weight: 500;
	color: var(--text-main);
}

.version-col {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	white-space: nowrap;
}

.version-old {
	color: var(--text-muted);
}

.arrow {
	color: var(--text-muted);
}

.version-new {
	color: var(--text-main);
	font-weight: 500;
}

.action-col {
	text-align: right;
	white-space: nowrap;
}

.btn-cancel {
	padding: 8px 16px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	background: var(--bg-card);
	color: var(--text-main);
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	transition: background-color 0.2s;
}

.btn-cancel:hover {
	background-color: var(--bg-hover);
}

.btn-sm {
	padding: 4px 12px;
	font-size: 13px;
}

.spinner {
	display: inline-block;
	width: 20px;
	height: 20px;
	border: 2px solid var(--border-color);
	border-top-color: var(--primary);
	border-radius: 50%;
	animation: spin 0.7s linear infinite;
}

.spinner-sm {
	width: 16px;
	height: 16px;
}

@keyframes spin {
	to {
		transform: rotate(360deg);
	}
}
</style>
