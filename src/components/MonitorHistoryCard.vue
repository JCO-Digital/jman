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

const TOTAL_MINUTES = 1440; // 24 hours
const MS_PER_MINUTE = 60000;

const isIgnored = computed(() => {
	if (!props.domain) return false;
	return monitorStore.ignoredDomains.some((d) => d.domain === props.domain);
});

const liveStatus = computed(() =>
	props.domain ? monitorStore.currentStatus[props.domain] : null,
);

/**
 * Process history into a 24h timeline.
 * We calculate the duration of each status and map it to a duration-based flex-grow.
 */
const timelineData = computed(() => {
	if (props.history.length === 0) return [];

	const now = new Date();
	const startTime = new Date(now.getTime() - TOTAL_MINUTES * MS_PER_MINUTE);

	// Sort history by date
	const sorted = [...props.history].sort(
		(a, b) =>
			new Date(a.first_seen).getTime() - new Date(b.first_seen).getTime(),
	);

	return sorted
		.map((item) => {
			const first = new Date(item.first_seen);
			const last = new Date(item.last_seen);

			// Clip to 24h window
			const effectiveFirst = first < startTime ? startTime : first;
			const effectiveLast = last < startTime ? startTime : last;

			// If the entire record is older than 24h, it has 0 duration in our view
			if (last < startTime) return null;

			// Actual math duration in minutes (minimum 1 minute for any recorded event)
			const durationMs = Math.max(
				0,
				effectiveLast.getTime() - effectiveFirst.getTime(),
			);
			const durationMinutes = Math.max(durationMs / MS_PER_MINUTE, 1);

			return {
				...item,
				duration: durationMinutes,
				effectiveFirst,
				effectiveLast,
			};
		})
		.filter(
			(item): item is NonNullable<typeof item> =>
				item !== null && item.duration > 0,
		);
});

const uptimePercentage = computed(() => {
	if (timelineData.value.length === 0) return 100;

	const totalDuration = timelineData.value.reduce(
		(acc, item) => acc + item.duration,
		0,
	);
	const upDuration = timelineData.value
		.filter((item) => item.status.toUpperCase() === "UP")
		.reduce((acc, item) => acc + item.duration, 0);

	if (totalDuration === 0) return 100;
	return Math.min(100, (upDuration / totalDuration) * 100);
});

const getStatusClass = (status: string, errorCode: number) => {
	if (status.toUpperCase() === "UP") return "status-up";
	if (errorCode === 0) return "status-unknown";
	return "status-down";
};

const formatDate = (date: string | Date | null) => {
	if (!date) return "";
	return new Date(date).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
};

const startTimeLabel = computed(() => {
	const date = new Date(Date.now() - TOTAL_MINUTES * MS_PER_MINUTE);
	return formatDate(date);
});
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
			<div v-if="!isIgnored" class="uptime-stat">
				<span class="label">Uptime (Scale: 24h):</span>
				<span
					class="value"
					:class="{
						'text-success': uptimePercentage > 95,
						'text-warning':
							uptimePercentage <= 95 && uptimePercentage > 80,
						'text-error': uptimePercentage <= 80,
					}"
				>
					{{ uptimePercentage.toFixed(2) }}%
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

		<div v-else-if="timelineData.length > 0" class="history-container">
			<div class="status-timeline">
				<div
					v-for="item in timelineData"
					:key="item.id"
					class="status-block"
					:class="getStatusClass(item.status, item.error_code)"
					:style="{ flex: item.duration + ' 0 auto' }"
					:title="`${formatDate(item.first_seen)} - ${formatDate(item.last_seen)}: ${item.status} (${item.error_code})`"
				></div>
			</div>

			<div class="timeline-labels">
				<span>{{ startTimeLabel }}</span>
				<span>Latest</span>
			</div>
		</div>

		<div v-else-if="!monitorStore.isLoadingHistory" class="empty-state">
			<p>No monitor history available for this domain in the last 24h.</p>
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

.loading-container,
.ignored-container,
.empty-state {
	padding: 20px;
	text-align: center;
	color: var(--text-muted);
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
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

.status-timeline {
	display: flex;
	height: 32px;
	width: 100%;
	background-color: var(--bg-body);
	border-radius: 4px;
	overflow: hidden;
}

.status-block {
	height: 100%;
	min-width: 3px; /* Ensures even 1-minute outages are visible */
	transition: opacity 0.1s ease;
	border-right: 1px solid rgba(0, 0, 0, 0.05);
}

.status-block:last-child {
	border-right: none;
}

.status-block:hover {
	opacity: 0.8;
}

.status-up {
	background-color: var(--badge-active-bg);
}

.status-down {
	background-color: var(--badge-inactive-bg);
}

.status-unknown {
	background-color: var(--text-muted);
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
</style>
