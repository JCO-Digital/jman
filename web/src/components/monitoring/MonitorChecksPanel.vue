<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { useMonitorStore } from "../../stores/monitor";
import { useDataStore } from "../../stores/data";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import AppIcon from "../AppIcon.vue";
import LoadingSpinner from "../LoadingSpinner.vue";
import MonitorCheckFormModal from "./MonitorCheckFormModal.vue";
import HeartbeatCheckFormModal from "./HeartbeatCheckFormModal.vue";
import PanelToolbar from "./PanelToolbar.vue";
import type { MonitorCheck } from "../../types";

const props = defineProps<{
	/** Show only this site's checks. */
	siteId?: string;
}>();

const emit = defineEmits<{
	(e: "clear-site"): void;
}>();

const monitorStore = useMonitorStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

type CheckState =
	| "orphaned"
	| "paused"
	| "down"
	| "investigating"
	| "up"
	| "waiting";

const stateLabels: Record<CheckState, string> = {
	orphaned: "Orphaned",
	paused: "Paused",
	down: "Down",
	investigating: "Investigating",
	up: "Up",
	waiting: "Waiting",
};

// Order in the table: new checks still waiting to be seen up first, then
// what needs attention.
const stateOrder: Record<CheckState, number> = {
	waiting: 0,
	down: 1,
	investigating: 2,
	up: 3,
	paused: 4,
	orphaned: 5,
};

const isLoading = ref(false);
const search = ref("");
const stateFilter = ref<"" | CheckState>("");
const formVisible = ref(false);
const heartbeatFormVisible = ref(false);
const editing = ref<MonitorCheck | null>(null);
const busyId = ref<string | null>(null);

onMounted(() => {
	refresh();
	dataStore.initData();
});

async function refresh() {
	isLoading.value = true;
	try {
		await Promise.all([
			monitorStore.fetchChecks(),
			monitorStore.fetchCheckStatuses(),
			monitorStore.fetchRules(),
		]);
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load checks",
			"error",
		);
	} finally {
		isLoading.value = false;
	}
}

defineExpose({ refresh });

function checkState(c: MonitorCheck): CheckState {
	if (c.orphaned_at) return "orphaned";
	if (!c.enabled) return "paused";
	const st = monitorStore.statusByCheck[c.id];
	// Not probed or checked in yet, or not yet seen up: it can't alert.
	if (!st || !st.last_checked || st.current_mode === "waiting") {
		return "waiting";
	}
	if (st.is_down) return "down";
	if (st.failure_count > 0) return "investigating";
	return "up";
}

function checkLabel(c: MonitorCheck): string {
	if (c.name) return c.name;
	if (c.type === "heartbeat") return "Check-in";
	if (!c.domain) return "(unknown site)";
	return c.path && c.path !== "/" ? c.domain + c.path : c.domain;
}

function expectationLabel(c: MonitorCheck): string {
	if (c.type === "heartbeat") return "Check-in";
	return c.expect === "json" ? "JSON" : "HTML";
}

/**
 * The condition behind an expectation, shown on hover so a long assertion
 * doesn't widen the table.
 */
function expectationDetail(c: MonitorCheck): string | undefined {
	if (c.type === "heartbeat") {
		const threshold = c.fail_threshold ?? 1;
		return threshold > 1
			? `Alerts after ${threshold} failed check-ins in a row`
			: undefined;
	}
	if (c.assert_op === "exists") return `${c.assert_path} exists`;
	if (c.assert_op === "equals") {
		return `${c.assert_path} = ${JSON.stringify(c.assert_value)}`;
	}
	return undefined;
}

/** "1 day", "90 minutes": a duration in its largest even unit. */
function formatSeconds(seconds: number): string {
	const units: [number, string][] = [
		[86400, "day"],
		[3600, "hour"],
		[60, "minute"],
	];
	for (const [size, unit] of units) {
		if (seconds >= size && seconds % size === 0) {
			const n = seconds / size;
			return `${n} ${unit}${n === 1 ? "" : "s"}`;
		}
	}
	return `${seconds} seconds`;
}

