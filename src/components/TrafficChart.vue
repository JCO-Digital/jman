<script setup lang="ts">
import { computed, ref } from "vue";
import type { SiteTrafficPeriod } from "../types";
import type { TrafficPeriod } from "../stores/trafficAnalytics";

const props = defineProps<{
	periods: SiteTrafficPeriod[];
	period: TrafficPeriod;
}>();

const SVG_WIDTH = 640;
const SVG_HEIGHT = 220;
const PADDING = { top: 16, right: 12, bottom: 24, left: 44 };
const plotWidth = SVG_WIDTH - PADDING.left - PADDING.right;
const plotHeight = SVG_HEIGHT - PADDING.top - PADDING.bottom;
const baselineY = PADDING.top + plotHeight;

const svgRef = ref<SVGSVGElement | null>(null);
const activeIndex = ref<number | null>(null);

// Round up to a "clean" axis max (1/2/5 * 10^n) so gridline labels read as
// whole numbers instead of arbitrary fractions of the raw peak value.
function niceCeil(value: number): number {
	if (value <= 0) return 1;
	const exponent = Math.floor(Math.log10(value));
	const magnitude = 10 ** exponent;
	const fraction = value / magnitude;
	const niceFraction = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10;
	return niceFraction * magnitude;
}

function formatCompact(n: number): string {
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1).replace(/\.0$/, "")}K`;
	return Math.round(n).toString();
}

function formatPeriodLabel(date: string) {
	return new Date(date).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: props.period === "hourly" ? "2-digit" : undefined,
		minute: props.period === "hourly" ? "2-digit" : undefined,
	});
}

const n = computed(() => props.periods.length);

function xStep() {
	return plotWidth / Math.max(n.value - 1, 1);
}

function pointX(i: number) {
	return PADDING.left + i * xStep();
}

const yMax = computed(() => {
	const rawMax = props.periods.reduce(
		(max, p) => Math.max(max, p.requests_human + p.requests_bot),
		0,
	);
	return niceCeil(rawMax * 1.05);
});

function yScale(value: number) {
	return PADDING.top + plotHeight - (value / yMax.value) * plotHeight;
}

const humanPoints = computed(() =>
	props.periods.map((p, i) => ({ x: pointX(i), y: yScale(p.requests_human) })),
);
const stackPoints = computed(() =>
	props.periods.map((p, i) => ({
		x: pointX(i),
		y: yScale(p.requests_human + p.requests_bot),
	})),
);

function linePath(points: { x: number; y: number }[]) {
	if (points.length === 0) return "";
	return (
		`M ${points[0].x},${points[0].y} ` +
		points
			.slice(1)
			.map((p) => `L ${p.x},${p.y}`)
			.join(" ")
	);
}

const humanLinePath = computed(() => linePath(humanPoints.value));
const stackLinePath = computed(() => linePath(stackPoints.value));

const humanAreaPath = computed(() => {
	const pts = humanPoints.value;
	if (pts.length === 0) return "";
	return `${linePath(pts)} L ${pts[pts.length - 1].x},${baselineY} L ${pts[0].x},${baselineY} Z`;
});

const botAreaPath = computed(() => {
	const bottom = humanPoints.value;
	const top = stackPoints.value;
	if (bottom.length === 0) return "";
	return (
		linePath(bottom) +
		" " +
		[...top]
			.reverse()
			.map((p) => `L ${p.x},${p.y}`)
			.join(" ") +
		" Z"
	);
});

const yTicks = computed(() => [
	{ value: 0, y: yScale(0) },
	{ value: yMax.value / 2, y: yScale(yMax.value / 2) },
	{ value: yMax.value, y: yScale(yMax.value) },
]);

const xTicks = computed(() => {
	const count = n.value;
	if (count === 0) return [];
	const indices = new Set(
		[0, Math.floor(count / 4), Math.floor(count / 2), Math.floor((3 * count) / 4), count - 1].filter(
			(i) => i >= 0 && i < count,
		),
	);
	return [...indices].map((i) => ({
		index: i,
		x: pointX(i),
		label: formatPeriodLabel(props.periods[i].period_start),
	}));
});

const ariaLabel = computed(
	() =>
		`Stacked area chart of human and bot requests per ${props.period === "hourly" ? "hour" : "day"} over the last ${n.value} ${props.period === "hourly" ? "hours" : "days"}.`,
);

function nearestIndex(clientX: number): number | null {
	if (!svgRef.value || n.value === 0) return null;
	const rect = svgRef.value.getBoundingClientRect();
	if (rect.width === 0) return null;
	const scale = SVG_WIDTH / rect.width;
	const logicalX = (clientX - rect.left) * scale;
	const step = xStep();
	const raw = (logicalX - PADDING.left) / step;
	return Math.min(n.value - 1, Math.max(0, Math.round(raw)));
}

function onPointerMove(event: PointerEvent) {
	activeIndex.value = nearestIndex(event.clientX);
}

function onFocus() {
	if (activeIndex.value === null) activeIndex.value = n.value - 1;
}

function clearActive() {
	activeIndex.value = null;
}

function onKeydown(event: KeyboardEvent) {
	if (n.value === 0) return;
	if (event.key === "ArrowLeft") {
		activeIndex.value = Math.max(0, (activeIndex.value ?? n.value) - 1);
		event.preventDefault();
	} else if (event.key === "ArrowRight") {
		activeIndex.value = Math.min(n.value - 1, (activeIndex.value ?? -1) + 1);
		event.preventDefault();
	}
}

const activePeriod = computed(() =>
	activeIndex.value !== null ? props.periods[activeIndex.value] : null,
);

const tooltipStyle = computed(() => {
	if (activeIndex.value === null) return {};
	return { left: `${(pointX(activeIndex.value) / SVG_WIDTH) * 100}%` };
});

const tooltipAlign = computed(() => {
	if (activeIndex.value === null || n.value === 0) return "center";
	const fraction = activeIndex.value / Math.max(n.value - 1, 1);
	if (fraction < 0.2) return "start";
	if (fraction > 0.8) return "end";
	return "center";
});
</script>

<template>
	<div v-if="n >= 2" class="traffic-chart">
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
			<svg
				ref="svgRef"
				:viewBox="`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`"
				preserveAspectRatio="none"
				class="chart-svg"
				role="img"
				:aria-label="ariaLabel"
			>
				<g class="gridlines">
					<line
						v-for="tick in yTicks"
						:key="tick.value"
						:x1="PADDING.left"
						:x2="SVG_WIDTH - PADDING.right"
						:y1="tick.y"
						:y2="tick.y"
					/>
					<text
						v-for="tick in yTicks"
						:key="`lbl-${tick.value}`"
						:x="PADDING.left - 8"
						:y="tick.y"
						dy="4"
						text-anchor="end"
						class="axis-label"
					>
						{{ formatCompact(tick.value) }}
					</text>
				</g>

				<path :d="botAreaPath" class="area-bot" />
				<path :d="humanAreaPath" class="area-human" />
				<path :d="stackLinePath" class="line-bot" />
				<path :d="humanLinePath" class="line-human" />

				<g class="x-labels">
					<text
						v-for="tick in xTicks"
						:key="tick.index"
						:x="tick.x"
						:y="SVG_HEIGHT - 4"
						text-anchor="middle"
						class="axis-label"
					>
						{{ tick.label }}
					</text>
				</g>

				<line
					v-if="activeIndex !== null"
					class="crosshair"
					:x1="pointX(activeIndex)"
					:x2="pointX(activeIndex)"
					:y1="PADDING.top"
					:y2="baselineY"
				/>

				<rect
					class="hover-capture"
					:x="PADDING.left"
					:y="PADDING.top"
					:width="plotWidth"
					:height="plotHeight"
					tabindex="0"
					role="slider"
					:aria-valuemin="0"
					:aria-valuemax="n - 1"
					:aria-valuenow="activeIndex ?? n - 1"
					:aria-valuetext="
						activePeriod
							? `${formatPeriodLabel(activePeriod.period_start)}: ${activePeriod.requests_human.toLocaleString()} human, ${activePeriod.requests_bot.toLocaleString()} bot requests`
							: undefined
					"
					@pointermove="onPointerMove"
					@pointerleave="clearActive"
					@focus="onFocus"
					@blur="clearActive"
					@keydown="onKeydown"
				/>
			</svg>

			<div
				v-if="activePeriod"
				class="chart-tooltip"
				:class="`chart-tooltip--${tooltipAlign}`"
				:style="tooltipStyle"
			>
				<div class="chart-tooltip-header">
					{{ formatPeriodLabel(activePeriod.period_start) }}
				</div>
				<div class="chart-tooltip-row">
					<span class="chart-tooltip-key chart-tooltip-key--human"></span>
					<span class="chart-tooltip-value">{{
						activePeriod.requests_human.toLocaleString()
					}}</span>
					<span class="chart-tooltip-name">Human</span>
				</div>
				<div class="chart-tooltip-row">
					<span class="chart-tooltip-key chart-tooltip-key--bot"></span>
					<span class="chart-tooltip-value">{{
						activePeriod.requests_bot.toLocaleString()
					}}</span>
					<span class="chart-tooltip-name">Bot</span>
				</div>
				<div class="chart-tooltip-row chart-tooltip-row--total">
					<span class="chart-tooltip-value">{{
						(
							activePeriod.requests_human + activePeriod.requests_bot
						).toLocaleString()
					}}</span>
					<span class="chart-tooltip-name">Total</span>
				</div>
			</div>
		</div>
	</div>
	<p v-else class="text-muted font-sm">Not enough data yet for a trend chart.</p>
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
}

.chart-svg {
	width: 100%;
	height: 200px;
	display: block;
	overflow: visible;
}

.gridlines line {
	stroke: var(--border-color);
	stroke-width: 1;
}

.axis-label {
	fill: var(--text-muted);
	font-size: 10px;
}

.area-human {
	fill: var(--primary);
	fill-opacity: 0.1;
	stroke: none;
}

.area-bot {
	fill: var(--text-muted);
	fill-opacity: 0.1;
	stroke: none;
}

.line-human {
	fill: none;
	stroke: var(--primary);
	stroke-width: 2;
	stroke-linejoin: round;
	stroke-linecap: round;
}

.line-bot {
	fill: none;
	stroke: var(--text-muted);
	stroke-width: 2;
	stroke-linejoin: round;
	stroke-linecap: round;
}

.crosshair {
	stroke: var(--text-muted);
	stroke-width: 1;
	pointer-events: none;
}

.hover-capture {
	fill: transparent;
	cursor: crosshair;

	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}

.chart-tooltip {
	position: absolute;
	top: 8px;
	transform: translateX(-50%);
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 6px;
	padding: 8px 10px;
	font-size: 12px;
	box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
	pointer-events: none;
	white-space: nowrap;
	z-index: 1;
}

.chart-tooltip--start {
	transform: translateX(0);
}

.chart-tooltip--end {
	transform: translateX(-100%);
}

.chart-tooltip-header {
	color: var(--text-muted);
	margin-bottom: 4px;
}

.chart-tooltip-row {
	display: flex;
	align-items: center;
	gap: 6px;
}

.chart-tooltip-row--total {
	border-top: 1px solid var(--border-color);
	margin-top: 4px;
	padding-top: 4px;
}

.chart-tooltip-key {
	width: 10px;
	height: 2px;
	border-radius: 1px;
}

.chart-tooltip-key--human {
	background: var(--primary);
}

.chart-tooltip-key--bot {
	background: var(--text-muted);
}

.chart-tooltip-value {
	font-weight: 600;
	color: var(--text-main);
}

.chart-tooltip-name {
	color: var(--text-muted);
}
</style>
