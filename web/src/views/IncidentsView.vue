<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useIncidentStore } from "../stores/incidents";
import { useDataStore } from "../stores/data";
import { useAuthStore } from "../stores/auth";
import { useToastStore } from "../stores/toast";
import ViewHeader from "../components/ViewHeader.vue";
import AppIcon from "../components/AppIcon.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import Pagination from "../components/Pagination.vue";
import type { Incident } from "../types";

const incidentStore = useIncidentStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toastStore = useToastStore();

const activeTab = ref<"active" | "history">("active");
const historyPage = ref(1);
const historyLimit = ref(20);

// Ignore modal state
const showIgnoreModal = ref(false);
const selectedIncident = ref<Incident | null>(null);
const ignoreReason = ref("");
const ignoreMonitor = ref(true);
const ignoreVuln = ref(false);
const isSubmittingIgnore = ref(false);

onMounted(() => {
	loadData();
});

function loadData() {
	incidentStore.fetchActiveIncidents();
	if (activeTab.value === "history") {
		incidentStore.fetchHistoryIncidents(
			historyPage.value,
			historyLimit.value,
		);
	}
}

function switchTab(tab: "active" | "history") {
	activeTab.value = tab;
	if (tab === "history") {
		historyPage.value = 1;
		incidentStore.fetchHistoryIncidents(
			historyPage.value,
			historyLimit.value,
		);
	} else {
		incidentStore.fetchActiveIncidents();
	}
}

const historyTotalPages = computed(() => {
	return Math.max(
		1,
		Math.ceil(incidentStore.historyTotal / historyLimit.value),
	);
});

function handlePrevPage() {
	if (historyPage.value > 1) {
		historyPage.value--;
		incidentStore.fetchHistoryIncidents(
			historyPage.value,
			historyLimit.value,
		);
	}
}

function handleNextPage() {
	if (historyPage.value < historyTotalPages.value) {
		historyPage.value++;
		incidentStore.fetchHistoryIncidents(
			historyPage.value,
			historyLimit.value,
		);
	}
}

function handleRowsPerPageChange(val: number) {
	historyLimit.value = val;
	historyPage.value = 1;
	incidentStore.fetchHistoryIncidents(historyPage.value, historyLimit.value);
}

