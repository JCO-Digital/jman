<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useIncidentStore } from "../../stores/incidents";
import { useMonitorStore } from "../../stores/monitor";
import { useDataStore } from "../../stores/data";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import { useRouteSection } from "../../composables/useRouteSection";
import AppIcon from "../AppIcon.vue";
import LoadingSpinner from "../LoadingSpinner.vue";
import Pagination from "../Pagination.vue";
import SectionBar from "./SectionBar.vue";
import EmptyState from "./EmptyState.vue";
import MonitorChecksPanel from "./MonitorChecksPanel.vue";
import MonitorRulesPanel from "./MonitorRulesPanel.vue";
import type { Section } from "./sections";
import type { Incident } from "../../types";

defineProps<{
	/** Show only this site's checks. */
	siteId?: string;
}>();

const emit = defineEmits<{
	(e: "clear-site"): void;
}>();

type SectionKey = "incidents" | "checks" | "history" | "rules";
const sectionKeys: SectionKey[] = ["incidents", "checks", "history", "rules"];

const incidentStore = useIncidentStore();
const monitorStore = useMonitorStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

const section = useRouteSection(sectionKeys, "incidents");
const sections = computed<Section<SectionKey>[]>(() => [
	{
		key: "incidents",
		label: "Incidents",
		count: incidentStore.activeByType.uptime ?? 0,
	},
	{ key: "checks", label: "Checks" },
	{ key: "history", label: "History" },
	{ key: "rules", label: "Rules" },
]);

const checksPanel = ref<InstanceType<typeof MonitorChecksPanel> | null>(null);
const rulesPanel = ref<InstanceType<typeof MonitorRulesPanel> | null>(null);

const activePage = ref(1);
const activeLimit = ref(50);
const historyPage = ref(1);
const historyLimit = ref(50);

// Pause modal state
const showPauseModal = ref(false);
const selectedIncident = ref<Incident | null>(null);
const pauseReason = ref("");
const isSubmittingPause = ref(false);

const runningChecks = computed(
	() => monitorStore.checks.filter((c) => c.enabled).length,
);
const pausedChecks = computed(
	() => monitorStore.checks.length - runningChecks.value,
);

onMounted(() => {
	refresh();
	monitorStore.ensureChecks().catch(() => {});
});

watch(section, (s) => {
	if (s === "history") historyPage.value = 1;
	refresh();
});

function refresh() {
	switch (section.value) {
		case "incidents":
			loadActiveIncidents();
			break;
		case "checks":
			checksPanel.value?.refresh();
			break;
		case "history":
			loadHistory();
			break;
		case "rules":
			rulesPanel.value?.refresh();
			break;
	}
}

defineExpose({ refresh });

function loadActiveIncidents() {
	return incidentStore.fetchActiveIncidents(
		activePage.value,
		activeLimit.value,
	);
}

function loadHistory() {
	return incidentStore.fetchHistoryIncidents(
		historyPage.value,
		historyLimit.value,
	);
}

const activeTotalPages = computed(() =>
	Math.max(1, Math.ceil(incidentStore.activeTotal / activeLimit.value)),
);

function handleActivePrevPage() {
	if (activePage.value > 1) {
		activePage.value--;
		loadActiveIncidents();
	}
}

function handleActiveNextPage() {
	if (activePage.value < activeTotalPages.value) {
		activePage.value++;
		loadActiveIncidents();
	}
}

function handleActiveRowsPerPageChange(val: number) {
	activeLimit.value = val;
	activePage.value = 1;
	loadActiveIncidents();
}

// Refetches the current page after an incident leaves the active list, so
// the next page's rows move up, stepping back if the page is now empty.
function reloadActiveAfterRemoval() {
	activePage.value = Math.min(activePage.value, activeTotalPages.value);
	loadActiveIncidents();
}

const historyTotalPages = computed(() =>
	Math.max(1, Math.ceil(incidentStore.historyTotal / historyLimit.value)),
);

function handlePrevPage() {
	if (historyPage.value > 1) {
		historyPage.value--;
		loadHistory();
	}
}

function handleNextPage() {
	if (historyPage.value < historyTotalPages.value) {
		historyPage.value++;
		loadHistory();
	}
}

function handleRowsPerPageChange(val: number) {
	historyLimit.value = val;
	historyPage.value = 1;
	loadHistory();
}

function getSiteIdForDomain(domain: string): string | null {
	const site = dataStore.sites.find(
		(s) => s.domain.toLowerCase() === domain.toLowerCase(),
	);
	return site ? site.id : null;
}

