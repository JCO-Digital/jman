<script setup lang="ts">
import { computed, ref } from "vue";
import { RouterLink } from "vue-router";
import type { Plugin, Site, SiteEnvironment } from "../types";
import { useDataStore } from "../stores/data";
import { useAuthStore } from "../stores/auth";
import { useToastStore } from "../stores/toast";
import { useConfirm } from "../composables/useConfirm";
import SearchableSelect from "./SearchableSelect.vue";
import { compareVersions } from "../utils/vulnList";
import {
	isLinkableEnvironment,
	suggestProductionSites,
} from "../utils/siteLinks";

// A production site's environments: the staging, development and archived
// sites linked to it, with their WordPress, PHP and plugin versions side by
// side. On a linked site it shows the same group, with the site highlighted.
const props = defineProps<{ site: Site }>();

const dataStore = useDataStore();
const authStore = useAuthStore();
const toast = useToastStore();
const { confirm } = useConfirm();

function formatDate(d: string): string {
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "short",
		timeStyle: "short",
	});
}

const ENV_ORDER: Record<SiteEnvironment, number> = {
	production: 0,
	staging: 1,
	development: 2,
	archived: 3,
};

const productionSites = computed(() =>
	dataStore.sites.filter(
		(s) => s.environment === "production" && !s.production_site_id,
	),
);

/** The production site of the site's group, if it has one. */
const productionSite = computed<Site | undefined>(() => {
	if (props.site.environment === "production") return props.site;
	if (props.site.production_site_id) {
		return dataStore.getSiteById(props.site.production_site_id);
	}
	return undefined;
});

const linkedSites = computed(() => {
	const prod = productionSite.value;
	if (!prod) return [];
	return dataStore.sites
		.filter((s) => s.production_site_id === prod.id)
		.sort(
			(a, b) =>
				(ENV_ORDER[a.environment ?? "archived"] ?? 9) -
					(ENV_ORDER[b.environment ?? "archived"] ?? 9) ||
				a.domain.localeCompare(b.domain),
		);
});

/** The group's sites, production first. */
const group = computed<Site[]>(() =>
	productionSite.value ? [productionSite.value, ...linkedSites.value] : [],
);

/** A link the environments no longer fit, e.g. a staging site made production. */
function linkMismatch(s: Site): string {
	if (s.id === productionSite.value?.id) {
		return s.environment === "production" ? "" : "No longer production";
	}
	return isLinkableEnvironment(s)
		? ""
		: "Now " + (s.environment ?? "unclassified");
}

const unlinkedLinkable = computed(() =>
	dataStore.sites.filter(
		(s) => isLinkableEnvironment(s) && !s.production_site_id,
	),
);

// On a production site: unlinked sites whose domain points at it.
const suggestedChildren = computed(() => {
	if (props.site.environment !== "production") return [];
	return unlinkedLinkable.value.filter((s) =>
		suggestProductionSites(s, productionSites.value).some(
			(p) => p.id === props.site.id,
		),
	);
});

// On an unlinked staging/dev/archived site: its likely production sites
// (several when the name matches more than one, e.g. sitename.fi and
// sitename.com).
const suggestedParents = computed(() =>
	isLinkableEnvironment(props.site) && !props.site.production_site_id
		? suggestProductionSites(props.site, productionSites.value)
		: [],
);

const pickerValue = ref<string | number | null>(null);
const pickerOptions = computed(() => {
	const sites =
		props.site.environment === "production"
			? unlinkedLinkable.value
			: productionSites.value.filter((s) => s.id !== props.site.id);
	return sites.map((s) => ({
		value: s.id,
		label: `${s.domain}${s.environment === "production" ? "" : ` (${s.environment})`}`,
	}));
});

const busy = ref(false);

async function link(childId: string, productionId: string) {
	busy.value = true;
	try {
		await dataStore.setProductionSite(childId, productionId);
		toast.addToast("Site linked.", "success");
		pickerValue.value = null;
	} catch (e: any) {
		toast.addToast("Failed to link site: " + e.message, "error");
	} finally {
		busy.value = false;
	}
}

function linkPicked() {
	const picked = pickerValue.value as string | null;
	if (!picked) return;
	if (props.site.environment === "production") {
		link(picked, props.site.id);
	} else {
		link(props.site.id, picked);
	}
}

