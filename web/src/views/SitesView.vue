<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useIgnoreStore } from "../stores/ignore";
import { useOrganizationStore } from "../stores/organization";
import type { EnrichedSite, SiteEnvironment, Organization } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import ListToolbar from "../components/ListToolbar.vue";
import SortableTh from "../components/SortableTh.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import { useListState } from "../composables/useListState";
import { useClientList } from "../composables/useClientList";
import { formatBytes } from "../utils/format";

const router = useRouter();
const dataStore = useDataStore();
const ignoreStore = useIgnoreStore();
const organizationStore = useOrganizationStore();

const ENVIRONMENTS = [
	"production",
	"staging",
	"development",
	"archived",
	"unclassified",
] as const;

// /sites?q=&sort=&dir=&page=&per=&env=&multisite=1&filemods=enabled|disabled
const list = useListState({
	sortKeys: [
		"domain",
		"organization",
		"server",
		"environment",
		"plugins",
		"vulns",
	],
	defaultSort: "domain",
	descFirst: ["plugins", "vulns"],
	filters: {
		env: ENVIRONMENTS,
		multisite: ["1"],
		filemods: ["enabled", "disabled"],
	},
});

const batchMode = ref(false);
const selectedSiteIds = ref<Set<string>>(new Set());
const batchEnvironment = ref<SiteEnvironment | "">("");
const isApplyingBatch = ref(false);

onMounted(() => {
	ignoreStore.fetchIgnoreEntries();
	organizationStore.fetchOrganizations();
});

const organizationsById = computed(() => {
	const map = new Map<number, Organization>();
	for (const org of organizationStore.organizations) {
		map.set(org.id, org);
	}
	return map;
});

const getOrganizationName = (orgId?: number) => {
	if (!orgId) return "";
	return organizationsById.value.get(orgId)?.name || "";
};

const toggleFilter = (name: "multisite" | "filemods", value: string) => {
	list.setFilter(name, list.filters.value[name] === value ? "" : value);
};

// The Vulns column counts plugin and WordPress core vulnerabilities.
function vulnCount(site: EnrichedSite): number {
	return site.vulnerabilities.length + site.coreVulnerabilities.length;
}

function vulnsAllSuppressed(site: EnrichedSite): boolean {
	return (
		site.vulnerabilities.every((v) => v.suppressed) &&
		site.coreVulnerabilities.every((v) => v.suppressed)
	);
}

function vulnTitle(site: EnrichedSite): string {
	if (vulnsAllSuppressed(site)) {
		return "All vulnerabilities are suppressed via ignore rules";
	}
	const parts: string[] = [];
	if (site.vulnerabilities.length) {
		parts.push(`${site.vulnerabilities.length} in plugins`);
	}
	if (site.coreVulnerabilities.length) {
		parts.push(`${site.coreVulnerabilities.length} in WordPress core`);
	}
	const n = vulnCount(site);
	return `${n} ${n === 1 ? "vulnerability" : "vulnerabilities"}: ${parts.join(", ")}`;
}

const sites = useClientList(() => dataStore.enrichedSites, list, {
	search: (site, q) => site.domain.toLowerCase().includes(q),
	filter: (site) => {
		const f = list.filters.value;
		if (f.env === "unclassified" && site.environment) return false;
		if (f.env && f.env !== "unclassified" && site.environment !== f.env)
			return false;
		if (f.multisite && !site.wp_flags?.is_multisite) return false;
		const fileModsOff = Boolean(site.wp_flags?.disallow_file_mods);
		if (f.filemods === "enabled" && fileModsOff) return false;
		if (f.filemods === "disabled" && !fileModsOff) return false;
		return true;
	},
	sortValue: (site, key) => {
		switch (key) {
			case "domain":
				return site.domain;
			case "organization":
				return getOrganizationName(site.organization_id);
			case "server":
				return site.server;
			case "environment":
				return site.environment;
			case "plugins":
				return site.plugins.length;
			case "vulns":
				return vulnCount(site);
		}
	},
});

const paginatedSites = sites.pageItems;

const allOnPageSelected = computed(
	() =>
		paginatedSites.value.length > 0 &&
		paginatedSites.value.every((site) =>
			selectedSiteIds.value.has(site.id),
		),
);

const toggleSelectAllOnPage = () => {
	if (allOnPageSelected.value) {
		for (const site of paginatedSites.value) {
			selectedSiteIds.value.delete(site.id);
		}
	} else {
		for (const site of paginatedSites.value) {
			selectedSiteIds.value.add(site.id);
		}
	}
};

const toggleSiteSelected = (id: string) => {
	if (selectedSiteIds.value.has(id)) {
		selectedSiteIds.value.delete(id);
	} else {
		selectedSiteIds.value.add(id);
	}
};