function formatDuration(startTime: string): string {
	const start = new Date(startTime).getTime();
	const now = Date.now();
	const diffSec = Math.max(0, Math.floor((now - start) / 1000));
	const mins = Math.floor(diffSec / 60);
	const hours = Math.floor(mins / 60);
	const days = Math.floor(hours / 24);

	if (days > 0) {
		return `${days}d ${hours % 24}h ago`;
	}
	if (hours > 0) {
		return `${hours}h ${mins % 60}m ago`;
	}
	if (mins > 0) {
		return `${mins}m ago`;
	}
	return "Just now";
}

function formatTimestamp(isoStr?: string | null): string {
	if (!isoStr) return "-";
	const d = new Date(isoStr);
	return d.toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}

async function handleAcknowledge(incident: Incident) {
	try {
		await incidentStore.acknowledgeIncident(incident.id);
		toastStore.addToast(
			`Acknowledged outage for ${incident.domain}`,
			"success",
		);
	} catch (err: unknown) {
		const message =
			err instanceof Error
				? err.message
				: "Failed to acknowledge incident";
		toastStore.addToast(message, "error");
	}
}

async function handleClose(incident: Incident) {
	if (
		!(await confirm(
			`Close the incident for ${incident.domain}? If the site is still down, it will re-alert.`,
			{ confirmLabel: "Close incident" },
		))
	) {
		return;
	}
	try {
		await incidentStore.closeIncident(incident.id);
		reloadActiveAfterRemoval();
		toastStore.addToast(
			`Closed incident for ${incident.domain}`,
			"success",
		);
	} catch (err: unknown) {
		const message =
			err instanceof Error ? err.message : "Failed to close incident";
		toastStore.addToast(message, "error");
	}
}

function openPauseModal(incident: Incident) {
	selectedIncident.value = incident;
	pauseReason.value = `Site down: ${incident.error_message || "Paused from incident"}`;
	showPauseModal.value = true;
}

function closePauseModal() {
	showPauseModal.value = false;
	selectedIncident.value = null;
	pauseReason.value = "";
}

async function submitPause() {
	if (!selectedIncident.value) return;
	isSubmittingPause.value = true;
	try {
		await incidentStore.pauseIncidentCheck(
			selectedIncident.value.id,
			pauseReason.value,
		);
		reloadActiveAfterRemoval();
		toastStore.addToast(
			`Paused monitoring for ${selectedIncident.value.domain} and resolved incident`,
			"success",
		);
		closePauseModal();
	} catch (err: unknown) {
		const message =
			err instanceof Error ? err.message : "Failed to pause check";
		toastStore.addToast(message, "error");
	} finally {
		isSubmittingPause.value = false;
	}
}

const activeIncidents = computed(() => incidentStore.activeIncidents);
const historyIncidents = computed(() => incidentStore.historyIncidents);
const canEdit = computed(() => authStore.canEdit);
</script>

