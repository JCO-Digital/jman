<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useAuthStore } from "../../stores/auth";
import {
	useManagedSitesStore,
	READ_ONLY_PROVIDER,
} from "../../stores/managedSites";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import LoadingSpinner from "../LoadingSpinner.vue";
import SiteFormModal from "./SiteFormModal.vue";
import ServerFormModal from "./ServerFormModal.vue";
import type { ManagedServer, ManagedSite } from "../../types";

const SHOW_READ_ONLY_KEY = "jman_settings_show_read_only_sites";

const authStore = useAuthStore();
const managedSitesStore = useManagedSitesStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const showReadOnly = ref(loadShowReadOnly());
const showSiteModal = ref(false);
const showServerModal = ref(false);
const editingSite = ref<ManagedSite | null>(null);
const testingSiteId = ref<string | null>(null);
const isSyncing = ref(false);

function loadShowReadOnly(): boolean {
	try {
		return localStorage.getItem(SHOW_READ_ONLY_KEY) === "1";
	} catch {
		return false;
	}
}

watch(showReadOnly, (value) => {
	try {
		localStorage.setItem(SHOW_READ_ONLY_KEY, value ? "1" : "0");
	} catch {
		// Storage unavailable (private mode etc.); the toggle still works.
	}
});

onMounted(() => {
	managedSitesStore.fetchAll();
});

function isReadOnly(entity: { provider: string }): boolean {
	return entity.provider === READ_ONLY_PROVIDER;
}

const visibleSites = computed(() =>
	managedSitesStore.sites
		.filter((s) => showReadOnly.value || !isReadOnly(s))
		.slice()
		.sort((a, b) => a.domain.localeCompare(b.domain)),
);

const visibleServers = computed(() =>
	managedSitesStore.servers.filter(
		(s) => showReadOnly.value || !isReadOnly(s),
	),
);

const hiddenSiteCount = computed(
	() => managedSitesStore.sites.length - visibleSites.value.length,
);
const hiddenServerCount = computed(
	() => managedSitesStore.servers.length - visibleServers.value.length,
);

const siteCountByServer = computed(() => {
	const counts = new Map<string, number>();
	for (const site of managedSitesStore.sites) {
		if (!site.server_id) continue;
		counts.set(site.server_id, (counts.get(site.server_id) ?? 0) + 1);
	}
	return counts;
});

function connectionLabel(site: ManagedSite): string {
	if (!site.ssh_host) return "—";
	const user = site.ssh_user ? `${site.ssh_user}@` : "";
	const port =
		site.ssh_port && site.ssh_port !== 22 ? `:${site.ssh_port}` : "";
	return `${user}${site.ssh_host}${port}`;
}

function capabilityLabel(site: ManagedSite): string {
	const caps: string[] = [];
	if (site.can_wp_cli) caps.push("WP-CLI");
	if (site.has_agent) caps.push("Agent");
	if (site.has_monitoring) caps.push("Monitor");
	return caps.join(", ");
}

function statusBadgeClass(status: string): string {
	// SpinupWP reports "deployed" for live sites.
	if (status === "active" || status === "deployed") return "active";
	if (status === "archived") return "archived";
	return "warning";
}

// ---------------------------------------------------------------------------
// Site actions
// ---------------------------------------------------------------------------

function openCreateSite() {
	editingSite.value = null;
	showSiteModal.value = true;
}

function openEditSite(site: ManagedSite) {
	editingSite.value = site;
	showSiteModal.value = true;
}

function closeSiteModal() {
	showSiteModal.value = false;
	editingSite.value = null;
}

function handleSiteSaved(site: ManagedSite) {
	toast.addToast(`Site "${site.domain}" saved.`, "success");
}

async function handleDeleteSite(site: ManagedSite) {
	if (
		!(await confirm(
			`Are you sure you want to delete "${site.domain}"? Its collected inventory data will no longer be refreshed.`,
			{ danger: true, confirmLabel: "Delete" },
		))
	)
		return;

	try {
		await managedSitesStore.deleteSite(site.id);
		toast.addToast(`Site "${site.domain}" deleted.`, "success");
	} catch (e: any) {
		toast.addToast(e.message || "Failed to delete site", "error");
	}
}

