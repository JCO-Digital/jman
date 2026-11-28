<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useScanStore } from "../../stores/scan";
import { useIncidentStore } from "../../stores/incidents";
import { useDataStore } from "../../stores/data";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import AppIcon from "../AppIcon.vue";
import LoadingSpinner from "../LoadingSpinner.vue";
import Pagination from "../Pagination.vue";
import ListToolbar from "../ListToolbar.vue";
import SortableTh from "../SortableTh.vue";
import { useListState } from "../../composables/useListState";
import { useClientList } from "../../composables/useClientList";
import ScanFindingsTable from "./ScanFindingsTable.vue";
import ScanTriageModal from "./ScanTriageModal.vue";
import SectionBar from "./SectionBar.vue";
import EmptyState from "./EmptyState.vue";
import {
	pluginName,
	pluginSlug,
	severityClass,
	severityRank,
	unknownPluginRule,
	worstSeverity,
} from "./scan";
import type { Section } from "./sections";
import { useRouteSection } from "../../composables/useRouteSection";
import type { Incident, ScanAllowlistEntry, ScanFinding } from "../../types";

// "open" lists every site with open findings, incident or not. Its URL is
// /monitoring/security; the incidents and informational sections it
// replaced fall back to it.
type SectionKey = "open" | "history" | "plugins" | "allowlist" | "acks";
const sectionKeys: SectionKey[] = [
	"open",
	"history",
	"plugins",
	"allowlist",
	"acks",
];

const scanStore = useScanStore();
const incidentStore = useIncidentStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();
const route = useRoute();

const section = useRouteSection(sectionKeys, "open");
const sections = computed<Section<SectionKey>[]>(() => [
	{
		key: "open",
		label: "Open",
		count: scanStore.activeIncidents.length,
	},
	{ key: "history", label: "History" },
	{ key: "plugins", label: "Allowed plugins" },
	{ key: "allowlist", label: "Allowlisted files" },
	{ key: "acks", label: "Acknowledged paths" },
]);
/** Sites with a file scan, for the empty state; null until loaded. */
const scannedSites = ref<number | null>(null);
const loading = ref(false);
const canTriage = computed(() => authStore.canExecute);

// Findings of the expanded incidents, by incident ID.
const expanded = ref(new Set<number>());
const incidentFindings = ref<Record<number, ScanFinding[]>>({});
/** Open findings in no incident: low and informational ones. */
const unassigned = ref<ScanFinding[]>([]);
/** Expanded cards of the open section, by SiteGroup key. */
const expandedGroups = ref(new Set<string>());
/** The incident a link opened (?incident=, from Slack), until it's shown. */
let linkedIncident = 0;
const showInfo = ref(true);

/**
 * A card of the open section: a security incident (findings of medium and
 * above), or a site's findings in no incident. A site's findings in no
 * incident go on the card of its worst incident if it has one; a site with
 * two active incidents (one reopened beside it) gets a card for each.
 */
interface SiteGroup {
	key: string;
	/** "" for an incident without a site. */
	siteId: string;
	domain: string;
	incident?: Incident;
	/** Open findings in no incident: low and informational ones. */
	minor: ScanFinding[];
	severity: string;
}

function incidentKey(inc: Incident): string {
	return `incident-${inc.id}`;
}