<template>
	<div>
		<SectionBar v-model="section" :sections="sections" />

		<!-- Open incidents -->
		<div v-if="section === 'incidents'" class="card">
			<EmptyState
				v-if="activeIncidents.length === 0"
				icon="✅"
				title="All systems operational"
			>
				No open downtime incidents.
				<template v-if="monitorStore.checksFetched">
					Watching {{ runningChecks }} check{{
						runningChecks === 1 ? "" : "s"
					}}<template v-if="pausedChecks">
						({{ pausedChecks }} paused)</template
					>.
				</template>
			</EmptyState>

			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Domain</th>
							<th>Status</th>
							<th>Down Since</th>
							<th>Error / Reason</th>
							<th>PagerDuty Alert</th>
							<th class="text-right">Actions</th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="inc in activeIncidents"
							:key="inc.id"
							:class="{
								'row-acknowledged':
									inc.status === 'acknowledged',
							}"
						>
							<td class="font-medium">
								<RouterLink
									v-if="getSiteIdForDomain(inc.domain)"
									:to="`/site/${getSiteIdForDomain(inc.domain)}`"
									class="site-link"
								>
									{{ inc.domain }}
								</RouterLink>
								<span v-else>{{ inc.domain }}</span>
							</td>
							<td>
								<span
									v-if="inc.status === 'open'"
									class="status-pill status-open"
								>
									<span class="status-pulse"></span>
									Open
								</span>
								<span
									v-else-if="inc.status === 'acknowledged'"
									class="status-pill status-ack"
									:title="`Acknowledged by ${inc.acknowledged_by || 'user'} at ${formatTimestamp(inc.acknowledged_at)}`"
								>
									<AppIcon name="eye" size="14" />
									Acked by {{ inc.acknowledged_by || "User" }}
								</span>
								<span v-else class="status-pill status-muted">
									{{ inc.status }}
								</span>
							</td>
							<td>
								<div class="time-col">
									<span class="font-medium">{{
										formatDuration(inc.down_since)
									}}</span>
									<span class="sub-text text-xs">{{
										formatTimestamp(inc.down_since)
									}}</span>
								</div>
							</td>
							<td>
								<code class="error-badge">
									{{
										inc.error_message ||
										(inc.error_code
											? `HTTP ${inc.error_code}`
											: "Connection Failed")
									}}
								</code>
							</td>
							<td>
								<span
									v-if="inc.pd_triggered"
									class="pd-tag pd-alerting"
								>
									🚨 Paging On-Call
								</span>
								<span
									v-else-if="inc.status === 'acknowledged'"
									class="pd-tag pd-cancelled"
								>
									✋ Cancelled (Acked)
								</span>
								<span v-else class="pd-tag pd-waiting">
									⏳ 10m buffer
								</span>
							</td>
							<td class="text-right actions-cell">
								<div class="actions-group">
									<button
										v-if="inc.status === 'open'"
										class="btn btn-sm btn-primary"
										title="Acknowledge incident and cancel PagerDuty page"
										@click="handleAcknowledge(inc)"
									>
										<AppIcon name="check" size="14" />
										Acknowledge
									</button>
									<button
										v-if="canEdit && inc.check_id"
										class="btn btn-sm btn-outline"
										title="Pause this monitor check"
										@click="openPauseModal(inc)"
									>
										Pause Check
									</button>
									<button
										class="btn btn-sm btn-outline"
										title="Close incident"
										@click="handleClose(inc)"
									>
										Close
									</button>
								</div>
							</td>
						</tr>
					</tbody>
				</table>

				<div class="pagination-wrapper">
					<Pagination
						:current-page="activePage"
						:total-pages="activeTotalPages"
						:rows-per-page="activeLimit"
						@prev="handleActivePrevPage"
						@next="handleActiveNextPage"
						@update:rows-per-page="handleActiveRowsPerPageChange"
					/>
				</div>
			</div>
		</div>

		<!-- Checks -->
		<MonitorChecksPanel
			v-else-if="section === 'checks'"
			ref="checksPanel"
			:site-id="siteId"
			@clear-site="emit('clear-site')"
		/>

		<!-- Resolved and closed incidents -->
		<div v-else-if="section === 'history'" class="card">
			<div v-if="incidentStore.isLoading" class="loading-container">
				<LoadingSpinner />
			</div>
			<p v-else-if="historyIncidents.length === 0" class="sub-text empty">
				No incident history recorded.
			</p>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Domain</th>
							<th>Result</th>
							<th>Down Since</th>
							<th>Resolved / Closed At</th>
							<th>Resolved By</th>
							<th>Error</th>
						</tr>
					</thead>
					<tbody>
						<tr v-for="inc in historyIncidents" :key="inc.id">
							<td class="font-medium">
								<RouterLink
									v-if="getSiteIdForDomain(inc.domain)"
									:to="`/site/${getSiteIdForDomain(inc.domain)}`"
									class="site-link"
								>
									{{ inc.domain }}
								</RouterLink>
								<span v-else>{{ inc.domain }}</span>
							</td>
							<td>
								<span
									v-if="inc.status === 'resolved'"
									class="status-pill status-resolved"
								>
									<AppIcon name="check" size="14" />
									Resolved
								</span>
								<span
									v-else-if="inc.status === 'closed'"
									class="status-pill status-closed"
								>
									Closed
								</span>
								<span v-else class="status-pill status-muted">
									{{ inc.status }}
								</span>
							</td>
							<td>
								<span class="sub-text">{{
									formatTimestamp(inc.down_since)
								}}</span>
							</td>
							<td>
								<span class="sub-text">{{
									formatTimestamp(inc.resolved_at)
								}}</span>
							</td>
							<td>
								<span>{{
									inc.resolved_by ||
									(inc.status === "resolved"
										? "Auto (Recovered)"
										: "-")
								}}</span>
							</td>
							<td>
								<code class="error-badge">
									{{ inc.error_message || "-" }}
								</code>
							</td>
						</tr>
					</tbody>
				</table>

				<div class="pagination-wrapper">
					<Pagination
						:current-page="historyPage"
						:total-pages="historyTotalPages"
						:rows-per-page="historyLimit"
						@prev="handlePrevPage"
						@next="handleNextPage"
						@update:rows-per-page="handleRowsPerPageChange"
					/>
				</div>
			</div>
		</div>

		<!-- New-check rules -->
		<MonitorRulesPanel v-else-if="section === 'rules'" ref="rulesPanel" />

		<!-- Pause Modal -->
		<div
			v-if="showPauseModal"
			class="modal-overlay"
			@click.self="closePauseModal"
		>
			<div class="modal-content card">
				<div class="modal-header">
					<h2>Pause monitoring for {{ selectedIncident?.domain }}</h2>
					<button class="icon-btn" @click="closePauseModal">
						<AppIcon name="x" size="18" />
					</button>
				</div>
				<form @submit.prevent="submitPause">
					<div class="modal-body">
						<p class="sub-text">
							Pausing this check stops it running and immediately
							resolves this incident. Resume it from the site's
							uptime history.
						</p>

						<div class="form-group">
							<label for="pauseReason">Reason</label>
							<input
								id="pauseReason"
								v-model="pauseReason"
								type="text"
								class="input-field"
								required
								placeholder="e.g. Planned maintenance, site decommissioned"
							/>
						</div>
					</div>
					<div class="modal-footer">
						<button
							type="button"
							class="btn btn-outline"
							:disabled="isSubmittingPause"
							@click="closePauseModal"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="isSubmittingPause"
						>
							<LoadingSpinner v-if="isSubmittingPause" small />
							<span v-else>Confirm & Pause</span>
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>