async function handleTestConnection(site: ManagedSite) {
	testingSiteId.value = site.id;
	try {
		const result = await managedSitesStore.testConnection(site.id);
		if (result.success) {
			toast.addToast(
				`${site.domain}: ${result.message || "Connection OK"}`,
				"success",
			);
		} else {
			const detail = result.stderr || result.error || "Unknown error";
			toast.addToast(
				`${site.domain}: connection failed — ${detail}`,
				"error",
				10000,
			);
		}
	} catch (e: any) {
		toast.addToast(
			`${site.domain}: ${e.message || "Connection test failed"}`,
			"error",
		);
	} finally {
		testingSiteId.value = null;
	}
}

// ---------------------------------------------------------------------------
// Server actions
// ---------------------------------------------------------------------------

function handleServerCreated(server: ManagedServer) {
	toast.addToast(`Server "${server.name}" created.`, "success");
}

async function handleDeleteServer(server: ManagedServer) {
	if (
		!(await confirm(
			`Are you sure you want to delete server "${server.name}"?`,
			{ danger: true, confirmLabel: "Delete" },
		))
	)
		return;

	try {
		await managedSitesStore.deleteServer(server.id);
		toast.addToast(`Server "${server.name}" deleted.`, "success");
	} catch (e: any) {
		toast.addToast(e.message || "Failed to delete server", "error");
	}
}

async function handleSync() {
	isSyncing.value = true;
	try {
		const result = await managedSitesStore.syncSpinupWP();
		toast.addToast(result.message, "success");
	} catch (e: any) {
		toast.addToast(e.message || "SpinupWP sync failed", "error");
	} finally {
		isSyncing.value = false;
	}
}
</script>