function getSiteIdForDomain(domain: string): number | null {
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
		!confirm(
			`Are you sure you want to close the incident for ${incident.domain}? If the site is still down, it will re-alert.`,
		)
	) {
		return;
	}
	try {
		await incidentStore.closeIncident(incident.id);
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

function openIgnoreModal(incident: Incident) {
	selectedIncident.value = incident;
	ignoreReason.value = `Site down: ${incident.error_message || "Manual ignore from incident"}`;
	ignoreMonitor.value = true;
	ignoreVuln.value = false;
	showIgnoreModal.value = true;
}

function closeIgnoreModal() {
	showIgnoreModal.value = false;
	selectedIncident.value = null;
	ignoreReason.value = "";
}

async function submitIgnore() {
	if (!selectedIncident.value) return;
	isSubmittingIgnore.value = true;
	try {
		await incidentStore.ignoreIncident(
			selectedIncident.value.id,
			ignoreReason.value,
			ignoreMonitor.value,
			ignoreVuln.value,
		);
		toastStore.addToast(
			`Added ${selectedIncident.value.domain} to ignore list and closed incident`,
			"success",
		);
		closeIgnoreModal();
	} catch (err: unknown) {
		const message =
			err instanceof Error ? err.message : "Failed to ignore site";
		toastStore.addToast(message, "error");
	} finally {
		isSubmittingIgnore.value = false;
	}
}

const activeIncidents = computed(() => incidentStore.activeIncidents);
const historyIncidents = computed(() => incidentStore.historyIncidents);
const canEdit = computed(() => authStore.canEdit);
</script>

<template>
	<div class="incidents-view">
		<ViewHeader title="Incidents">
			<template #actions>
				<button class="btn btn-outline btn-sm" @click="loadData">
					<AppIcon name="refresh" size="14" />
					Refresh
				</button>
			</template>
		</ViewHeader>

		<div class="incidents-tabs">
			<button
				class="tab-btn"
				:class="{ active: activeTab === 'active' }"
				@click="switchTab('active')"
			>
				Active Incidents
				<span
					v-if="incidentStore.activeCount > 0"
					class="tab-badge tab-badge-danger"
				>
					{{ incidentStore.activeCount }}
				</span>
			</button>
			<button
				class="tab-btn"
				:class="{ active: activeTab === 'history' }"
				@click="switchTab('history')"
			>
				Incident History
			</button>
		</div>

		<!-- Active Incidents Tab -->
		<div v-if="activeTab === 'active'" class="card">
			<div v-if="activeIncidents.length === 0" class="empty-state">
				<div class="empty-icon">✅</div>
				<h3>All Systems Operational</h3>
				<p class="sub-text">No active downtime incidents reported.</p>
			</div>

			<div v-else class="table-responsive">
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
										v-if="canEdit"
										class="btn btn-sm btn-outline"
										title="Add to monitor ignore list"
										@click="openIgnoreModal(inc)"
									>
										Ignore Site
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
			</div>
		</div>

		<!-- Incident History Tab -->
		<div v-if="activeTab === 'history'" class="card">
			<div v-if="incidentStore.isLoading" class="loading-container">
				<LoadingSpinner />
			</div>
			<div v-else-if="historyIncidents.length === 0" class="empty-state">
				<p class="sub-text">No incident history recorded.</p>
			</div>
			<div v-else class="table-responsive">
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

		<!-- Ignore Modal -->
		<div
			v-if="showIgnoreModal"
			class="modal-overlay"
			@click.self="closeIgnoreModal"
		>
			<div class="modal-card">
				<div class="modal-header">
					<h2>Ignore Site {{ selectedIncident?.domain }}</h2>
					<button class="icon-btn" @click="closeIgnoreModal">
						<AppIcon name="x" size="18" />
					</button>
				</div>
				<form @submit.prevent="submitIgnore">
					<div class="modal-body">
						<p class="sub-text">
							Adding this site to the ignore list will suppress
							alerts and immediately close this incident.
						</p>

						<div class="form-group">
							<label for="ignoreReason">Reason</label>
							<input
								id="ignoreReason"
								v-model="ignoreReason"
								type="text"
								class="input-field"
								required
								placeholder="e.g. Planned maintenance, site decommissioned"
							/>
						</div>

						<div class="form-checkbox-group">
							<label class="checkbox-label">
								<input
									v-model="ignoreMonitor"
									type="checkbox"
								/>
								<span>Ignore in Uptime Monitoring</span>
							</label>
							<label class="checkbox-label">
								<input v-model="ignoreVuln" type="checkbox" />
								<span>Ignore in Vulnerability Scanning</span>
							</label>
						</div>
					</div>
					<div class="modal-footer">
						<button
							type="button"
							class="btn btn-outline"
							:disabled="isSubmittingIgnore"
							@click="closeIgnoreModal"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="isSubmittingIgnore"
						>
							<LoadingSpinner v-if="isSubmittingIgnore" small />
							<span v-else>Confirm & Ignore</span>
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>

<style scoped>
.incidents-view {
	max-width: 1400px;
	margin: 0 auto;
}

.incidents-tabs {
	display: flex;
	gap: 8px;
	margin-bottom: 16px;
	border-bottom: 1px solid var(--border-color);
	padding-bottom: 8px;
}

.tab-btn {
	display: inline-flex;
	align-items: center;
	gap: 8px;
	padding: 8px 16px;
	background: none;
	border: 1px solid transparent;
	border-radius: 6px;
	color: var(--text-muted);
	font-weight: 500;
	font-size: 14px;
	cursor: pointer;
	transition: all 0.2s;
}

.tab-btn:hover {
	color: var(--text-heading);
	background: var(--bg-card);
}

.tab-btn.active {
	color: var(--text-heading);
	background: var(--bg-card);
	border-color: var(--border-color);
}

.tab-badge {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	padding: 2px 8px;
	border-radius: 999px;
	font-size: 12px;
	font-weight: 600;
}

.tab-badge-danger {
	background: var(--danger, #dc2626);
	color: #fff;
}

.empty-state {
	text-align: center;
	padding: 48px 24px;
}

.empty-icon {
	font-size: 36px;
	margin-bottom: 12px;
}

.empty-state h3 {
	margin: 0 0 8px;
	color: var(--text-heading);
}

.table-responsive {
	overflow-x: auto;
}

.data-table {
	width: 100%;
	border-collapse: collapse;
	text-align: left;
	font-size: 14px;
}

.data-table th {
	padding: 12px 16px;
	color: var(--text-muted);
	font-weight: 600;
	font-size: 13px;
	border-bottom: 1px solid var(--border-color);
	background: var(--bg-card);
}

.data-table td {
	padding: 14px 16px;
	border-bottom: 1px solid var(--border-color);
	vertical-align: middle;
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

.loading-container {
	display: flex;
	justify-content: center;
	padding: 40px;
}

.pagination-wrapper {
	margin-top: 16px;
	display: flex;
	justify-content: flex-end;
}

.modal-overlay {
	position: fixed;
	inset: 0;
	background: rgba(0, 0, 0, 0.5);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 999;
}

.modal-card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 8px;
	width: 100%;
	max-width: 500px;
	padding: 24px;
	box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1);
}

.modal-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 16px;
}

.modal-header h2 {
	margin: 0;
	font-size: 18px;
	color: var(--text-heading);
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

.form-checkbox-group {
	display: flex;
	flex-direction: column;
	gap: 8px;
}

.checkbox-label {
	display: inline-flex;
	align-items: center;
	gap: 8px;
	font-size: 13px;
	color: var(--text-heading);
	cursor: pointer;
}

.modal-footer {
	display: flex;
	justify-content: flex-end;
	gap: 12px;
}

.text-right {
	text-align: right;
}

.text-xs {
	font-size: 11px;
}

.font-medium {
	font-weight: 500;
}
</style>
