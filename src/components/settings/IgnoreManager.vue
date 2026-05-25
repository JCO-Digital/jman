<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useIgnoreStore } from "../../stores/ignore";
import { useDataStore } from "../../stores/data";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import AppIcon from "../AppIcon.vue";
import LoadingSpinner from "../LoadingSpinner.vue";
import SearchableSelect from "../SearchableSelect.vue";
import type { IgnoreType, IgnoreEntry, CreateIgnorePayload, Site } from "../../types";
import { useConfirm } from "../../composables/useConfirm";

const ignoreStore = useIgnoreStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const isSubmitting = ref(false);
const showAddForm = ref(false);
const editingEntryId = ref<number | null>(null);
const negatedSitesSearch = ref("");

const newEntry = ref<CreateIgnorePayload>({
	type: "site",
	target: "",
	reason: "",
	use_for_monitor: true,
	use_for_vuln: true,
	negated_site_ids: [],
});

const isSubmitDisabled = computed(() => {
	if (!newEntry.value.target) return true;
	if (!newEntry.value.use_for_monitor && !newEntry.value.use_for_vuln)
		return true;
	return isSubmitting.value;
});

const sortedSites = computed(() => {
	return [...dataStore.sites].sort((a, b) =>
		a.domain.localeCompare(b.domain),
	);
});

const sortedServers = computed(() => {
	return [...dataStore.servers].sort((a, b) => a.name.localeCompare(b.name));
});

const siteOptions = computed(() => {
	return sortedSites.value.map((s) => ({
		value: s.id.toString(),
		label: s.domain,
	}));
});

const serverOptions = computed(() => {
	return sortedServers.value.map((s) => ({
		value: s.id.toString(),
		label: s.name,
	}));
});

const pluginOptions = computed(() => {
	return [...dataStore.enrichedPlugins]
		.sort((a, b) => a.name.localeCompare(b.name))
		.map((p) => ({
			value: p.slug,
			label: p.name,
		}));
});

const sitesOnSelectedServer = computed(() => {
	if (newEntry.value.type !== "server" || !newEntry.value.target) return [];
	const serverId = parseInt(newEntry.value.target);
	return sortedSites.value.filter((s) => s.server_id === serverId);
});

const filteredNegatedSites = computed(() => {
	let available = sitesOnSelectedServer.value;

	// Filter out already selected sites
	if (
		newEntry.value.negated_site_ids &&
		newEntry.value.negated_site_ids.length > 0
	) {
		available = available.filter(
			(s) => !newEntry.value.negated_site_ids?.includes(s.id),
		);
	}

	if (!negatedSitesSearch.value) return available;
	const q = negatedSitesSearch.value.toLowerCase();
	return available.filter((s) => s.domain.toLowerCase().includes(q));
});

const selectedNegatedSites = computed(() => {
	if (!newEntry.value.negated_site_ids) return [];
	return newEntry.value.negated_site_ids
		.map((id) => dataStore.getSiteById(id))
		.filter((s): s is Site => s !== undefined)
		.sort((a, b) => a.domain.localeCompare(b.domain));
});

const resetForm = () => {
	newEntry.value = {
		type: "site",
		target: "",
		reason: "",
		use_for_monitor: true,
		use_for_vuln: true,
		negated_site_ids: [],
	};
	showAddForm.value = false;
	editingEntryId.value = null;
	negatedSitesSearch.value = "";
};

onMounted(() => {
	ignoreStore.fetchIgnoreEntries();
	dataStore.initData();
});

watch(
	() => [newEntry.value.type, newEntry.value.target],
	(newVal, oldVal) => {
		// Only clear if we are not in edit mode
		// and the values actually changed (not just initialization)
		if (editingEntryId.value) return;
		if (oldVal && newVal[0] === oldVal[0] && newVal[1] === oldVal[1])
			return;

		newEntry.value.negated_site_ids = [];
		negatedSitesSearch.value = "";

		// Disable monitor if type is plugin or vulnerability
		if (
			newEntry.value.type === "plugin" ||
			newEntry.value.type === "vulnerability"
		) {
			newEntry.value.use_for_monitor = false;
		}
	},
);

const toggleNegatedSite = (id: number) => {
	if (!newEntry.value.negated_site_ids) {
		newEntry.value.negated_site_ids = [id];
		return;
	}
	const index = newEntry.value.negated_site_ids.indexOf(id);
	if (index === -1) {
		newEntry.value.negated_site_ids.push(id);
	} else {
		newEntry.value.negated_site_ids.splice(index, 1);
	}
};

const handleAddEntry = async () => {
	if (!newEntry.value.target) return;

	isSubmitting.value = true;
	try {
		if (editingEntryId.value) {
			await ignoreStore.updateIgnoreEntry(
				editingEntryId.value,
				newEntry.value,
			);
			toast.addToast("Ignore entry updated", "success");
		} else {
			await ignoreStore.addIgnoreEntry(newEntry.value);
			toast.addToast("Ignore entry added", "success");
		}
		resetForm();
	} catch (e: any) {
		toast.addToast(e.message || "Failed to save ignore entry", "error");
	} finally {
		isSubmitting.value = false;
	}
};