const siteGroups = computed<SiteGroup[]>(() => {
	const groups: SiteGroup[] = scanStore.activeIncidents.map((inc) => ({
		key: incidentKey(inc),
		siteId: inc.site_id ?? "",
		domain: inc.domain,
		incident: inc,
		minor: [],
		severity: inc.severity ?? "",
	}));
	// Worst incident first, so a site's other findings join that one.
	groups.sort(
		(a, b) =>
			severityRank(b.severity) - severityRank(a.severity) ||
			a.domain.localeCompare(b.domain),
	);
	const bySite = new Map<string, SiteGroup>();
	for (const g of groups) {
		if (g.siteId && !bySite.has(g.siteId)) bySite.set(g.siteId, g);
	}
	const minorOnly: SiteGroup[] = [];
	for (const f of unassigned.value) {
		if (!showInfo.value && f.severity === "info") continue;
		let g = bySite.get(f.site_id);
		if (!g) {
			g = {
				key: `site-${f.site_id}`,
				siteId: f.site_id,
				domain: siteDomain(f.site_id),
				minor: [],
				severity: "",
			};
			bySite.set(f.site_id, g);
			minorOnly.push(g);
		}
		g.minor.push(f);
	}
	for (const g of minorOnly) g.severity = worstSeverity(g.minor);
	minorOnly.sort(
		(a, b) =>
			severityRank(b.severity) - severityRank(a.severity) ||
			a.domain.localeCompare(b.domain),
	);
	return [...groups, ...minorOnly];
});

const infoCount = computed(
	() => unassigned.value.filter((f) => f.severity === "info").length,
);

/** "2 low, 5 info", for a site's findings in no incident. */
function minorSummary(findings: ScanFinding[]): string {
	const counts = new Map<string, number>();
	for (const f of findings) {
		counts.set(f.severity, (counts.get(f.severity) ?? 0) + 1);
	}
	return [...counts.entries()]
		.sort(([a], [b]) => severityRank(b) - severityRank(a))
		.map(([sev, n]) => `${n} ${sev}`)
		.join(", ");
}

/** "3 open findings: 2 stray-file, 1 unreadable" */
function ruleSummary(findings: ScanFinding[]): string {
	const counts = new Map<string, number>();
	for (const f of findings) counts.set(f.rule, (counts.get(f.rule) ?? 0) + 1);
	const n = findings.length;
	return `${n} open finding${n === 1 ? "" : "s"}: ${[...counts.entries()]
		.sort((a, b) => b[1] - a[1])
		.map(([rule, c]) => `${c} ${rule}`)
		.join(", ")}`;
}
// The history and plugin sections' page, and the plugin list's search
// and sort, are in the URL like every list's; choosing another section
// drops them.
const list = useListState({
	sortKeys: ["plugin", "added"],
	defaultSort: "plugin",
	descFirst: ["added"],
});
const showRemoved = ref(false);

/** Allowlisted plugins: unknown-plugin entries, trusted by slug. */
interface PluginEntry {
	entry: ScanAllowlistEntry;
	slug: string;
	name: string;
}
const pluginEntries = computed<PluginEntry[]>(() =>
	scanStore.allowlist
		.filter((e) => e.rule === unknownPluginRule)
		.map((entry) => ({
			entry,
			slug: pluginSlug(entry.path),
			name: pluginName(entry.snippet),
		})),
);
const fileEntries = computed(() =>
	scanStore.allowlist.filter((e) => e.rule !== unknownPluginRule),
);
const pluginList = useClientList(pluginEntries, list, {
	search: (p, q) =>
		p.slug.toLowerCase().includes(q) ||
		p.name.toLowerCase().includes(q) ||
		p.entry.reason.toLowerCase().includes(q),
	sortValue: (p, key) => (key === "added" ? p.entry.created_at : p.slug),
});

const triageMode = ref<"allowlist" | "acknowledge" | null>(null);
const triageFindings = ref<ScanFinding[]>([]);

onMounted(async () => {
	const id = Number(route.query.incident);
	if (id) {
		expanded.value.add(id);
		linkedIncident = id;
	}
	scanStore
		.fetchScannedSiteCount()
		.then((n) => (scannedSites.value = n))
		.catch(() => {});
	await refresh();
});

watch([section, list.page, list.perPage], () => refresh());
watch(showRemoved, () => refresh());

