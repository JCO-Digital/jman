<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from "vue";
import {
	Chart,
	Filler,
	LinearScale,
	LineController,
	LineElement,
	PointElement,
	CategoryScale,
	Tooltip,
	type ChartConfiguration,
} from "chart.js";
import type { SiteTrafficPeriod } from "../types";
import type { TrafficPeriod } from "../stores/trafficAnalytics";

Chart.register(
	LineController,
	LineElement,
	PointElement,
	LinearScale,
	CategoryScale,
	Filler,
	Tooltip,
);

const props = defineProps<{
	periods: SiteTrafficPeriod[];
	period: TrafficPeriod;
}>();

const canvasRef = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

function formatPeriodLabel(date: string) {
	if (props.period === "monthly") {
		return new Date(date).toLocaleString(undefined, {
			month: "short",
			year: "numeric",
		});
	}
	return new Date(date).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: props.period === "hourly" ? "2-digit" : undefined,
		minute: props.period === "hourly" ? "2-digit" : undefined,
	});
}

function formatCompact(n: number): string {
	if (n >= 1_000_000)
		return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1).replace(/\.0$/, "")}K`;
	return Math.round(n).toString();
}

// Chart.js draws on a canvas, so it can't read our CSS custom properties the
// way the old inline-SVG chart did — pull the current theme's resolved
// colors once per (re)build instead.
function themeColors() {
	const styles = getComputedStyle(document.documentElement);
	const get = (name: string) => styles.getPropertyValue(name).trim();
	return {
		primary: get("--primary"),
		muted: get("--text-muted"),
		border: get("--border-color"),
		card: get("--bg-card"),
		text: get("--text-main"),
	};
}

function buildConfig(): ChartConfiguration<"line"> {
	const colors = themeColors();
	const labels = props.periods.map((p) => formatPeriodLabel(p.period_start));

	return {
		type: "line",
		data: {
			labels,
			datasets: [
				{
					label: "Human",
					data: props.periods.map((p) => p.requests_human),
					borderColor: colors.primary,
					backgroundColor: colors.primary + "1a",
					fill: "origin",
					stack: "requests",
					pointRadius: 0,
					pointHoverRadius: 3,
					borderWidth: 2,
					tension: 0.15,
				},
				{
					label: "Bot",
					data: props.periods.map((p) => p.requests_bot),
					borderColor: colors.muted,
					backgroundColor: colors.muted + "1a",
					fill: "-1",
					stack: "requests",
					pointRadius: 0,
					pointHoverRadius: 3,
					borderWidth: 2,
					tension: 0.15,
				},
			],
		},
		options: {
			responsive: true,
			maintainAspectRatio: false,
			interaction: { mode: "index", intersect: false },
			scales: {
				x: {
					stacked: true,
					grid: { display: false },
					ticks: {
						color: colors.muted,
						font: { size: 10 },
						autoSkip: true,
						maxRotation: 0,
					},
				},
				y: {
					stacked: true,
					beginAtZero: true,
					grid: { color: colors.border },
					ticks: {
						color: colors.muted,
						font: { size: 10 },
						callback: (value) => formatCompact(Number(value)),
					},
				},
			},
			plugins: {
				legend: { display: false },
				tooltip: {
					backgroundColor: colors.card,
					titleColor: colors.muted,
					bodyColor: colors.text,
					borderColor: colors.border,
					borderWidth: 1,
					padding: 10,
					cornerRadius: 6,
					displayColors: true,
					callbacks: {
						afterBody: (items) => {
							const total = items.reduce(
								(sum, item) => sum + (item.parsed.y ?? 0),
								0,
							);
							return [`Total: ${total.toLocaleString()}`];
						},
						label: (item) =>
							` ${item.dataset.label}: ${(item.parsed.y ?? 0).toLocaleString()}`,
					},
				},
			},
		},
	};
}

function render() {
	if (!canvasRef.value) return;
	chart?.destroy();
	chart = new Chart(canvasRef.value, buildConfig());
}

let mediaQuery: MediaQueryList | null = null;

onMounted(() => {
	render();
	mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
	mediaQuery.addEventListener("change", render);
});

onUnmounted(() => {
	chart?.destroy();
	mediaQuery?.removeEventListener("change", render);
});

watch([() => props.periods, () => props.period], render);
</script>

<template>
	<div v-if="periods.length >= 2" class="traffic-chart">
		<div class="chart-legend">
			<span class="legend-item">
				<span class="legend-swatch legend-swatch--human"></span>
				Human
			</span>
			<span class="legend-item">
				<span class="legend-swatch legend-swatch--bot"></span>
				Bot
			</span>
		</div>

		<div class="chart-wrap">
			<canvas ref="canvasRef"></canvas>
		</div>
	</div>
	<p v-else class="text-muted font-sm">
		Not enough data yet for a trend chart.
	</p>
</template>

<style scoped>
.chart-legend {
	display: flex;
	gap: 16px;
	margin-bottom: 8px;
	font-size: 12px;
	color: var(--text-muted);
}

.legend-item {
	display: flex;
	align-items: center;
	gap: 6px;
}

.legend-swatch {
	width: 10px;
	height: 10px;
	border-radius: 2px;
}

.legend-swatch--human {
	background: var(--primary);
}

.legend-swatch--bot {
	background: var(--text-muted);
}

.chart-wrap {
	position: relative;
	height: 220px;
}
</style>