const handleEditEntry = (entry: IgnoreEntry) => {
	editingEntryId.value = entry.id;
	newEntry.value = {
		type: entry.type,
		target: entry.target,
		reason: entry.reason,
		use_for_monitor: entry.use_for_monitor,
		use_for_vuln: entry.use_for_vuln,
		negated_site_ids: entry.negated_site_ids
			? [...entry.negated_site_ids]
			: [],
	};
	showAddForm.value = true;
	window.scrollTo({ top: 0, behavior: "smooth" });
};

const handleRemoveEntry = async (id: number) => {
	if (!await confirm("Are you sure you want to remove this ignore rule?", { danger: true })) return;

	try {
		await ignoreStore.deleteIgnoreEntry(id);
		toast.addToast("Ignore entry removed", "success");
	} catch (e: any) {
		toast.addToast(e.message || "Failed to remove entry", "error");
	}
};

const resolveTargetName = (type: IgnoreType, target: string) => {
	if (type === "site") {
		const site = dataStore.getSiteById(parseInt(target));
		return site ? site.domain : `Site ${target}`;
	}
	if (type === "server") {
		const server = dataStore.getServerById(parseInt(target));
		return server ? server.name : `Server ${target}`;
	}
	if (type === "vulnerability" && target.length > 12) {
		return target.substring(0, 12) + "...";
	}
	return target;
};
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Ignore List</h2>
			<button
				v-if="authStore.canEdit && !showAddForm"
				class="btn btn-primary btn-sm"
				@click="showAddForm = true"
			>
				Add Entry
			</button>
		</div>
		<p class="sub-text mb-4">
			Manage exclusions for uptime monitoring and vulnerability scanning.
		</p>

		<!-- Add form -->
		<div v-if="showAddForm" class="card-muted mb-4">
			<div class="flex-row justify-between items-center mb-3">
				<h3 class="font-medium">
					{{
						editingEntryId
							? "Edit Ignore Rule"
							: "Add New Ignore Rule"
					}}
				</h3>
			</div>

			<form @submit.prevent="handleAddEntry">
				<div class="grid-2-cols gap-4">
					<div class="form-group">
						<label>Type</label>
						<select
							v-model="newEntry.type"
							class="w-full"
							:disabled="!!editingEntryId"
						>
							<option value="site">Site</option>
							<option value="server">Server</option>
							<option value="plugin">Plugin (slug)</option>
							<option value="vulnerability">
								Vulnerability (UUID)
							</option>
						</select>
					</div>

					<div class="form-group">
						<label>Target</label>
						<SearchableSelect
							v-if="newEntry.type === 'site'"
							v-model="newEntry.target"
							:options="siteOptions"
							placeholder="Select Site"
							:disabled="!!editingEntryId"
						/>
						<SearchableSelect
							v-else-if="newEntry.type === 'server'"
							v-model="newEntry.target"
							:options="serverOptions"
							placeholder="Select Server"
							:disabled="!!editingEntryId"
						/>
						<SearchableSelect
							v-else-if="newEntry.type === 'plugin'"
							v-model="newEntry.target"
							:options="pluginOptions"
							placeholder="Select Plugin"
							:disabled="!!editingEntryId"
						/>
						<input
							v-else
							v-model="newEntry.target"
							type="text"
							placeholder="slug or UUID"
							required
							class="w-full"
							:disabled="!!editingEntryId"
						/>
					</div>

					<div class="form-group col-span-2">
						<label>Reason</label>
						<input
							v-model="newEntry.reason"
							type="text"
							placeholder="Why is this ignored?"
							class="w-full"
						/>
					</div>

					<div
						v-if="newEntry.type === 'server'"
						class="form-group col-span-2"
					>
						<label>Negated Site IDs (Sites to NOT ignore)</label>

						<!-- Selected Pills -->
						<div
							v-if="selectedNegatedSites.length > 0"
							class="flex-row flex-wrap gap-2 mb-3"
						>
							<div
								v-for="site in selectedNegatedSites"
								:key="site.id"
								class="site-pill"
								@click="toggleNegatedSite(site.id)"
							>
								<span>{{ site.domain }}</span>
								<AppIcon name="x" size="12" />
							</div>
						</div>

						<div class="mb-2">
							<input
								v-model="negatedSitesSearch"
								type="text"
								placeholder="Search sites to add..."
								class="w-full font-sm"
							/>
						</div>
						<div class="checkbox-list">
							<div
								v-for="site in filteredNegatedSites"
								:key="site.id"
								class="checkbox-item"
								@click="toggleNegatedSite(site.id)"
							>
								<AppIcon
									name="plus-circle"
									size="14"
									class="text-primary opacity-50"
								/>
								<span class="font-sm">{{ site.domain }}</span>
							</div>
							<div
								v-if="filteredNegatedSites.length === 0"
								class="text-muted font-xs p-2 text-center"
							>
								{{
									sitesOnSelectedServer.length === 0
										? "No sites on this server."
										: negatedSitesSearch
											? "No matches found."
											: "All sites on this server are negated."
								}}
							</div>
						</div>
						<p class="font-xs text-muted mt-2">
							Select sites that should stay active while the rest
							of the server is ignored.
						</p>
					</div>

					<div class="form-group col-span-2">
						<label>Apply logic to:</label>
						<div class="flex-row gap-6 mt-2">
							<label
								class="flex-row items-center gap-2 cursor-pointer"
								:class="{
									'opacity-50 cursor-not-allowed':
										newEntry.type === 'plugin' ||
										newEntry.type === 'vulnerability',
								}"
							>
								<input
									v-model="newEntry.use_for_monitor"
									type="checkbox"
									:disabled="
										newEntry.type === 'plugin' ||
										newEntry.type === 'vulnerability'
									"
								/>
								<span class="font-sm"
									>Ignore when monitoring sites</span
								>
							</label>
							<label
								class="flex-row items-center gap-2 cursor-pointer"
							>
								<input
									v-model="newEntry.use_for_vuln"
									type="checkbox"
								/>
								<span class="font-sm"
									>Ignore when reporting vulnerabilities</span
								>
							</label>
						</div>
					</div>
				</div>

				<div class="mt-6 flex-row justify-end gap-3">
					<button
						type="button"
						class="btn btn-outline"
						@click="resetForm"
					>
						Cancel
					</button>
					<button
						type="submit"
						class="btn btn-primary"
						:disabled="isSubmitDisabled"
					>
						{{
							isSubmitting
								? "Saving..."
								: editingEntryId
									? "Update Entry"
									: "Add Entry"
						}}
					</button>
				</div>
			</form>
		</div>

		<div v-if="ignoreStore.isLoading" class="loading-state">
			<LoadingSpinner message="Loading ignore entries..." />
		</div>

		<div
			v-else-if="ignoreStore.ignoreEntries.length > 0"
			class="table-container"
		>
			<table class="data-table">
				<thead>
					<tr>
						<th>Type</th>
						<th>Target</th>
						<th>Purpose</th>
						<th class="hide-mobile">Reason</th>
						<th v-if="authStore.canEdit" class="text-right">
							Actions
						</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="entry in ignoreStore.ignoreEntries"
						:key="entry.id"
					>
						<td>
							<span class="status-badge info badge-sm">{{
								entry.type
							}}</span>
						</td>
						<td>
							<div
								class="font-medium text-main"
								:title="
									entry.type === 'vulnerability'
										? entry.target
										: ''
								"
							>
								{{
									resolveTargetName(entry.type, entry.target)
								}}
							</div>
							<div
								v-if="
									entry.negated_site_ids &&
									entry.negated_site_ids.length > 0
								"
								class="font-xs text-muted"
							>
								Negated:
								{{ entry.negated_site_ids.length }} sites
							</div>
						</td>
						<td>
							<div class="flex-row gap-1">
								<span
									v-if="entry.use_for_monitor"
									class="status-badge badge-sm"
									title="Uptime Monitoring"
									>Monitor</span
								>
								<span
									v-if="entry.use_for_vuln"
									class="status-badge badge-sm"
									title="Vulnerability Scanning"
									>Vuln</span
								>
							</div>
						</td>
						<td class="hide-mobile text-muted font-sm">
							{{ entry.reason || "—" }}
						</td>
						<td v-if="authStore.canEdit" class="text-right">
							<div class="flex-row justify-end gap-2">
								<button
									class="icon-btn"
									title="Edit Entry"
									@click="handleEditEntry(entry)"
								>
									<AppIcon name="edit" size="18" />
								</button>
								<button
									class="icon-btn danger"
									title="Remove Entry"
									@click="handleRemoveEntry(entry.id)"
								>
									<AppIcon name="trash" size="18" />
								</button>
							</div>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">No ignore entries found.</p>
		</div>
	</section>
</template>

<style scoped>
.checkbox-list {
	max-height: 150px;
	overflow-y: auto;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	padding: 4px;
	background-color: var(--bg-card);
}

.checkbox-item {
	display: flex;
	align-items: center;
	gap: 8px;
	padding: 4px 8px;
	cursor: pointer;
	border-radius: 2px;
	transition: background-color 0.2s;
}

.checkbox-item:hover {
	background-color: var(--bg-hover);
}

.checkbox-item input {
	margin: 0;
}

.gap-6 {
	gap: 24px;
}

.col-span-2 {
	grid-column: span 2;
}

.w-full {
	width: 100%;
}

.flex-wrap {
	flex-wrap: wrap;
}

.site-pill {
	display: flex;
	align-items: center;
	gap: 6px;
	padding: 4px 10px;
	background-color: var(--primary);
	color: var(--primary-text);
	border-radius: 16px;
	font-size: 12px;
	font-weight: 500;
	cursor: pointer;
	transition: opacity 0.2s;

	&:hover {
		opacity: 0.8;
	}
}
</style>