async function refresh() {
	loading.value = true;
	try {
		switch (section.value) {
			case "open": {
				const [, minor] = await Promise.all([
					scanStore.fetchActiveIncidents(),
					scanStore.fetchFindings({
						status: "open",
						unassigned: true,
						limit: 1000,
					}),
				]);
				unassigned.value = minor;
				// An incident linked to (?incident=, from Slack) opens, once.
				if (linkedIncident) {
					expandedGroups.value.add(`incident-${linkedIncident}`);
					linkedIncident = 0;
				}
				await Promise.all(
					scanStore.activeIncidents
						.filter((inc) =>
							expandedGroups.value.has(incidentKey(inc)),
						)
						.map((inc) => loadFindings(inc.id)),
				);
				break;
			}
			case "history":
				await scanStore.fetchHistoryIncidents(
					list.page.value,
					list.perPage.value,
				);
				await Promise.all([...expanded.value].map(loadFindings));
				break;
			case "plugins":
			case "allowlist":
				await scanStore.fetchAllowlist(showRemoved.value);
				break;
			case "acks":
				await scanStore.fetchPathAcks(showRemoved.value);
				break;
		}
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load security data",
			"error",
		);
	} finally {
		loading.value = false;
	}
}

defineExpose({ refresh });

async function loadFindings(id: number) {
	incidentFindings.value[id] = await scanStore.fetchFindings({
		incident_id: id,
		limit: 1000,
	});
}

async function toggleSite(g: SiteGroup) {
	if (expandedGroups.value.has(g.key)) {
		expandedGroups.value.delete(g.key);
		return;
	}
	expandedGroups.value.add(g.key);
	if (!g.incident) return;
	try {
		await loadFindings(g.incident.id);
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load findings",
			"error",
		);
	}
}

async function toggleIncident(inc: Incident) {
	if (expanded.value.has(inc.id)) {
		expanded.value.delete(inc.id);
		return;
	}
	expanded.value.add(inc.id);
	try {
		await loadFindings(inc.id);
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load findings",
			"error",
		);
	}
}

function siteDomain(siteId?: string): string {
	return dataStore.sites.find((s) => s.id === siteId)?.domain ?? siteId ?? "";
}

function formatTime(iso?: string | null): string {
	if (!iso) return "-";
	return new Date(iso).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}

async function afterTriage() {
	triageMode.value = null;
	await Promise.all([refresh(), incidentStore.fetchActiveCount()]);
}

function openTriage(
	mode: "allowlist" | "acknowledge",
	findings: ScanFinding[],
) {
	triageFindings.value = findings;
	triageMode.value = mode;
}

async function dismiss(findings: ScanFinding[]) {
	const n = findings.length;
	if (
		!(await confirm(
			`Dismiss ${n} finding${n === 1 ? "" : "s"}? A finding the agent reports again opens again. An incident left without open findings is closed.`,
			{ confirmLabel: "Dismiss" },
		))
	) {
		return;
	}
	try {
		await scanStore.closeFindings(findings.map((f) => f.id));
		await afterTriage();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to dismiss",
			"error",
		);
	}
}

async function redact(findings: ScanFinding[]) {
	const n = findings.length;
	if (
		!(await confirm(
			`Remove the snippet${n === 1 ? "" : "s"} of ${n} finding${n === 1 ? "" : "s"}? What ${n === 1 ? "it quotes" : "they quote"} from the files is deleted for good; the rule, path and hash stay.`,
			{ confirmLabel: "Remove", danger: true },
		))
	) {
		return;
	}
	try {
		const res = await scanStore.redactFindings(findings.map((f) => f.id));
		toastStore.addToast(
			`Removed ${res?.redacted ?? 0} snippet(s)`,
			"success",
		);
		await refresh();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to remove the snippets",
			"error",
		);
	}
}

async function acknowledgeIncident(inc: Incident) {
	try {
		await incidentStore.acknowledgeIncident(inc.id);
		await refresh();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to acknowledge",
			"error",
		);
	}
}