/** The schedule line under a heartbeat check's name. */
function heartbeatSchedule(c: MonitorCheck): string {
	let text = `Every ${formatSeconds(c.interval_seconds ?? 0)}`;
	if (c.grace_seconds) text += ` + ${formatSeconds(c.grace_seconds)} grace`;
	return text;
}

function formatShort(iso: string): string {
	return new Date(iso).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}

/** "Running since …" while a heartbeat check's job has started but not finished. */
function runningSince(c: MonitorCheck): string | null {
	const st = monitorStore.statusByCheck[c.id];
	if (!st?.last_started_at) return null;
	if (st.last_checked && st.last_checked >= st.last_started_at) return null;
	return formatShort(st.last_started_at);
}

/** When a heartbeat check misses its next check-in, if it has a deadline. */
function dueBy(c: MonitorCheck): string | null {
	const st = monitorStore.statusByCheck[c.id];
	if (!st?.next_check_at || !st.last_checked || st.is_down) return null;
	if (st.next_check_at.startsWith("9999")) return null;
	return formatShort(st.next_check_at);
}

/** "Started paused by rule …" for a check a rule holds paused. */
function pausedByRule(c: MonitorCheck): string | undefined {
	if (c.enabled || !c.paused_by_rule_id) return undefined;
	const rule = monitorStore.rules.find((r) => r.id === c.paused_by_rule_id);
	if (!rule) return undefined;
	const what =
		rule.match_type === "server"
			? `server ${rule.server_name || rule.value}`
			: rule.value;
	return `Paused by the rule for ${what}`;
}

function siteDomain(siteId?: string): string | undefined {
	if (!siteId) return undefined;
	return dataStore.sites.find((s) => s.id === siteId)?.domain;
}

const filterSiteDomain = computed(() => siteDomain(props.siteId));

const rows = computed(() => {
	const q = search.value.trim().toLowerCase();
	return monitorStore.checks
		.filter((c) => !props.siteId || c.site_id === props.siteId)
		.map((c) => ({ check: c, state: checkState(c), label: checkLabel(c) }))
		.filter((r) => !stateFilter.value || r.state === stateFilter.value)
		.filter(
			(r) =>
				!q ||
				r.label.toLowerCase().includes(q) ||
				(r.check.url ?? "").toLowerCase().includes(q),
		)
		.sort(
			(a, b) =>
				stateOrder[a.state] - stateOrder[b.state] ||
				a.label.localeCompare(b.label),
		);
});

const counts = computed(() => {
	const out: Partial<Record<CheckState, number>> = {};
	for (const c of monitorStore.checks) {
		if (props.siteId && c.site_id !== props.siteId) continue;
		const st = checkState(c);
		out[st] = (out[st] ?? 0) + 1;
	}
	return out;
});

function lastChecked(c: MonitorCheck): string {
	const iso = monitorStore.statusByCheck[c.id]?.last_checked;
	return iso ? formatShort(iso) : "-";
}

function openCreate(type: "http" | "heartbeat") {
	editing.value = null;
	if (type === "heartbeat") heartbeatFormVisible.value = true;
	else formVisible.value = true;
}

function openEdit(c: MonitorCheck) {
	editing.value = c;
	if (c.type === "heartbeat") heartbeatFormVisible.value = true;
	else formVisible.value = true;
}

async function togglePaused(c: MonitorCheck) {
	busyId.value = c.id;
	try {
		await monitorStore.setCheckPaused(c.id, c.enabled);
		await monitorStore.fetchCheckStatuses();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to update the check",
			"error",
		);
	} finally {
		busyId.value = null;
	}
}

