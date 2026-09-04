<script setup lang="ts">
import { ref, computed, watch } from "vue";
import {
	useTrafficAnalyticsStore,
	type TrafficPeriod,
} from "../stores/trafficAnalytics";
import LoadingSpinner from "./LoadingSpinner.vue";
import TrafficChart from "./TrafficChart.vue";

const props = defineProps<{
	siteId: number;
}>();

const trafficStore = useTrafficAnalyticsStore();

// Simple period toggle; the store caches per (siteId, period, days) so
// flipping back and forth between hourly/daily doesn't refetch.
const period = ref<TrafficPeriod>("hourly");
// Hourly data is only retained server-side for 168h/7 days (see jman-api's
// site_traffic_hourly pruning), so requesting more would just silently
// return fewer points than the window implies. Daily rollups are cheap to
// keep much longer, so that view can show a wider range. Monthly is
// aggregated on the fly from daily rows, so 366 days comfortably covers up
// to 12 months — most sites won't have that much history yet, which is
// fine, the chart just renders however many months actually come back.
const HOURLY_DAYS = 7;
const DAILY_DAYS = 30;
const MONTHLY_DAYS = 366;
const days = computed(() => {
	if (period.value === "hourly") return HOURLY_DAYS;
	if (period.value === "monthly") return MONTHLY_DAYS;
	return DAILY_DAYS;
});
const windowLabel = computed(() => {
	if (period.value === "hourly") return `Last ${HOURLY_DAYS} Days`;
	if (period.value === "monthly") return "Last 12 Months";
	return `Last ${DAILY_DAYS} Days`;
});

const traffic = computed(
	() => trafficStore.getTraffic(props.siteId, period.value, days.value) ?? [],
);
const isLoading = computed(() =>
	trafficStore.isLoadingTraffic(props.siteId, period.value, days.value),
);
const error = computed(() =>
	trafficStore.getError(props.siteId, period.value, days.value),
);

async function load() {
	try {
		await trafficStore.fetchTraffic(props.siteId, period.value, days.value);
	} catch (e) {
		// Error state is already surfaced reactively via the store; nothing
		// further to do here.
		console.error("Failed to fetch site traffic:", e);
	}
}

watch([() => props.siteId, period], load, { immediate: true });

// Traffic is returned oldest-first; the most recently completed period is
// the most useful "at a glance" snapshot.
const latest = computed(() =>
	traffic.value.length > 0 ? traffic.value[traffic.value.length - 1] : null,
);

const TOP_LIST_LIMIT = 10;
const topPages = computed(
	() => latest.value?.top_pages.slice(0, TOP_LIST_LIMIT) ?? [],
);
const topReferrers = computed(
	() => latest.value?.top_referrers.slice(0, TOP_LIST_LIMIT) ?? [],
);
// No slicing here (unlike topPages/topReferrers above) — realistically only
// a handful of distinct status codes ever appear, well under the backend's
// own 20-entry cap.
const statusCodes = computed(() => latest.value?.status_codes ?? []);

// Maps a status code's leading digit to a `.status-badge` color modifier
// (see src/styles/components.css) — anything outside 2xx-5xx (e.g. a rare
// 1xx) falls back to the neutral "info" style.
function statusBadgeClass(code: string): string {
	switch (code.charAt(0)) {
		case "2":
			return "success";
		case "3":
			return "info";
		case "4":
			return "warning";
		case "5":
			return "error";
		default:
			return "info";
	}
}

function formatPeriodStart(date: string) {
	if (period.value === "monthly") {
		return new Date(date).toLocaleString(undefined, {
			month: "short",
			year: "numeric",
		});
	}
	return new Date(date).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: period.value === "hourly" ? "2-digit" : undefined,
		minute: period.value === "hourly" ? "2-digit" : undefined,
	});
}

function setPeriod(p: TrafficPeriod) {
	period.value = p;
}
</script>