async function closeIncident(inc: Incident) {
	let present = "";
	try {
		const open = (
			await scanStore.fetchFindings({
				incident_id: inc.id,
				status: "open",
				limit: 1000,
			})
		).filter((f) => !f.cleared_at);
		present = open.length
			? `${open.length} open finding${open.length === 1 ? " still holds" : "s still hold"}: the agent hasn't seen the file${open.length === 1 ? "" : "s"} removed or fixed, and won't flag ${open.length === 1 ? "it" : "them"} again unless ${open.length === 1 ? "it changes" : "they change"}. `
			: "The agent found every open finding's file removed or fixed. ";
	} catch {
		// Closing doesn't depend on the warning.
	}
	if (
		!(await confirm(
			`Close the security incident for ${inc.domain}? ${present}Its open findings are closed too; if a file with the same content is found again, the incident reopens.`,
			{ confirmLabel: "Close incident" },
		))
	) {
		return;
	}
	try {
		await incidentStore.closeIncident(inc.id);
		toastStore.addToast(`Closed incident for ${inc.domain}`, "success");
		await afterTriage();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to close",
			"error",
		);
	}
}

async function removeAllowlist(e: ScanAllowlistEntry) {
	const question =
		e.rule === unknownPluginRule
			? `Remove the plugin ${pluginSlug(e.path)} from the allowlist? It will be flagged again on every site that has it; findings it already covered stay allowlisted.`
			: `Remove ${e.sha256.slice(0, 12)}… from the allowlist? Files with this content will be flagged again; findings it already covered stay allowlisted.`;
	if (!(await confirm(question, { confirmLabel: "Remove", danger: true }))) {
		return;
	}
	try {
		await scanStore.removeAllowlist(e.id);
		await refresh();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to remove",
			"error",
		);
	}
}

async function removeAck(id: number, path: string) {
	if (
		!(await confirm(
			`Remove the acknowledgement of ${path}? The path will be flagged again when it changes.`,
			{ confirmLabel: "Remove", danger: true },
		))
	) {
		return;
	}
	try {
		await scanStore.removePathAck(id);
		await refresh();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to remove",
			"error",
		);
	}
}
</script>

