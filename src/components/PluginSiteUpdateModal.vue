<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useDataStore } from "../stores/data";
import { usePluginUpdatesStore } from "../stores/pluginUpdates";
import type { Plugin } from "../types";

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
const isUpdatingAll = ref(false);
const confirmMode = ref<"all" | "vulnerable" | null>(null);

const isAnyUpdating = computed(() =>
	Object.values(siteStatus.value).some((s) => s === "updating"),
);

const isPending = (u: UpdateEntry) => {
	const s = siteStatus.value[u.site_id];
	return s !== "success" && s !== "updating";
};

const hasUpdatesRemaining = computed(() =>
	updates.value.some(isPending),
);

const hasVulnerableUpdatesRemaining = computed(() =>
	updates.value.some((u) => u.isVulnerable && isPending(u)),
);

function snapshot() {
	const instances = dataStore.pluginsBySlugMap.get(props.pluginSlug) || [];
	const enriched = dataStore.enrichedPlugins.find(
		(p) => p.slug === props.pluginSlug,
	);
	const vulnerableSiteIds = new Set(
		enriched?.vulnerabilities.flatMap((v) => v.sites.map((s) => s.site_id)) ??
			[],
	);
	updates.value = instances
		.filter((p) => p.update !== "")
		.map((p) => ({
			...p,
			site_domain: dataStore.getSiteById(p.site_id)?.domain ?? "Unknown Site",
			isVulnerable: vulnerableSiteIds.has(p.site_id),
		}))
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
	siteStatus.value = {};
	siteError.value = {};
	confirmMode.value = null;
	isUpdatingAll.value = false;
}

async function updateSite(entry: UpdateEntry): Promise<void> {
	siteStatus.value[entry.site_id] = "updating";
	siteError.value[entry.site_id] = "";
	try {
		await pluginUpdatesStore.updatePlugin(entry.site_id, entry.name);
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
	await runUpdates(updates.value.filter((u) => u.isVulnerable && isPending(u)));
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
			<div class="modal-card">
				<header class="modal-header">
					<h3>Update Plugin on Sites</h3>
				</header>

				<div class="modal-body">
					<div
						v-if="updates.length === 0"
						class="center-state muted"
					>
						No updates available for this plugin.
					</div>

					<template v-else>
						<table class="update-table">
							<thead>
								<tr>
									<th>Site</th>
									<th>Version</th>
									<th class="hide-mobile">Vuln</th>
									<th></th>
								</tr>
							</thead>
							<tbody>
								<tr
									v-for="entry in updates"
									:key="entry.site_id"
								>
									<td class="site-name">
										{{ entry.site_domain }}
									</td>
									<td>
										<span class="version-col">
											<span class="version-old">{{
												entry.version
											}}</span>
											<span class="arrow">→</span>
											<span class="version-new">{{
												entry.update
											}}</span>
										</span>
									</td>
									<td class="hide-mobile">
										<span
											v-if="entry.isVulnerable"
											class="status-badge error"
										>
											Yes
										</span>
										<span v-else class="empty-dash">—</span>
									</td>
									<td class="action-col">
										<span
											v-if="
												siteStatus[entry.site_id] ===
												'success'
											"
											class="status-badge active"
										>
											Updated
										</span>
										<span
											v-else-if="
												siteStatus[entry.site_id] ===
												'error'
											"
											class="status-badge error"
											:title="siteError[entry.site_id]"
										>
											Failed
										</span>
										<span
											v-else-if="
												siteStatus[entry.site_id] ===
												'updating'
											"
											class="spinner spinner-sm"
										/>
										<button
											v-else
											class="btn btn-primary btn-sm"
											:disabled="
												isUpdatingAll || isAnyUpdating
											"
											@click="updateSite(entry)"
										>
											Update
										</button>
									</td>
								</tr>
							</tbody>
						</table>

						<div v-if="confirmMode" class="confirm-banner">
							<p v-if="confirmMode === 'all'">
								<strong>Are you sure?</strong> Updating the plugin
								on all sites should only be used as an emergency
								measure in case of vulnerabilities.
							</p>
							<p v-else>
								<strong>Are you sure?</strong> This will update
								the plugin on all sites with vulnerable versions.
							</p>
							<div class="confirm-actions">
								<button
									class="btn btn-cancel"
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
				</div>

				<footer class="modal-footer">
					<button class="btn btn-cancel" @click="emit('close')">
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

.confirm-banner {
	margin-top: 16px;
	background-color: var(--warning-bg, #fef3c7);
	border-left: 4px solid var(--warning-border, #f59e0b);
	color: var(--warning-text, #92400e);
	padding: 12px 14px;
	border-radius: 4px;
	margin-bottom: 16px;
}

.confirm-banner p {
	margin: 0 0 12px;
	font-size: 14px;
}

.confirm-actions {
	display: flex;
	gap: 8px;
	justify-content: flex-end;
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

.site-name {
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
	font-weight: 500;
	color: var(--text-main);
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

.btn-danger {
	padding: 8px 16px;
	border: none;
	border-radius: 4px;
	background-color: #ef4444;
	color: #fff;
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	transition: background-color 0.2s;
}

.btn-danger:hover {
	background-color: #dc2626;
}

.btn-danger:disabled {
	opacity: 0.7;
	cursor: not-allowed;
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
