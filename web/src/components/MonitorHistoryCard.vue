<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import type { MonitorHistory } from "../types";
import { useMonitorStore } from "../stores/monitor";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import { useToastStore } from "../stores/toast";
import LoadingSpinner from "./LoadingSpinner.vue";

const props = defineProps<{
	history: MonitorHistory[];
	domain?: string;
	siteId?: string;
	serverId?: string | null;
}>();

const monitorStore = useMonitorStore();
const authStore = useAuthStore();
const dataStore = useDataStore();
const toast = useToastStore();

const TOTAL_MINUTES = 1440; // 24 hours
const MS_PER_MINUTE = 60000;

const effectiveSiteId = computed(() => {
	if (props.siteId) return props.siteId;
	// Fallback to domain-based lookup if the ID is missing
	return props.domain
		? dataStore.sites.find((s) => s.domain === props.domain)?.id
		: undefined;
});

/** The site's default monitor check, once checks are loaded. */
const defaultCheck = computed(() =>
	effectiveSiteId.value
		? monitorStore.defaultCheckBySite.get(effectiveSiteId.value)
		: undefined,
);

const isPaused = computed(() => defaultCheck.value?.enabled === false);
const isToggling = ref(false);

async function togglePaused() {
	const check = defaultCheck.value;
	if (!check) return;
	isToggling.value = true;
	try {
		await monitorStore.setCheckPaused(check.id, check.enabled);
		toast.addToast(
			check.enabled ? "Monitoring paused" : "Monitoring resumed",
			"success",
		);
	} catch (err) {
		toast.addToast(
			err instanceof Error ? err.message : "Failed to update check",
			"error",
		);
	} finally {
		isToggling.value = false;
	}
}

onMounted(() => {
	monitorStore.ensureChecks().catch((err) => {
		console.error("Failed to load monitor checks:", err);
	});
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
			<div class="flex-row gap-3">
				<h2>Uptime History</h2>
				<RouterLink
					v-if="effectiveSiteId"
					:to="{
						path: '/monitoring',
						query: {
							tab: 'uptime',
							section: 'checks',
							site: effectiveSiteId,
						},
					}"
					class="btn btn-text font-sm"
					title="Manage this site's monitor checks"
				>
					Checks →
				</RouterLink>
				<div v-if="isPaused" class="ignored-badge">Paused</div>
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
			<div v-if="!isPaused" class="flex-row gap-2 font-sm">
				<span class="text-muted font-medium">Uptime (24h)</span>
				<span
					class="font-weight-700"
					:class="{
						'text-success': uptimePercentage > 95,
						'text-warning':
							uptimePercentage <= 95 && uptimePercentage > 80,
						'text-error': uptimePercentage <= 80,
					}"
					style="font-size: 1.1em; font-weight: 700"
				>
					{{ uptimePercentage.toFixed(2) }}%
				</span>
			</div>
		</div>

		<div v-if="monitorStore.isLoadingHistory" class="loading-state">
			<LoadingSpinner message="Loading history..." />
		</div>

		<div v-else-if="isPaused" class="loading-state">
			<p>Monitoring is paused for this site.</p>
			<button
				v-if="authStore.canEdit"
				class="btn btn-text font-sm mt-2"
				:disabled="isToggling"
				@click="togglePaused"
			>
				Resume monitoring
			</button>
		</div>

		<div v-else-if="timelineData.length > 0" class="mt-2">
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

		<div
			v-else-if="!monitorStore.isLoadingHistory"
			class="loading-state text-muted"
		>
			<p>No monitor history available for this domain in the last 24h.</p>
		</div>
	</section>
</template>

<style scoped>
/* Scoped styles removed in favor of global utility classes and component styles */
</style>