<template>
	<section class="card mt-4">
		<div class="card-header">
			<h2>Visitor Traffic</h2>
			<div class="btn-group">
				<button
					type="button"
					class="btn btn-sm"
					:class="period === 'hourly' ? 'btn-primary' : 'btn-outline'"
					@click="setPeriod('hourly')"
				>
					Hourly
				</button>
				<button
					type="button"
					class="btn btn-sm"
					:class="period === 'daily' ? 'btn-primary' : 'btn-outline'"
					@click="setPeriod('daily')"
				>
					Daily
				</button>
				<button
					type="button"
					class="btn btn-sm"
					:class="
						period === 'monthly' ? 'btn-primary' : 'btn-outline'
					"
					@click="setPeriod('monthly')"
				>
					Monthly
				</button>
			</div>
		</div>

		<div v-if="isLoading && traffic.length === 0" class="loading-state">
			<LoadingSpinner message="Loading traffic data..." />
		</div>

		<div v-else-if="error" class="loading-state text-muted">
			<p>Failed to load traffic data: {{ error }}</p>
		</div>

		<div v-else-if="!latest" class="loading-state text-muted">
			<p>No traffic data yet.</p>
		</div>

		<div v-else>
			<p class="font-sm text-muted mb-4">
				Showing latest
				{{
					period === "hourly"
						? "hour"
						: period === "daily"
							? "day"
							: "month"
				}}:
				{{ formatPeriodStart(latest.period_start) }}
			</p>

			<div class="info-grid">
				<div class="info-item">
					<span class="label">Total Requests</span>
					<span class="value">{{
						latest.requests_total.toLocaleString()
					}}</span>
				</div>
				<div class="info-item">
					<span class="label">Human Requests</span>
					<span class="value">{{
						latest.requests_human.toLocaleString()
					}}</span>
				</div>
				<div class="info-item">
					<span class="label">Bot Requests</span>
					<span class="value">{{
						latest.requests_bot.toLocaleString()
					}}</span>
				</div>
				<div class="info-item">
					<span class="label">Unique Visitors</span>
					<span class="value">{{
						latest.unique_visitors.toLocaleString()
					}}</span>
				</div>
				<div
					v-if="statusCodes.length > 0"
					class="info-item info-item--full-width"
				>
					<span class="label">Status Codes</span>
					<div class="status-badge-row">
						<span
							v-for="sc in statusCodes"
							:key="sc.key"
							class="status-badge badge-sm"
							:class="statusBadgeClass(sc.key)"
						>
							{{ sc.key }} · {{ sc.count.toLocaleString() }}
						</span>
					</div>
				</div>
			</div>

			<div class="mt-4">
				<h3 class="sub-text font-medium mb-4">
					Requests Over the {{ windowLabel }}
				</h3>
				<TrafficChart :periods="traffic" :period="period" />
			</div>

			<div class="grid-2-cols mt-4">
				<div>
					<h3 class="sub-text font-medium mb-4">Top Pages</h3>
					<ol v-if="topPages.length > 0" class="ranked-list">
						<li
							v-for="page in topPages"
							:key="page.key"
							class="ranked-list-item"
						>
							<span class="truncate" :title="page.key">{{
								page.key
							}}</span>
							<span class="text-muted">{{
								page.count.toLocaleString()
							}}</span>
						</li>
					</ol>
					<p v-else class="text-muted font-sm">
						No page data for this period.
					</p>
				</div>
				<div>
					<h3 class="sub-text font-medium mb-4">Top Referrers</h3>
					<ol v-if="topReferrers.length > 0" class="ranked-list">
						<li
							v-for="referrer in topReferrers"
							:key="referrer.key"
							class="ranked-list-item"
						>
							<span class="truncate" :title="referrer.key">{{
								referrer.key
							}}</span>
							<span class="text-muted">{{
								referrer.count.toLocaleString()
							}}</span>
						</li>
					</ol>
					<p v-else class="text-muted font-sm">
						No referrer data for this period.
					</p>
				</div>
			</div>
		</div>
	</section>
</template>

<style scoped>
.info-item--full-width {
	grid-column: 1 / -1;
}

.status-badge-row {
	display: flex;
	flex-wrap: wrap;
	gap: 6px;
}

.ranked-list {
	list-style: none;
	margin: 0;
	padding: 0;
	display: flex;
	flex-direction: column;
	gap: 8px;
}

.ranked-list-item {
	display: flex;
	justify-content: space-between;
	align-items: center;
	gap: 12px;
	padding: 6px 0;
	border-bottom: 1px solid var(--border-color);
	font-size: 14px;

	&:last-child {
		border-bottom: none;
	}
}

.ranked-list-item .truncate {
	flex: 1 1 auto;
	min-width: 0;
}

.ranked-list-item .text-muted {
	flex-shrink: 0;
}
</style>
