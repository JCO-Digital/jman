<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import type { MonitorHistory } from "../types";
import { useMonitorStore } from "../stores/monitor";
import LoadingSpinner from "./LoadingSpinner.vue";

const props = defineProps<{
	history: MonitorHistory[];
	domain?: string;
}>();

const monitorStore = useMonitorStore();

const isIgnored = computed(() => {
	if (!props.domain) return false;
	return monitorStore.ignoredDomains.some((d) => d.domain === props.domain);
});
const liveStatus = computed(() =>
	props.domain ? monitorStore.currentStatus[props.domain] : null,
);

/**
 * We want to show a series of blocks representing the status.
 * Since the backend provides aggregated history, we'll sort them by date.
 */
const sortedHistory = computed(() => {
	return [...props.history].sort(
		(a, b) =>
			new Date(a.first_seen).getTime() - new Date(b.first_seen).getTime(),
	);
});

const uptimePercentage = computed(() => {
	if (props.history.length === 0) return 0;
	const upCount = props.history.filter((h) => h.status === "UP").length;
	return Math.round((upCount / props.history.length) * 100);
});

const getStatusClass = (status: string) => {
	return status.toLowerCase() === "up" ? "status-up" : "status-down";
};

const formatDate = (dateStr: string) => {
	return new Date(dateStr).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
};
</script>

<template>
	<section class="card">
		<div class="card-header">
			<div class="title-with-status">
				<h2>Uptime History</h2>
				<div v-if="isIgnored" class="ignored-badge">Ignored</div>
				<div
					v-else-if="liveStatus"
					class="live-status-indicator"
					:title="
						liveStatus.last_checked
							? `Last checked: ${formatDate(liveStatus.last_checked)}${liveStatus.failure_count > 0 ? ` (${liveStatus.failure_count} failures)` : ''}`
							: liveStatus.status_message || 'Pending check'
					"
				>
					<span
						class="pulse-icon"
						:class="
							liveStatus.last_checked
								? !liveStatus.is_down
									? 'bg-success'
									: 'bg-error'
								: 'bg-pending'
						"
					></span>
					{{ liveStatus.last_checked ? "Live" : "Pending" }}
				</div>
			</div>
			<div v-if="history.length > 0 && !isIgnored" class="uptime-stat">
				<span class="label">Uptime (Last 48h):</span>
				<span
					class="value"
					:class="{
						'text-success': uptimePercentage > 95,
						'text-warning': uptimePercentage <= 95 && uptimePercentage > 80,
						'text-error': uptimePercentage <= 80,
					}"
				>
					{{ uptimePercentage }}%
				</span>
			</div>
		</div>

		<div v-if="monitorStore.isLoadingHistory" class="loading-container">
			<LoadingSpinner message="Loading history..." />
		</div>

		<div v-else-if="isIgnored" class="ignored-container">
			<p>Monitoring is disabled for this domain.</p>
			<RouterLink to="/settings" class="settings-link">
				Manage ignored sites in Settings
			</RouterLink>
		</div>

		<div class="history-container" v-else-if="history.length > 0">
			<div class="status-timeline">
				<div
					v-for="item in sortedHistory"
					:key="item.id"
					class="status-block"
					:class="getStatusClass(item.status)"
					:title="`${formatDate(item.first_seen)} - ${item.status} (${item.error_code})`"
				></div>
			</div>

			<div class="timeline-labels">
				<span v-if="sortedHistory.length > 0">
					{{ formatDate(sortedHistory[0]?.first_seen ?? "") }}
				</span>
				<span>Latest</span>
			</div>
		</div>

		<div v-else-if="!monitorStore.isLoadingHistory" class="empty-state">
			<p>No monitor history available for this domain.</p>
		</div>
	</section>
</template>

<style scoped>
.card-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	border-bottom: 1px solid var(--border-color);
	margin-bottom: 16px;
	padding-bottom: 8px;
}

.card-header h2 {
	margin: 0;
	border-bottom: none;
	padding-bottom: 0;
}

.uptime-stat {
	display: flex;
	align-items: center;
	gap: 8px;
	font-size: 0.9em;
}