async function unlink(child: Site) {
	if (
		!(await confirm(
			`Unlink ${child.domain} from ${productionSite.value?.domain ?? "its production site"}?`,
		))
	)
		return;
	busy.value = true;
	try {
		await dataStore.setProductionSite(child.id, null);
		toast.addToast("Site unlinked.", "success");
	} catch (e: any) {
		toast.addToast("Failed to unlink site: " + e.message, "error");
	} finally {
		busy.value = false;
	}
}

// --- Version comparison ---

const onlyDifferences = ref(true);

interface DiffCell {
	/** The site of the cell's column. */
	siteId: string;
	text: string;
	/** Compared with production: newer, older, missing or the same. */
	cls: string;
	title?: string;
}

interface DiffRow {
	key: string;
	label: string;
	isPlugin: boolean;
	cells: DiffCell[];
	differs: boolean;
}

function compareCell(
	value: string | undefined,
	base: string | undefined,
): string {
	if (!value) return "text-muted";
	if (!base || value === base) return "";
	return compareVersions(value, base) > 0 ? "text-success" : "text-warning";
}

function versionRow(
	key: string,
	label: string,
	siteIds: string[],
	values: (string | undefined)[],
	isPlugin = false,
	titles: (string | undefined)[] = [],
): DiffRow {
	const base = values[0];
	const cells = values.map((v, i) => ({
		siteId: siteIds[i] ?? "",
		text: v || "—",
		cls: i === 0 ? (v ? "" : "text-muted") : compareCell(v, base),
		title:
			titles[i] ??
			(i > 0 && v && base && v !== base
				? compareVersions(v, base) > 0
					? "Newer than production"
					: "Older than production"
				: undefined),
	}));
	const present = values.filter(Boolean);
	const differs =
		present.length !== values.length || new Set(present).size > 1;
	return { key, label, isPlugin, cells, differs };
}

const diffRows = computed<DiffRow[]>(() => {
	const sites = group.value;
	if (sites.length < 2) return [];
	const ids = sites.map((s) => s.id);

	const rows: DiffRow[] = [
		versionRow(
			"core",
			"WordPress",
			ids,
			sites.map((s) => s.wp_core?.version),
		),
		versionRow(
			"php",
			"PHP",
			ids,
			sites.map((s) => s.php_version || undefined),
		),
	];

	const pluginMaps = sites.map(
		(s) =>
			new Map<string, Plugin>(
				dataStore.getPluginsBySiteId(s.id).map((p) => [p.name, p]),
			),
	);
	const names = [...new Set(pluginMaps.flatMap((m) => [...m.keys()]))].sort();
	for (const name of names) {
		const plugins = pluginMaps.map((m) => m.get(name));
		const row = versionRow(
			"plugin:" + name,
			name,
			ids,
			plugins.map((p) => p?.version),
			true,
			plugins.map((p) => (p ? `Status: ${p.status}` : "Not installed")),
		);
		// An inactive copy is worth seeing even at the same version.
		row.cells.forEach((cell, i) => {
			const p = plugins[i];
			if (p && p.status !== "active" && p.status !== "must-use") {
				cell.text += ` (${p.status})`;
			}
		});
		if (new Set(plugins.map((p) => p?.status ?? "")).size > 1) {
			row.differs = true;
		}
		rows.push(row);
	}
	return rows;
});

const visibleDiffRows = computed(() =>
	onlyDifferences.value
		? diffRows.value.filter((r) => r.differs)
		: diffRows.value,
);
const differenceCount = computed(
	() => diffRows.value.filter((r) => r.differs).length,
);
</script>

