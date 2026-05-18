<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useIgnoreStore } from "../../stores/ignore";
import { useDataStore } from "../../stores/data";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import LoadingSpinner from "../LoadingSpinner.vue";
import SearchableSelect from "../SearchableSelect.vue";
import type { IgnoreType, CreateIgnorePayload } from "../../types";

const ignoreStore = useIgnoreStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toast = useToastStore();

const isSubmitting = ref(false);
const showAddForm = ref(false);
const negatedSitesSearch = ref("");

const isSubmitDisabled = computed(() => {
	if (!newEntry.value.target) return true;
	if (!newEntry.value.use_for_monitor && !newEntry.value.use_for_vuln)
		return true;
	return isSubmitting.value;
});

const newEntry = ref<CreateIgnorePayload>({
	type: "site",
	target: "",
	reason: "",
	use_for_monitor: true,
	use_for_vuln: true,
	negated_site_ids: [],
});

onMounted(() => {
	ignoreStore.fetchIgnoreEntries();
	dataStore.initData();
});

watch(
	() => [newEntry.value.type, newEntry.value.target],
	() => {
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
	if (!negatedSitesSearch.value) return sitesOnSelectedServer.value;
	const q = negatedSitesSearch.value.toLowerCase();
	return sitesOnSelectedServer.value.filter((s) =>
		s.domain.toLowerCase().includes(q),
	);
});

const handleAddEntry = async () => {
	if (!newEntry.value.target) return;

	isSubmitting.value = true;
	try {
		await ignoreStore.addIgnoreEntry(newEntry.value);
		newEntry.value = {
			type: "site",
			target: "",
			reason: "",
			use_for_monitor: true,
			use_for_vuln: true,
			negated_site_ids: [],
		};
		showAddForm.value = false;
		toast.addToast("Ignore entry added", "success");
	} catch (e: any) {
		toast.addToast(e.message || "Failed to add ignore entry", "error");
	} finally {
		isSubmitting.value = false;
	}
};

const handleRemoveEntry = async (id: number) => {
	if (!confirm("Are you sure you want to remove this ignore rule?")) return;

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
	return target;
};
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Unified Ignore List</h2>
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
				<h3 class="font-medium">Add New Ignore Rule</h3>
				<button
					class="btn btn-text btn-sm"
					@click="showAddForm = false"
				>
					Cancel
				</button>
			</div>

			<form @submit.prevent="handleAddEntry">
				<div class="grid-2-cols gap-4">
					<div class="form-group">
						<label>Type</label>
						<select v-model="newEntry.type" class="w-full">
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
						/>
						<SearchableSelect
							v-else-if="newEntry.type === 'server'"
							v-model="newEntry.target"
							:options="serverOptions"
							placeholder="Select Server"
						/>
						<SearchableSelect
							v-else-if="newEntry.type === 'plugin'"
							v-model="newEntry.target"
							:options="pluginOptions"
							placeholder="Select Plugin"
						/>
						<input
							v-else
							v-model="newEntry.target"
							type="text"
							placeholder="slug or UUID"
							required
							class="w-full"
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
						<div class="mb-2">
							<input
								v-model="negatedSitesSearch"
								type="text"
								placeholder="Filter sites..."
								class="w-full font-sm"
							/>
						</div>
						<div class="checkbox-list">
							<label
								v-for="site in filteredNegatedSites"
								:key="site.id"
								class="checkbox-item"
							>
								<input
									v-model="newEntry.negated_site_ids"
									type="checkbox"
									:value="site.id"
								/>
								<span class="font-sm">{{ site.domain }}</span>
							</label>
							<div
								v-if="filteredNegatedSites.length === 0"
								class="text-muted font-xs p-2"
							>
								{{
									sitesOnSelectedServer.length === 0
										? "No sites on this server."
										: "No matches found."
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

				<div class="mt-6 text-right">
					<button
						type="submit"
						class="btn btn-primary"
						:disabled="isSubmitDisabled"
					>
						{{ isSubmitting ? "Adding..." : "Add Entry" }}
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
							<div class="font-medium text-main">
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
							<button
								class="btn btn-text danger"
								@click="handleRemoveEntry(entry.id)"
							>
								Remove
							</button>
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
</style>
