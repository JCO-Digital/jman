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
import ScanFindingsTable from "./ScanFindingsTable.vue";
import ScanTriageModal from "./ScanTriageModal.vue";
import { severityClass } from "./scan";
import type { Incident, ScanFinding } from "../../types";

type Section = "incidents" | "informational" | "history" | "allowlist" | "acks";
const sections: { key: Section; label: string }[] = [
	{ key: "incidents", label: "Incidents" },
	{ key: "informational", label: "Informational" },
	{ key: "history", label: "History" },
	{ key: "allowlist", label: "Allowlist" },
	{ key: "acks", label: "Acknowledged paths" },
];

const scanStore = useScanStore();
const incidentStore = useIncidentStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();
const route = useRoute();

const section = ref<Section>("incidents");
const loading = ref(false);
const canTriage = computed(() => authStore.canExecute);

// Findings of the expanded incidents, by incident ID.
const expanded = ref(new Set<number>());
const incidentFindings = ref<Record<number, ScanFinding[]>>({});
const informational = ref<ScanFinding[]>([]);
const historyPage = ref(1);
const historyLimit = ref(20);
const showRemoved = ref(false);

const triageMode = ref<"allowlist" | "acknowledge" | null>(null);
const triageFindings = ref<ScanFinding[]>([]);

const historyTotalPages = computed(() =>
	Math.max(1, Math.ceil(scanStore.historyTotal / historyLimit.value)),
);

onMounted(async () => {
	const id = Number(route.query.incident);
	if (id) expanded.value.add(id);
	await refresh();
});

watch(section, () => refresh());
watch(showRemoved, () => refresh());