const toggleBatchMode = () => {
	batchMode.value = !batchMode.value;
	selectedSiteIds.value.clear();
	batchEnvironment.value = "";
};

const applyBatchEnvironment = async () => {
	if (selectedSiteIds.value.size === 0) return;
	isApplyingBatch.value = true;
	try {
		await Promise.all(
			Array.from(selectedSiteIds.value).map((id) =>
				dataStore.setSiteEnvironment(id, batchEnvironment.value),
			),
		);
		selectedSiteIds.value.clear();
	} finally {
		isApplyingBatch.value = false;
	}
};

const goToSite = (id: string) => {
	if (batchMode.value) {
		toggleSiteSelected(id);
		return;
	}
	router.push({ name: "site-detail", params: { id } });
};

// Disk space is SpinupWP-only server detail; other servers return null here.
const getServerDiskSpace = (serverId: string | null) => {
	const server = dataStore.getServerById(serverId);
	if (!server || !server.disk_space || server.disk_space.total === 0)
		return null;
	return server.disk_space;
};

const getServerDiskSpaceString = (serverId: string | null) => {
	const diskSpace = getServerDiskSpace(serverId);
	if (!diskSpace) return "";
	const percent = Math.round((diskSpace.used / diskSpace.total) * 100);
	return `${formatBytes(diskSpace.used)} / ${formatBytes(diskSpace.total)} (${percent}%)`;
};

function formatDate(d: string | null) {
	if (!d) return "—";
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "short",
		timeStyle: "short",
	});
}