async function remove(c: MonitorCheck) {
	const what = c.orphaned_at
		? `Clean up the orphaned check ${checkLabel(c)}?`
		: `Delete the check ${checkLabel(c)}?`;
	if (
		!(await confirm(`${what} Its history and incidents are deleted too.`, {
			confirmLabel: "Delete",
			danger: true,
		}))
	) {
		return;
	}
	busyId.value = c.id;
	try {
		await monitorStore.deleteCheck(c.id);
		toastStore.addToast("Check deleted", "success");
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to delete the check",
			"error",
		);
	} finally {
		busyId.value = null;
	}
}
</script>

<template>
	<div class="card">
		<PanelToolbar>
			<input
				v-model="search"
				type="search"
				class="input-field"
				placeholder="Search checks"
			/>
			<select v-model="stateFilter" class="input-field">
				<option value="">All states</option>
				<option
					v-for="(label, key) in stateLabels"
					:key="key"
					:value="key"
				>
					{{ label }} ({{ counts[key] ?? 0 }})
				</option>
			</select>
			<span v-if="siteId" class="site-filter">
				Site: {{ filterSiteDomain || siteId }}
				<button
					class="icon-btn"
					title="Show all checks"
					@click="emit('clear-site')"
				>
					<AppIcon name="x" size="14" />
				</button>
			</span>
			<template v-if="authStore.canEdit" #actions>
				<button
					class="btn btn-outline btn-sm"
					title="A job reports in; alerts when it fails or doesn't"
					@click="openCreate('heartbeat')"
				>
					Add Check-in
				</button>
				<button
					class="btn btn-primary btn-sm"
					@click="openCreate('http')"
				>
					Add Check
				</button>
			</template>
		</PanelToolbar>

		<div
			v-if="isLoading && !monitorStore.checks.length"
			class="loading-container"
		>
			<LoadingSpinner />
		</div>
		<div v-else-if="rows.length === 0" class="empty-state">
			<p class="sub-text">No checks match.</p>
		</div>
		<div v-else class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Check</th>
						<th>Site</th>
						<th>Expects</th>
						<th>State</th>
						<th>Last Checked</th>
						<th v-if="authStore.canEdit" class="text-right">
							Actions
						</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="row in rows"
						:key="row.check.id"
						:class="{ 'row-orphaned': row.state === 'orphaned' }"
					>
						<td>
							<div class="check-cell">
								<span class="font-medium">{{ row.label }}</span>
								<span v-if="row.check.url" class="check-url">
									{{ row.check.url }}
								</span>
								<span
									v-if="row.check.type === 'heartbeat'"
									class="check-url"
								>
									{{ heartbeatSchedule(row.check) }}
									<template v-if="dueBy(row.check)">
										· due by {{ dueBy(row.check) }}
									</template>
								</span>
								<span
									v-if="runningSince(row.check)"
									class="sub-text text-xs"
								>
									Running since {{ runningSince(row.check) }}
								</span>
								<span
									v-if="row.check.secret_headers.length"
									class="sub-text text-xs"
									:title="
										row.check.secret_headers
											.map((s) => s.name)
											.join(', ')
									"
								>
									<AppIcon name="lock" size="12" />
									{{ row.check.secret_headers.length }} secret
									header{{
										row.check.secret_headers.length === 1
											? ""
											: "s"
									}}
								</span>
							</div>
						</td>
						<td>
							<RouterLink
								v-if="siteDomain(row.check.site_id)"
								:to="`/site/${row.check.site_id}`"
								class="site-link"
							>
								{{ siteDomain(row.check.site_id) }}
							</RouterLink>
							<span v-else class="sub-text">
								{{ row.check.site_id ? "Removed" : "-" }}
							</span>
							<span
								v-if="row.check.is_default"
								class="sub-text text-xs default-tag"
							>
								default
							</span>
						</td>
						<td>
							<code
								class="expect-badge"
								:class="{
									'has-detail': expectationDetail(row.check),
								}"
								:title="expectationDetail(row.check)"
								>{{ expectationLabel(row.check) }}</code
							>
						</td>
						<td>
							<span
								class="state-pill"
								:class="`state-${row.state}`"
							>
								{{ stateLabels[row.state] }}
							</span>
							<span
								v-if="pausedByRule(row.check)"
								class="sub-text text-xs rule-tag"
								:title="pausedByRule(row.check)"
							>
								by rule
							</span>
						</td>
						<td>
							<span class="sub-text">{{
								lastChecked(row.check)
							}}</span>
						</td>
						<td v-if="authStore.canEdit" class="text-right">
							<div class="actions-group">
								<template v-if="row.state !== 'orphaned'">
									<button
										class="btn btn-sm btn-outline"
										@click="openEdit(row.check)"
									>
										<AppIcon name="edit" size="14" />
										Edit
									</button>
									<button
										class="btn btn-sm btn-outline"
										:disabled="busyId === row.check.id"
										@click="togglePaused(row.check)"
									>
										{{
											row.check.enabled
												? "Pause"
												: "Resume"
										}}
									</button>
								</template>
								<button
									v-if="
										row.state === 'orphaned' ||
										!row.check.is_default
									"
									class="btn btn-sm btn-outline"
									:disabled="busyId === row.check.id"
									:title="
										row.state === 'orphaned'
											? 'Delete this check, its history and incidents'
											: 'Delete this check'
									"
									@click="remove(row.check)"
								>
									<AppIcon name="trash" size="14" />
									{{
										row.state === "orphaned"
											? "Clean up"
											: "Delete"
									}}
								</button>
							</div>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<MonitorCheckFormModal
			:visible="formVisible"
			:edit-check="editing"
			@close="formVisible = false"
			@saved="monitorStore.fetchCheckStatuses()"
		/>
		<HeartbeatCheckFormModal
			:visible="heartbeatFormVisible"
			:edit-check="editing"
			@close="heartbeatFormVisible = false"
			@saved="monitorStore.fetchCheckStatuses()"
		/>
	</div>