<template>
	<div class="security-panel">
		<SectionBar v-model="section" :sections="sections">
			<label v-if="section === 'open'" class="checkbox-label">
				<input v-model="showInfo" type="checkbox" />
				Show informational<template v-if="infoCount">
					({{ infoCount }})</template
				>
			</label>
			<label
				v-if="
					section === 'plugins' ||
					section === 'allowlist' ||
					section === 'acks'
				"
				class="checkbox-label"
			>
				<input v-model="showRemoved" type="checkbox" />
				Show removed
			</label>
			<LoadingSpinner v-if="loading" small />
		</SectionBar>

		<!-- Open findings by site: incidents and the rest -->
		<template v-if="section === 'open'">
			<p class="sub-text section-hint">
				Findings of medium severity and above open an incident for their
				site, which alerts by severity. Low and informational ones are
				listed with their site without one: clean up the file,
				acknowledge its path if it's meant to be there, or dismiss it.
				Informational findings, such as backups of files that aren't
				code, don't keep plugins from being copied off the site.
			</p>
			<EmptyState
				v-if="!loading && siteGroups.length === 0"
				class="card"
				icon="🛡️"
				title="Nothing open"
			>
				<template v-if="scannedSites !== null">
					Scanning {{ scannedSites }} site{{
						scannedSites === 1 ? "" : "s"
					}}.
				</template>
				<template v-if="!showInfo && infoCount">
					{{ infoCount }} informational finding{{
						infoCount === 1 ? " is" : "s are"
					}}
					hidden.
				</template>
			</EmptyState>
			<div
				v-for="g in siteGroups"
				:key="g.key"
				class="card incident-card"
				:class="{ 'site-minor': !g.incident }"
			>
				<div class="incident-head">
					<button
						class="btn-text incident-toggle"
						@click="toggleSite(g)"
					>
						<AppIcon
							name="chevron-right"
							size="14"
							:class="{ rotated: expandedGroups.has(g.key) }"
						/>
						<span
							class="status-badge badge-sm"
							:class="severityClass(g.severity)"
						>
							{{ g.severity || "-" }}
						</span>
						<span class="font-medium">{{ g.domain }}</span>
						<span v-if="g.incident" class="sub-text"
							>#{{ g.incident.id }}</span
						>
					</button>
					<span class="sub-text summary">
						<template v-if="g.incident">
							{{ g.incident.error_message }}
							<template v-if="g.minor.length">
								· also {{ minorSummary(g.minor) }}
							</template>
						</template>
						<template v-else>{{ ruleSummary(g.minor) }}</template>
					</span>
					<template v-if="g.incident">
						<span
							class="sub-text"
							:title="`Since ${formatTime(g.incident.down_since)}`"
						>
							since {{ formatTime(g.incident.down_since) }}
						</span>
						<span
							v-if="g.incident.status === 'acknowledged'"
							class="status-badge badge-sm warning"
							:title="`at ${formatTime(g.incident.acknowledged_at)}`"
						>
							Acked by {{ g.incident.acknowledged_by || "user" }}
						</span>
						<span
							v-if="g.incident.pd_triggered"
							class="status-badge badge-sm error"
						>
							Paging
						</span>
					</template>
					<span v-else class="status-badge badge-sm"
						>No incident</span
					>
					<div class="actions-group">
						<RouterLink
							v-if="g.siteId"
							:to="`/site/${g.siteId}`"
							class="btn btn-sm btn-text"
						>
							Site
						</RouterLink>
						<template v-if="g.incident">
							<button
								v-if="g.incident.status === 'open'"
								class="btn btn-sm btn-outline"
								@click="acknowledgeIncident(g.incident)"
							>
								Acknowledge
							</button>
							<button
								class="btn btn-sm btn-outline"
								@click="closeIncident(g.incident)"
							>
								Close
							</button>
						</template>
					</div>
				</div>
				<p v-if="g.incident?.escalation" class="escalation">
					<strong>Critical:</strong> {{ g.incident.escalation }}
				</p>
				<div v-if="expandedGroups.has(g.key)" class="incident-body">
					<template v-if="g.incident">
						<LoadingSpinner
							v-if="!incidentFindings[g.incident.id]"
							small
						/>
						<ScanFindingsTable
							v-else
							:findings="incidentFindings[g.incident.id] ?? []"
							:can-triage="canTriage"
							@allowlist="(fs) => openTriage('allowlist', fs)"
							@acknowledge="(fs) => openTriage('acknowledge', fs)"
							@dismiss="dismiss"
							@redact="redact"
						/>
					</template>
					<template v-if="g.minor.length">
						<h4 v-if="g.incident" class="minor-heading">
							Not in the incident
						</h4>
						<ScanFindingsTable
							:findings="g.minor"
							:can-triage="canTriage"
							@allowlist="(fs) => openTriage('allowlist', fs)"
							@acknowledge="(fs) => openTriage('acknowledge', fs)"
							@dismiss="dismiss"
							@redact="redact"
						/>
					</template>
				</div>
			</div>
		</template>

		<!-- Closed incidents -->
		<div v-else-if="section === 'history'" class="card">
			<p
				v-if="!loading && scanStore.historyIncidents.length === 0"
				class="sub-text empty-state"
			>
				No closed security incidents.
			</p>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Severity</th>
							<th>Site</th>
							<th>Findings</th>
							<th>Opened</th>
							<th>Closed</th>
						</tr>
					</thead>
					<tbody>
						<template
							v-for="inc in scanStore.historyIncidents"
							:key="inc.id"
						>
							<tr
								class="clickable-row"
								@click="toggleIncident(inc)"
							>
								<td>
									<span
										class="status-badge badge-sm"
										:class="severityClass(inc.severity)"
									>
										{{ inc.severity || "-" }}
									</span>
								</td>
								<td class="font-medium">
									{{ inc.domain }}
									<span class="sub-text">#{{ inc.id }}</span>
								</td>
								<td class="sub-text">
									{{ inc.error_message }}
								</td>
								<td class="sub-text">
									{{ formatTime(inc.down_since) }}
								</td>
								<td class="sub-text">
									{{ formatTime(inc.resolved_at) }}
									<template v-if="inc.resolved_by">
										by {{ inc.resolved_by }}
									</template>
								</td>
							</tr>
							<tr v-if="expanded.has(inc.id)">
								<td colspan="5">
									<LoadingSpinner
										v-if="!incidentFindings[inc.id]"
										small
									/>
									<ScanFindingsTable
										v-else
										:findings="
											incidentFindings[inc.id] ?? []
										"
										:can-triage="false"
										:can-redact="canTriage"
										@redact="redact"
									/>
								</td>
							</tr>
						</template>
					</tbody>
				</table>
				<div class="pagination-wrapper">
					<Pagination
						:list="list"
						:page="list.page.value"
						:total="scanStore.historyTotal"
					/>
				</div>
			</div>
		</div>

		<!-- Allowed plugins -->
		<div v-else-if="section === 'plugins'" class="card">
			<p class="sub-text section-hint">
				Plugins that aren't on wordpress.org or from J&amp;Co Digital,
				trusted by slug on every site, including sites that get them
				later. Allowlist an unknown-plugin finding to add one.
			</p>
			<ListToolbar
				:list="list"
				placeholder="Search by slug, name or reason"
			/>
			<p
				v-if="!loading && pluginList.total.value === 0"
				class="sub-text empty-state"
			>
				{{
					pluginEntries.length
						? "No plugin matches."
						: "No plugins are allowlisted."
				}}
			</p>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<SortableTh :list="list" sort-key="plugin"
								>Plugin</SortableTh
							>
							<th>Reason</th>
							<th class="hide-col-3-sm">First seen on</th>
							<SortableTh :list="list" sort-key="added"
								>Added</SortableTh
							>
							<th v-if="canTriage"></th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="p in pluginList.pageItems.value"
							:key="p.entry.id"
							:class="{ 'row-muted': p.entry.removed }"
						>
							<td>
								<code class="font-medium">{{ p.slug }}</code>
								<div
									v-if="p.name && p.name !== p.slug"
									class="sub-text"
								>
									{{ p.name }}
								</div>
							</td>
							<td>{{ p.entry.reason }}</td>
							<td class="sub-text hide-col-3-sm">
								{{ siteDomain(p.entry.site_id) }}
							</td>
							<td class="sub-text">
								{{ formatTime(p.entry.created_at) }} by
								{{ p.entry.created_by }}
								<div v-if="p.entry.removed">
									Removed
									{{ formatTime(p.entry.updated_at) }} by
									{{ p.entry.updated_by }}
								</div>
							</td>
							<td v-if="canTriage" class="text-right">
								<button
									v-if="!p.entry.removed"
									class="icon-btn"
									title="Remove from the allowlist"
									@click="removeAllowlist(p.entry)"
								>
									<AppIcon name="trash" size="14" />
								</button>
							</td>
						</tr>
					</tbody>
				</table>
				<div class="pagination-wrapper">
					<Pagination
						:list="list"
						:page="pluginList.page.value"
						:total="pluginList.total.value"
					/>
				</div>
			</div>
		</div>

		<!-- Allowlisted files -->
		<div v-else-if="section === 'allowlist'" class="card">
			<p class="sub-text section-hint">
				File content trusted on every site. Allowlist a finding to add
				an entry.
			</p>
			<p
				v-if="!loading && fileEntries.length === 0"
				class="sub-text empty-state"
			>
				The allowlist is empty.
			</p>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>SHA-256</th>
							<th>Reason</th>
							<th>From</th>
							<th>Added</th>
							<th v-if="canTriage"></th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="e in fileEntries"
							:key="e.id"
							:class="{ 'row-muted': e.removed }"
						>
							<td>
								<code :title="e.sha256"
									>{{ e.sha256.slice(0, 12) }}…</code
								>
							</td>
							<td>{{ e.reason }}</td>
							<td class="sub-text">
								{{ siteDomain(e.site_id) }}:
								<code>{{ e.path }}</code> ({{ e.rule }})
							</td>
							<td class="sub-text">
								{{ formatTime(e.created_at) }} by
								{{ e.created_by }}
								<div v-if="e.removed">
									Removed {{ formatTime(e.updated_at) }} by
									{{ e.updated_by }}
								</div>
							</td>
							<td v-if="canTriage" class="text-right">
								<button
									v-if="!e.removed"
									class="icon-btn"
									title="Remove from the allowlist"
									@click="removeAllowlist(e)"
								>
									<AppIcon name="trash" size="14" />
								</button>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</div>

		<!-- Path acknowledgements -->
		<div v-else-if="section === 'acks'" class="card">
			<p class="sub-text section-hint">
				Paths of stray, foreign-owned or unreadable files, and accounts,
				that are meant to be there, per site. Plugins are allowlisted
				instead.
			</p>
			<p
				v-if="!loading && scanStore.pathAcks.length === 0"
				class="sub-text empty-state"
			>
				No acknowledged paths.
			</p>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Site</th>
							<th>Path</th>
							<th>Reason</th>
							<th>Added</th>
							<th v-if="canTriage"></th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="a in scanStore.pathAcks"
							:key="a.id"
							:class="{ 'row-muted': a.removed }"
						>
							<td class="font-medium">
								{{ siteDomain(a.site_id) }}
							</td>
							<td>
								<code>{{ a.path }}</code>
								<span class="sub-text"> ({{ a.rule }})</span>
							</td>
							<td>{{ a.reason }}</td>
							<td class="sub-text">
								{{ formatTime(a.created_at) }} by
								{{ a.created_by }}
								<div v-if="a.removed">
									Removed {{ formatTime(a.updated_at) }} by
									{{ a.updated_by }}
								</div>
							</td>
							<td v-if="canTriage" class="text-right">
								<button
									v-if="!a.removed"
									class="icon-btn"
									title="Remove the acknowledgement"
									@click="removeAck(a.id, a.path)"
								>
									<AppIcon name="trash" size="14" />
								</button>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</div>

		<ScanTriageModal
			:mode="triageMode"
			:findings="triageFindings"
			@close="triageMode = null"
			@done="afterTriage"
		/>
	</div>