function timeSince(dateString: string) {
	const date = new Date(dateString);
	const now = new Date();
	const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);

	let interval = Math.floor(seconds / 31536000);
	if (interval >= 1)
		return interval === 1 ? "1 year ago" : `${interval} years ago`;
	interval = Math.floor(seconds / 2592000);
	if (interval >= 1)
		return interval === 1 ? "1 month ago" : `${interval} months ago`;
	interval = Math.floor(seconds / 86400);
	if (interval >= 1)
		return interval === 1 ? "1 day ago" : `${interval} days ago`;
	interval = Math.floor(seconds / 3600);
	if (interval >= 1)
		return interval === 1 ? "1 hour ago" : `${interval} hours ago`;
	interval = Math.floor(seconds / 60);
	if (interval >= 1)
		return interval === 1 ? "1 minute ago" : `${interval} minutes ago`;
	return "just now";
}
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Site Management" />

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<ListToolbar :list="list" placeholder="Search sites by domain">
			<select
				:value="list.filters.value.env"
				aria-label="Environment"
				@change="
					list.setFilter(
						'env',
						($event.target as HTMLSelectElement).value,
					)
				"
			>
				<option value="">All environments</option>
				<option value="production">Production</option>
				<option value="staging">Staging</option>
				<option value="development">Development</option>
				<option value="archived">Archived</option>
				<option value="unclassified">Unclassified</option>
			</select>
			<button
				type="button"
				class="filter-chip"
				:aria-pressed="list.filters.value.multisite === '1'"
				@click="toggleFilter('multisite', '1')"
			>
				Multisite
			</button>
			<button
				type="button"
				class="filter-chip"
				:aria-pressed="list.filters.value.filemods === 'enabled'"
				@click="toggleFilter('filemods', 'enabled')"
			>
				File mods enabled
			</button>
			<button
				type="button"
				class="filter-chip"
				:aria-pressed="list.filters.value.filemods === 'disabled'"
				@click="toggleFilter('filemods', 'disabled')"
			>
				File mods disabled
			</button>
			<template #actions>
				<button
					type="button"
					class="btn"
					:class="batchMode ? 'btn-primary' : 'btn-outline'"
					@click="toggleBatchMode"
				>
					{{ batchMode ? "Cancel batch edit" : "Batch edit" }}
				</button>
			</template>
		</ListToolbar>

		<div v-if="batchMode" class="card-muted batch-action-bar">
			<span class="font-medium"
				>{{ selectedSiteIds.size }} site(s) selected</span
			>
			<select v-model="batchEnvironment">
				<option value="">Unclassified</option>
				<option value="production">Production</option>
				<option value="staging">Staging</option>
				<option value="development">Development</option>
				<option value="archived">Archived</option>
			</select>
			<button
				class="btn btn-primary btn-sm"
				:disabled="selectedSiteIds.size === 0 || isApplyingBatch"
				@click="applyBatchEnvironment"
			>
				{{ isApplyingBatch ? "Applying…" : "Apply" }}
			</button>
		</div>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th v-if="batchMode" class="col-narrow text-center">
							<input
								type="checkbox"
								:checked="allOnPageSelected"
								@click="toggleSelectAllOnPage"
							/>
						</th>
						<SortableTh
							:list="list"
							sort-key="domain"
							class="col-expand"
						>
							Site Name
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="organization"
							class="col-medium"
						>
							Organization
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="server"
							class="col-wide"
						>
							Server
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="environment"
							class="col-narrow text-center"
						>
							Environment
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="plugins"
							class="col-narrow text-center"
						>
							Plugins
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="vulns"
							class="col-narrow text-center"
						>
							Vulns
						</SortableTh>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							dataStore.isLoading && dataStore.sites.length === 0
						"
					>
						<td colspan="6">
							<LoadingSpinner message="Loading data..." />
						</td>
					</tr>
					<tr v-else-if="paginatedSites.length === 0">
						<td colspan="6" class="empty-state">
							<span v-if="list.query.value"
								>No sites found matching "{{
									list.query.value
								}}".</span
							>
							<span v-else-if="list.narrowed.value"
								>No sites found matching the selected
								filters.</span
							>
							<span v-else>No sites available.</span>
						</td>
					</tr>
					<tr
						v-for="site in paginatedSites"
						:key="site.id"
						class="clickable-row"
						@click="goToSite(site.id)"
					>
						<td v-if="batchMode" class="col-narrow text-center">
							<input
								type="checkbox"
								:checked="selectedSiteIds.has(site.id)"
								@click.stop="toggleSiteSelected(site.id)"
							/>
						</td>
						<td class="font-medium truncate col-expand">
							<div class="flex-row gap-2">
								<RouterLink
									:to="{
										name: 'site-detail',
										params: { id: site.id },
									}"
									class="row-link truncate"
									@click.stop="
										batchMode && $event.preventDefault()
									"
								>
									{{ site.domain }}
								</RouterLink>
								<span
									v-if="site.wp_flags?.is_multisite"
									class="status-badge badge-sm info"
								>
									Multisite
								</span>
								<span
									v-if="site.wp_flags?.disallow_file_mods"
									class="status-badge badge-sm warning"
								>
									File Mods Disabled
								</span>
							</div>
							<div
								v-if="site.disk_usage"
								class="text-muted font-xs"
								:title="
									'Disk measured: ' +
									formatDate(site.disk_usage.measured_at)
								"
								style="margin-top: 2px"
							>
								Disk:
								{{ formatBytes(site.disk_usage.bytes_used) }}
							</div>
							<div
								v-if="site.last_update"
								class="text-muted font-xs"
								:title="
									'Last update: ' +
									formatDate(site.last_update.updated_at) +
									' by ' +
									site.last_update.updated_by
								"
								style="margin-top: 2px"
							>
								Updated:
								{{ timeSince(site.last_update.updated_at) }}
							</div>
						</td>
						<td
							class="col-medium truncate"
							:title="
								site.organization_id
									? getOrganizationName(site.organization_id)
									: ''
							"
						>
							<span v-if="site.organization_id">
								{{ getOrganizationName(site.organization_id) }}
							</span>
							<span v-else class="text-muted">—</span>
						</td>
						<td class="col-wide">
							<div
								class="truncate"
								:class="
									site.server_id == null
										? 'text-muted'
										: 'font-medium'
								"
							>
								{{ site.server }}
							</div>
							<div
								v-if="getServerDiskSpace(site.server_id)"
								class="text-muted font-xs"
								:title="
									'Disk updated: ' +
									formatDate(
										getServerDiskSpace(site.server_id)
											?.updated_at || null,
									)
								"
								style="margin-top: 2px"
							>
								Disk:
								{{ getServerDiskSpaceString(site.server_id) }}
							</div>
						</td>
						<td class="col-narrow text-center">
							<span
								v-if="site.environment"
								class="status-badge badge-sm"
								:class="site.environment"
							>
								{{ site.environment }}
							</span>
							<span v-else class="text-muted">—</span>
						</td>
						<td class="col-narrow text-center">
							{{
								site.is_wordpress
									? site.plugins.length
									: "Not WP"
							}}
						</td>
						<td class="col-narrow text-center">
							<span
								v-if="vulnCount(site) > 0"
								class="status-badge"
								:class="
									vulnsAllSuppressed(site)
										? 'warning'
										: 'error'
								"
								:title="vulnTitle(site)"
							>
								{{ vulnCount(site) }}
							</span>
							<span v-else class="text-muted">—</span>
						</td>
					</tr>
				</tbody>
			</table>

			<Pagination
				:list="list"
				:page="sites.page.value"
				:total="sites.total.value"
			/>
		</main>
	</div>
</template>