<template>
	<div class="flex-row gap-3 flex-between mb-4 toolbar">
		<label class="checkbox-label">
			<input v-model="showReadOnly" type="checkbox" />
			Show read-only (SpinupWP) entries
		</label>
		<button
			class="btn btn-outline"
			:disabled="isSyncing"
			@click="handleSync"
		>
			{{ isSyncing ? "Syncing..." : "Sync SpinupWP" }}
		</button>
	</div>

	<div v-if="managedSitesStore.error" class="error-banner mb-4">
		<p>{{ managedSitesStore.error }}</p>
	</div>

	<!-- Sites -->
	<section class="card">
		<div class="card-header">
			<h2>Sites</h2>
			<button class="btn btn-primary" @click="openCreateSite">
				Add Site
			</button>
		</div>

		<p class="sub-text mb-4">
			External sites are collected over SSH/WP-CLI by jman-api. Sites
			synced from SpinupWP are read-only here, since the next sync would
			overwrite any changes.
		</p>

		<div
			v-if="
				managedSitesStore.isLoading &&
				managedSitesStore.sites.length === 0
			"
			class="loading-state"
		>
			<LoadingSpinner message="Loading sites..." />
		</div>

		<div v-else-if="visibleSites.length > 0" class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Domain</th>
						<th class="hide-mobile">Server</th>
						<th class="hide-mobile">Connection</th>
						<th>Status</th>
						<th class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="site in visibleSites" :key="site.id">
						<td>
							<div class="font-medium text-main">
								{{ site.domain }}
							</div>
							<div class="sub-text">
								{{ site.provider }}
								<span
									v-if="site.environment"
									:class="[
										'status-badge',
										'badge-sm',
										'ml-1',
										site.environment,
									]"
									>{{ site.environment }}</span
								>
								<span
									v-if="isReadOnly(site)"
									class="status-badge badge-sm archived ml-1"
									>Read-only</span
								>
							</div>
						</td>
						<td class="text-muted hide-mobile">
							{{ site.server_name || "—" }}
						</td>
						<td class="hide-mobile">
							<code>{{ connectionLabel(site) }}</code>
							<div class="sub-text">
								{{ site.connection_type }} ·
								{{ site.site_path || "—" }}
								<template v-if="capabilityLabel(site)">
									· {{ capabilityLabel(site) }}
								</template>
							</div>
						</td>
						<td>
							<span
								:class="[
									'status-badge',
									'badge-sm',
									statusBadgeClass(site.status),
								]"
								>{{ site.status }}</span
							>
						</td>
						<td class="text-right actions-cell">
							<button
								v-if="authStore.canExecute && site.can_wp_cli"
								class="btn btn-text"
								:disabled="testingSiteId === site.id"
								@click="handleTestConnection(site)"
							>
								{{
									testingSiteId === site.id
										? "Testing..."
										: "Test"
								}}
							</button>
							<template v-if="!isReadOnly(site)">
								<button
									class="btn btn-text"
									@click="openEditSite(site)"
								>
									Edit
								</button>
								<button
									class="btn btn-text danger"
									@click="handleDeleteSite(site)"
								>
									Delete
								</button>
							</template>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">
				No external sites yet.
				<template v-if="hiddenSiteCount > 0">
					{{ hiddenSiteCount }} read-only SpinupWP site(s) hidden.
				</template>
			</p>
		</div>
	</section>

	<!-- Servers -->
	<section class="card mt-4">
		<div class="card-header">
			<h2>Servers</h2>
			<button class="btn btn-primary" @click="showServerModal = true">
				Add Server
			</button>
		</div>

		<p class="sub-text mb-4">
			Physical servers are hosts with SSH/root access. Logical servers
			group sites on managed platforms (e.g. WP Engine, Kinsta) where
			connection details are configured per site.
		</p>

		<div
			v-if="
				managedSitesStore.isLoading &&
				managedSitesStore.servers.length === 0
			"
			class="loading-state"
		>
			<LoadingSpinner message="Loading servers..." />
		</div>

		<div v-else-if="visibleServers.length > 0" class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Name</th>
						<th>Provider</th>
						<th>Type</th>
						<th class="hide-mobile">IP Address</th>
						<th class="hide-mobile">SSH Port</th>
						<th>Sites</th>
						<th class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="server in visibleServers" :key="server.id">
						<td class="font-medium text-main">
							{{ server.name }}
						</td>
						<td>
							<span class="type-tag">{{ server.provider }}</span>
							<span
								v-if="isReadOnly(server)"
								class="status-badge badge-sm archived ml-1"
								>Read-only</span
							>
						</td>
						<td class="text-muted">
							{{ server.is_logical ? "Logical" : "Physical" }}
						</td>
						<td class="text-muted hide-mobile">
							{{ server.ip_address || "—" }}
						</td>
						<td class="text-muted hide-mobile">
							{{ server.is_logical ? "—" : server.ssh_port }}
						</td>
						<td class="text-muted">
							{{ siteCountByServer.get(server.id) ?? 0 }}
						</td>
						<td class="text-right">
							<button
								v-if="!isReadOnly(server)"
								class="btn btn-text danger"
								:disabled="
									(siteCountByServer.get(server.id) ?? 0) > 0
								"
								:title="
									(siteCountByServer.get(server.id) ?? 0) > 0
										? 'Reassign or delete its sites first'
										: undefined
								"
								@click="handleDeleteServer(server)"
							>
								Delete
							</button>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">
				No external servers yet.
				<template v-if="hiddenServerCount > 0">
					{{ hiddenServerCount }} read-only SpinupWP server(s) hidden.
				</template>
			</p>
		</div>
	</section>

	<SiteFormModal
		:visible="showSiteModal"
		:edit-site="editingSite"
		@close="closeSiteModal"
		@saved="handleSiteSaved"
	/>
	<ServerFormModal
		:visible="showServerModal"
		@close="showServerModal = false"
		@created="handleServerCreated"
	/>
</template>

<style scoped>
.toolbar {
	flex-wrap: wrap;
}

.checkbox-label {
	display: flex;
	align-items: center;
	gap: 8px;
	cursor: pointer;
}

.actions-cell {
	white-space: nowrap;
}

.ml-1 {
	margin-left: 4px;
}
</style>