</template>

<style scoped>
.site-filter {
	display: inline-flex;
	align-items: center;
	gap: 4px;
	font-size: 13px;
	padding: 4px 8px;
	border-radius: 999px;
	background: var(--bg-body);
	border: 1px solid var(--border-color);
}

.check-cell {
	display: flex;
	flex-direction: column;
	gap: 2px;
}

.check-url {
	font-size: 12px;
	color: var(--text-muted);
	word-break: break-all;
}

.default-tag {
	margin-left: 6px;
}

.expect-badge {
	font-size: 12px;
	background: var(--bg-body);
	border: 1px solid var(--border-color);
	border-radius: 4px;
	padding: 2px 6px;
	white-space: nowrap;
}

/* A condition is waiting on hover. */
.expect-badge.has-detail {
	cursor: help;
	text-decoration: underline dotted;
	text-underline-offset: 3px;
}

.state-pill {
	display: inline-flex;
	padding: 3px 10px;
	border-radius: 999px;
	font-size: 12px;
	font-weight: 600;
	border: 1px solid transparent;
}

.state-up {
	background: rgba(22, 163, 74, 0.12);
	color: #16a34a;
}

.state-down {
	background: rgba(220, 38, 38, 0.12);
	color: var(--danger, #dc2626);
	border-color: rgba(220, 38, 38, 0.3);
}

.state-investigating {
	background: rgba(234, 179, 8, 0.12);
	color: #ca8a04;
}

.rule-tag {
	margin-left: 6px;
	cursor: help;
}

.state-waiting,
.state-paused,
.state-orphaned {
	background: var(--bg-body);
	color: var(--text-muted);
	border-color: var(--border-color);
}

.row-orphaned td {
	opacity: 0.55;
}

.row-orphaned td:last-child {
	opacity: 1;
}

.actions-group {
	display: inline-flex;
	align-items: center;
	gap: 6px;
}
</style>