<style scoped>
.empty {
	text-align: center;
	padding: var(--space-6);
}

.row-acknowledged {
	background: rgba(234, 179, 8, 0.03);
}

.site-link {
	color: var(--primary);
	text-decoration: none;
	font-weight: 600;
}

.site-link:hover {
	text-decoration: underline;
}

.status-pill {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	padding: 4px 10px;
	border-radius: 999px;
	font-size: 12px;
	font-weight: 600;
}

.status-open {
	background: rgba(220, 38, 38, 0.12);
	color: var(--danger, #dc2626);
	border: 1px solid rgba(220, 38, 38, 0.3);
}

.status-pulse {
	width: 8px;
	height: 8px;
	border-radius: 50%;
	background: var(--danger, #dc2626);
	box-shadow: 0 0 0 0 rgba(220, 38, 38, 0.7);
	animation: pulse 1.8s infinite;
}

@keyframes pulse {
	0% {
		box-shadow: 0 0 0 0 rgba(220, 38, 38, 0.7);
	}
	70% {
		box-shadow: 0 0 0 6px rgba(220, 38, 38, 0);
	}
	100% {
		box-shadow: 0 0 0 0 rgba(220, 38, 38, 0);
	}
}

.status-ack {
	background: rgba(234, 179, 8, 0.12);
	color: #ca8a04;
	border: 1px solid rgba(234, 179, 8, 0.3);
}

.status-resolved {
	background: rgba(22, 163, 74, 0.12);
	color: #16a34a;
	border: 1px solid rgba(22, 163, 74, 0.3);
}

.status-closed {
	background: var(--bg-body);
	color: var(--text-muted);
	border: 1px solid var(--border-color);
}

.status-muted {
	background: var(--bg-body);
	color: var(--text-muted);
}

.time-col {
	display: flex;
	flex-direction: column;
	gap: 2px;
}

.error-badge {
	display: inline-block;
	background: var(--bg-body);
	padding: 4px 8px;
	border-radius: 4px;
	font-size: 12px;
	color: var(--danger, #dc2626);
	border: 1px solid var(--border-color);
	max-width: 240px;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.pd-tag {
	display: inline-flex;
	align-items: center;
	font-size: 12px;
	font-weight: 500;
	padding: 3px 8px;
	border-radius: 4px;
}

.pd-alerting {
	background: rgba(220, 38, 38, 0.15);
	color: var(--danger, #dc2626);
	font-weight: 600;
}

.pd-cancelled {
	background: var(--bg-body);
	color: var(--text-muted);
}

.pd-waiting {
	background: rgba(59, 130, 246, 0.1);
	color: var(--primary, #2563eb);
}

.actions-group {
	display: inline-flex;
	align-items: center;
	gap: 6px;
}

.pagination-wrapper {
	margin-top: 16px;
	display: flex;
	justify-content: flex-end;
}

.modal-body {
	display: flex;
	flex-direction: column;
	gap: 16px;
	margin-bottom: 24px;
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-group label {
	font-size: 13px;
	font-weight: 500;
	color: var(--text-heading);
}

.input-field {
	padding: 8px 12px;
	border: 1px solid var(--border-color);
	border-radius: 6px;
	background: var(--bg-body);
	color: var(--text-heading);
	font-size: 14px;
}

.modal-footer {
	display: flex;
	justify-content: flex-end;
	gap: 12px;
}

.text-xs {
	font-size: 11px;
}
</style>