</template>

<style scoped>
.escalation {
	margin: var(--space-2) 0 0;
	padding: var(--space-2) var(--space-3);
	border-radius: var(--radius-sm);
	background: var(--error-bg);
	color: var(--error-text);
	font-size: var(--font-size-xs);
}

.section-hint {
	margin: 0 0 var(--space-3);
}

.empty-state {
	text-align: center;
	padding: var(--space-8) var(--space-6);
}

.incident-card {
	margin-bottom: var(--space-3);
}

.incident-head {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--space-3);

	.summary {
		flex: 1;
		min-width: 200px;
	}
}

.incident-toggle {
	display: inline-flex;
	align-items: center;
	gap: var(--space-2);
	padding: 0;
	background: none;
	border: none;
	color: var(--text-heading);
	cursor: pointer;

	.rotated {
		transform: rotate(90deg);
	}
}

.incident-body {
	margin-top: var(--space-3);
}

.minor-heading {
	margin: var(--space-4) 0 var(--space-2);
	font-size: 13px;
	color: var(--text-muted);
}

.actions-group {
	display: inline-flex;
	gap: 6px;
}

.row-muted {
	opacity: 0.6;
}

.pagination-wrapper {
	margin-top: var(--space-4);
	display: flex;
	justify-content: flex-end;
}
</style>