.uptime-stat .label {
	color: var(--text-muted);
	font-weight: 500;
}

.uptime-stat .value {
	font-weight: 700;
	font-size: 1.1em;
}

.history-container {
	margin-top: 8px;
}

.loading-container {
	padding: 20px;
	display: flex;
	justify-content: center;
}

.status-timeline {
	display: flex;
	gap: 2px;
	height: 32px;
	width: 100%;
}

.status-block {
	flex: 1;
	border-radius: 2px;
	min-width: 4px;
	transition: transform 0.1s ease;
}

.status-block:hover {
	transform: scaleY(1.2);
}

.status-up {
	background-color: var(--badge-active-bg);
}

.status-down {
	background-color: var(--badge-inactive-bg);
}

.timeline-labels {
	display: flex;
	justify-content: space-between;
	margin-top: 8px;
	font-size: 0.75em;
	color: var(--text-muted);
}

.text-success {
	color: var(--badge-active-text);
}

.text-warning {
	color: var(--badge-drop-in-text);
}

.text-error {
	color: var(--badge-inactive-text);
}

.title-with-status {
	display: flex;
	align-items: center;
	gap: 12px;
}

.ignored-badge {
	font-size: 0.75em;
	font-weight: 600;
	text-transform: uppercase;
	color: var(--badge-inactive-text);
	background: var(--badge-inactive-bg);
	padding: 2px 8px;
	border-radius: 4px;
}

.ignored-container {
	padding: 20px;
	text-align: center;
	color: var(--text-muted);
}

.settings-link {
	display: inline-block;
	margin-top: 8px;
	color: var(--primary);
	text-decoration: none;
	font-size: 0.9em;
	font-weight: 500;
}

.settings-link:hover {
	text-decoration: underline;
}

.live-status-indicator {
	display: flex;
	align-items: center;
	gap: 6px;
	font-size: 0.75em;
	font-weight: 600;
	text-transform: uppercase;
	color: var(--text-muted);
	background: var(--bg-body);
	padding: 2px 8px;
	border-radius: 4px;
}

.pulse-icon {
	width: 8px;
	height: 8px;
	border-radius: 50%;
	display: inline-block;
}

.bg-success {
	background-color: var(--badge-active-text);
	box-shadow: 0 0 0 0 rgba(52, 211, 153, 0.7);
	animation: pulse-green 2s infinite;
}

.bg-error {
	background-color: var(--badge-inactive-text);
	box-shadow: 0 0 0 0 rgba(248, 113, 113, 0.7);
	animation: pulse-red 2s infinite;
}

.bg-pending {
	background-color: var(--text-muted);
	box-shadow: 0 0 0 0 rgba(156, 163, 175, 0.7);
	animation: pulse-gray 2s infinite;
}

@keyframes pulse-green {
	0% {
		transform: scale(0.95);
		box-shadow: 0 0 0 0 rgba(52, 211, 153, 0.7);
	}
	70% {
		transform: scale(1);
		box-shadow: 0 0 0 4px rgba(52, 211, 153, 0);
	}
	100% {
		transform: scale(0.95);
		box-shadow: 0 0 0 0 rgba(52, 211, 153, 0);
	}
}

@keyframes pulse-red {
	0% {
		transform: scale(0.95);
		box-shadow: 0 0 0 0 rgba(248, 113, 113, 0.7);
	}
	70% {
		transform: scale(1);
		box-shadow: 0 0 0 4px rgba(248, 113, 113, 0);
	}
	100% {
		transform: scale(0.95);
		box-shadow: 0 0 0 0 rgba(248, 113, 113, 0);
	}
}

@keyframes pulse-gray {
	0% {
		transform: scale(0.95);
		box-shadow: 0 0 0 0 rgba(156, 163, 175, 0.7);
	}
	70% {
		transform: scale(1);
		box-shadow: 0 0 0 4px rgba(156, 163, 175, 0);
	}
	100% {
		transform: scale(0.95);
		box-shadow: 0 0 0 0 rgba(156, 163, 175, 0);
	}
}

.empty-state {
	text-align: center;
	padding: 20px;
	color: var(--text-muted);
}
</style>