async function refresh() {
	loading.value = true;
	try {
		switch (section.value) {
			case "incidents":
				await scanStore.fetchActiveIncidents();
				await Promise.all([...expanded.value].map(loadFindings));
				break;
			case "informational":
				informational.value = await scanStore.fetchFindings({
					status: "open",
					unassigned: true,
					limit: 1000,
				});
				break;
			case "history":
				await scanStore.fetchHistoryIncidents(
					historyPage.value,
					historyLimit.value,
				);
				await Promise.all([...expanded.value].map(loadFindings));
				break;
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
	if (
		!(await confirm(
			`Close the security incident for ${inc.domain}? Its open findings are closed too. If a file with the same content is found again, the incident reopens.`,
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

async function removeAllowlist(id: number, sha: string) {
	if (
		!(await confirm(
			`Remove ${sha.slice(0, 12)}… from the allowlist? Files with this content will be flagged again; findings it already covered stay allowlisted.`,
			{ confirmLabel: "Remove", danger: true },
		))
	) {
		return;
	}
	try {
		await scanStore.removeAllowlist(id);
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

function changeHistoryPage(page: number) {
	historyPage.value = page;
	refresh();
}

function changeHistoryLimit(limit: number) {
	historyLimit.value = limit;
	historyPage.value = 1;
	refresh();
}
</script>

<template>
	<div class="security-panel">
		<div class="flex-row section-bar">
			<div class="segmented">
				<button
					v-for="s in sections"
					:key="s.key"
					class="btn btn-sm"
					:class="section === s.key ? 'btn-primary' : 'btn-outline'"
					@click="section = s.key"
				>
					{{ s.label }}
					<span
						v-if="
							s.key === 'incidents' &&
							scanStore.activeIncidents.length
						"
						class="count"
					>
						{{ scanStore.activeIncidents.length }}
					</span>
				</button>
			</div>
			<label
				v-if="section === 'allowlist' || section === 'acks'"
				class="checkbox-label"
			>
				<input v-model="showRemoved" type="checkbox" />
				Show removed
			</label>
			<LoadingSpinner v-if="loading" small />
		</div>

		<!-- Active incidents -->
		<template v-if="section === 'incidents'">
			<div
				v-if="!loading && scanStore.activeIncidents.length === 0"
				class="card empty-state"
			>
				<div class="empty-icon">🛡️</div>
				<h3>No security incidents</h3>
				<p class="sub-text">
					Findings of medium severity and above open an incident. Low
					ones are under Informational.
				</p>
			</div>
			<div
				v-for="inc in scanStore.activeIncidents"
				:key="inc.id"
				class="card incident-card"
			>
				<div class="incident-head">
					<button
						class="btn-text incident-toggle"
						@click="toggleIncident(inc)"
					>
						<AppIcon
							name="chevron-right"
							size="14"
							:class="{ rotated: expanded.has(inc.id) }"
						/>
						<span
							class="status-badge badge-sm"
							:class="severityClass(inc.severity)"
						>
							{{ inc.severity || "-" }}
						</span>
						<span class="font-medium">{{ inc.domain }}</span>
						<span class="sub-text">#{{ inc.id }}</span>
					</button>
					<span class="sub-text summary">{{
						inc.error_message
					}}</span>
					<span
						class="sub-text"
						:title="`Since ${formatTime(inc.down_since)}`"
					>
						since {{ formatTime(inc.down_since) }}
					</span>
					<span
						v-if="inc.status === 'acknowledged'"
						class="status-badge badge-sm warning"
						:title="`at ${formatTime(inc.acknowledged_at)}`"
					>
						Acked by {{ inc.acknowledged_by || "user" }}
					</span>
					<span
						v-if="inc.pd_triggered"
						class="status-badge badge-sm error"
					>
						Paging
					</span>
					<div class="actions-group">
						<RouterLink
							v-if="inc.site_id"
							:to="`/site/${inc.site_id}`"
							class="btn btn-sm btn-text"
						>
							Site
						</RouterLink>
						<button
							v-if="inc.status === 'open'"
							class="btn btn-sm btn-outline"
							@click="acknowledgeIncident(inc)"
						>
							Acknowledge
						</button>
						<button
							class="btn btn-sm btn-outline"
							@click="closeIncident(inc)"
						>
							Close
						</button>
					</div>
				</div>
				<div v-if="expanded.has(inc.id)" class="incident-body">
					<LoadingSpinner v-if="!incidentFindings[inc.id]" small />
					<ScanFindingsTable
						v-else
						:findings="incidentFindings[inc.id] ?? []"
						:can-triage="canTriage"
						@allowlist="(f) => openTriage('allowlist', [f])"
						@acknowledge="(fs) => openTriage('acknowledge', fs)"
						@dismiss="dismiss"
					/>
				</div>
			</div>
		</template>

		<!-- Low findings, not in incidents -->
		<div v-else-if="section === 'informational'" class="card">
			<p class="sub-text section-hint">
				Low-severity findings, such as stray database dumps and backups,
				don't open incidents. Clean up the file, acknowledge its path if
				it's meant to be there, or dismiss it.
			</p>
			<p
				v-if="!loading && informational.length === 0"
				class="sub-text empty-state"
			>
				Nothing to look at.
			</p>
			<ScanFindingsTable
				v-else
				:findings="informational"
				:can-triage="canTriage"
				show-site
				@allowlist="(f) => openTriage('allowlist', [f])"
				@acknowledge="(fs) => openTriage('acknowledge', fs)"
				@dismiss="dismiss"
			/>
		</div>

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
									/>
								</td>
							</tr>
						</template>
					</tbody>
				</table>
				<div class="pagination-wrapper">
					<Pagination
						:current-page="historyPage"
						:total-pages="historyTotalPages"
						:rows-per-page="historyLimit"
						@prev="changeHistoryPage(historyPage - 1)"
						@next="changeHistoryPage(historyPage + 1)"
						@update:rows-per-page="changeHistoryLimit"
					/>
				</div>
			</div>
		</div>

		<!-- Allowlist -->
		<div v-else-if="section === 'allowlist'" class="card">
			<p class="sub-text section-hint">
				File content trusted on every site. Allowlist a finding to add
				an entry.
			</p>
			<p
				v-if="!loading && scanStore.allowlist.length === 0"
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
							v-for="e in scanStore.allowlist"
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
									@click="removeAllowlist(e.id, e.sha256)"
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
				Paths of stray, foreign-owned or unreadable files that are meant
				to be there, per site.
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
.section-bar {
	margin-bottom: var(--space-4);
}

.segmented {
	display: flex;
	flex-wrap: wrap;
	gap: var(--space-2);

	.count {
		margin-left: var(--space-1);
		font-weight: 700;
	}
}

.section-hint {
	margin: 0 0 var(--space-3);
}

.empty-state {
	text-align: center;
	padding: var(--space-8) var(--space-6);

	h3 {
		margin: 0 0 var(--space-2);
		color: var(--text-heading);
	}
}

.empty-icon {
	font-size: 36px;
	margin-bottom: var(--space-3);
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