<template>
	<section v-if="site.environment" class="card mt-4">
		<div class="card-header">
			<h2>Environments</h2>
			<div
				v-if="
					authStore.canEdit &&
					(site.environment === 'production' ||
						(isLinkableEnvironment(site) &&
							!site.production_site_id))
				"
				class="flex-row gap-3"
			>
				<SearchableSelect
					v-model="pickerValue"
					:options="pickerOptions"
					:placeholder="
						site.environment === 'production'
							? 'Link a staging or dev site…'
							: 'Link to a production site…'
					"
					:disabled="busy"
				/>
				<button
					class="btn btn-primary btn-sm"
					:disabled="!pickerValue || busy"
					@click="linkPicked"
				>
					Link
				</button>
			</div>
		</div>

		<!-- Suggestions from the domains -->
		<div
			v-if="authStore.canEdit && suggestedParents.length"
			class="flex-row gap-3 mb-4"
		>
			<span class="text-muted">Looks like a copy of</span>
			<button
				v-for="p in suggestedParents"
				:key="p.id"
				class="btn btn-outline btn-sm"
				:disabled="busy"
				@click="link(site.id, p.id)"
			>
				Link to {{ p.domain }}
			</button>
		</div>
		<div
			v-for="s in authStore.canEdit ? suggestedChildren : []"
			:key="s.id"
			class="flex-row gap-3 mb-4"
		>
			<span class="text-muted">
				Suggested:
				<span class="status-badge badge-sm" :class="s.environment">{{
					s.environment
				}}</span>
				<strong>{{ s.domain }}</strong>
			</span>
			<button
				class="btn btn-outline btn-sm"
				:disabled="busy"
				@click="link(s.id, site.id)"
			>
				Link
			</button>
		</div>

		<div v-if="group.length > 1" class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Environment</th>
						<th>Domain</th>
						<th>Server</th>
						<th>WordPress</th>
						<th>Last update</th>
						<th v-if="authStore.canEdit"></th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="s in group"
						:key="s.id"
						:class="{ 'is-current': s.id === site.id }"
					>
						<td>
							<span
								class="status-badge badge-sm"
								:class="s.environment"
								>{{ s.environment ?? "unclassified" }}</span
							>
							<span
								v-if="linkMismatch(s)"
								class="text-warning ml-4"
								:title="
									'The link no longer fits: ' +
									linkMismatch(s)
								"
								>⚠ {{ linkMismatch(s) }}</span
							>
						</td>
						<td class="font-medium">
							<span v-if="s.id === site.id">{{ s.domain }}</span>
							<RouterLink
								v-else
								:to="{
									name: 'site-detail',
									params: { id: s.id },
								}"
								>{{ s.domain }}</RouterLink
							>
						</td>
						<td class="text-muted">{{ s.server_name || "—" }}</td>
						<td>{{ s.wp_core?.version || "—" }}</td>
						<td class="text-muted">
							{{
								s.last_update
									? formatDate(s.last_update.updated_at)
									: "—"
							}}
						</td>
						<td v-if="authStore.canEdit" class="text-right">
							<button
								v-if="s.id !== productionSite?.id"
								class="btn btn-text danger"
								:disabled="busy"
								@click="unlink(s)"
							>
								Unlink
							</button>
						</td>
					</tr>
				</tbody>
			</table>
		</div>
		<div v-else-if="site.environment === 'production'" class="empty-state">
			No staging, development or archived sites linked.
		</div>
		<div v-else-if="isLinkableEnvironment(site)" class="empty-state">
			Not linked to a production site.
		</div>
		<div v-else class="empty-state">
			Only production, staging, development and archived sites can be
			linked.
		</div>

		<template v-if="diffRows.length">
			<div class="card-header mt-4">
				<h3>
					Version differences
					<span class="text-muted">({{ differenceCount }})</span>
				</h3>
				<label class="checkbox-label">
					<input v-model="onlyDifferences" type="checkbox" />
					Only differences
				</label>
			</div>
			<div v-if="visibleDiffRows.length" class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th></th>
							<th
								v-for="s in group"
								:key="s.id"
								:class="{ 'is-current': s.id === site.id }"
							>
								<span
									class="status-badge badge-sm"
									:class="s.environment"
									>{{ s.environment }}</span
								>
								<div class="sub-text">{{ s.domain }}</div>
							</th>
						</tr>
					</thead>
					<tbody>
						<tr v-for="row in visibleDiffRows" :key="row.key">
							<td class="font-medium">
								<RouterLink
									v-if="row.isPlugin"
									:to="{
										name: 'plugin-detail',
										params: { name: row.label },
									}"
									>{{ row.label }}</RouterLink
								>
								<template v-else>{{ row.label }}</template>
							</td>
							<td
								v-for="cell in row.cells"
								:key="cell.siteId"
								:class="[
									cell.cls,
									{ 'is-current': cell.siteId === site.id },
								]"
								:title="cell.title"
							>
								{{ cell.text }}
							</td>
						</tr>
					</tbody>
				</table>
			</div>
			<div v-else class="empty-state">
				WordPress, PHP and plugin versions match across the
				environments.
			</div>
		</template>
	</section>
</template>
