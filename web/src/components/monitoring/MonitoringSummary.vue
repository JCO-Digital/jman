<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useIncidentStore } from "../../stores/incidents";
import { useMonitorStore } from "../../stores/monitor";
import { useScanStore } from "../../stores/scan";
import { useVulnStore } from "../../stores/vulns";
import { useToastStore } from "../../stores/toast";
import LoadingSpinner from "../LoadingSpinner.vue";
import { severityClass } from "./scan";
import { cvssSeverity, formatCvss } from "./vuln";

/** How many of an area's most urgent items a card lists. */
const topN = 5;

const incidentStore = useIncidentStore();
const monitorStore = useMonitorStore();
const scanStore = useScanStore();
const vulnStore = useVulnStore();
const toastStore = useToastStore();

const loading = ref(false);
const scannedSites = ref<number | null>(null);

const uptimeCount = computed(() => incidentStore.activeByType.uptime ?? 0);
const securityCount = computed(() => scanStore.activeIncidents.length);
const uptimeTop = computed(() => incidentStore.activeIncidents.slice(0, topN));
// Critical and high incidents first, then the newest.
const severityRank: Record<string, number> = {
	critical: 0,
	high: 1,
	medium: 2,
	low: 3,
};
const securityTop = computed(() =>
	[...scanStore.activeIncidents]
		.sort(
			(a, b) =>
				(severityRank[a.severity ?? ""] ?? 4) -
					(severityRank[b.severity ?? ""] ?? 4) ||
				b.down_since.localeCompare(a.down_since),
		)
		.slice(0, topN),
);
const vulnOverdue = computed(() => vulnStore.summary?.overdue ?? 0);
// Groups come sorted by the API: overdue first, then by CVSS.
const vulnTop = computed(() => vulnStore.openGroups.slice(0, topN));
const runningChecks = computed(
	() => monitorStore.checks.filter((c) => c.enabled).length,
);

onMounted(refresh);

async function refresh() {
	loading.value = true;
	try {
		await Promise.all([
			incidentStore.fetchActiveIncidents(1, topN),
			scanStore.fetchActiveIncidents(),
			monitorStore.ensureChecks(),
			vulnStore.fetchOpenGroups(),
			vulnStore.fetchSummary(),
			scanStore
				.fetchScannedSiteCount()
				.then((n) => (scannedSites.value = n)),
		]);
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load monitoring",
			"error",
		);
	} finally {
		loading.value = false;
	}
}

defineExpose({ refresh });

function since(iso: string): string {
	return new Date(iso).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}
</script>

<template>
	<div class="summary-grid">
		<!-- Uptime -->
		<section class="card area-card">
			<header class="area-head">
				<h2>Uptime</h2>
				<span
					class="area-count"
					:class="uptimeCount ? 'count-alert' : 'count-clear'"
				>
					{{ uptimeCount ? `${uptimeCount} open` : "All clear" }}
				</span>
				<LoadingSpinner v-if="loading" small />
			</header>
			<ul v-if="uptimeTop.length" class="area-items">
				<li v-for="inc in uptimeTop" :key="inc.id">
					<RouterLink
						:to="{ path: '/monitoring', query: { tab: 'uptime' } }"
						class="item-link"
					>
						<span class="font-medium">{{ inc.domain }}</span>
						<span
							v-if="inc.status === 'acknowledged'"
							class="status-badge badge-sm warning"
							>Acked</span
						>
						<span class="sub-text item-meta"
							>down since {{ since(inc.down_since) }}</span
						>
					</RouterLink>
				</li>
			</ul>
			<p v-else class="sub-text watching">
				<template v-if="monitorStore.checksFetched">
					No sites down. Watching {{ runningChecks }} check{{
						runningChecks === 1 ? "" : "s"
					}}.
				</template>
				<template v-else>No sites down.</template>
			</p>
			<footer class="area-foot">
				<RouterLink
					:to="{ path: '/monitoring', query: { tab: 'uptime' } }"
					class="btn btn-text btn-sm"
				>
					Open Uptime →
				</RouterLink>
			</footer>
		</section>

		<!-- Security -->
		<section class="card area-card">
			<header class="area-head">
				<h2>Security</h2>
				<span
					class="area-count"
					:class="securityCount ? 'count-alert' : 'count-clear'"
				>
					{{ securityCount ? `${securityCount} open` : "All clear" }}
				</span>
			</header>
			<ul v-if="securityTop.length" class="area-items">
				<li v-for="inc in securityTop" :key="inc.id">
					<RouterLink
						:to="{
							path: '/monitoring',
							query: { tab: 'security', incident: inc.id },
						}"
						class="item-link"
					>
						<span
							class="status-badge badge-sm"
							:class="severityClass(inc.severity)"
						>
							{{ inc.severity || "-" }}
						</span>
						<span class="font-medium">{{ inc.domain }}</span>
						<span class="sub-text item-meta">{{
							inc.error_message
						}}</span>
					</RouterLink>
				</li>
			</ul>
			<p v-else class="sub-text watching">
				No security incidents.
				<template v-if="scannedSites !== null">
					Scanning {{ scannedSites }} site{{
						scannedSites === 1 ? "" : "s"
					}}.
				</template>
			</p>
			<footer class="area-foot">
				<RouterLink
					:to="{ path: '/monitoring', query: { tab: 'security' } }"
					class="btn btn-text btn-sm"
				>
					Open Security →
				</RouterLink>
			</footer>
		</section>

		<!-- Vulnerabilities -->
		<section class="card area-card">
			<header class="area-head">
				<h2>Vulnerabilities</h2>
				<span
					class="area-count"
					:class="vulnOverdue ? 'count-alert' : 'count-clear'"
				>
					{{
						vulnOverdue ? `${vulnOverdue} overdue` : "None overdue"
					}}
				</span>
			</header>
			<ul v-if="vulnTop.length" class="area-items">
				<li
					v-for="g in vulnTop"
					:key="`${g.kind}:${g.slug}:${g.version}`"
				>
					<RouterLink
						:to="{
							path: '/monitoring',
							query: { tab: 'vulnerabilities' },
						}"
						class="item-link"
					>
						<span
							class="status-badge badge-sm"
							:class="severityClass(cvssSeverity(g.max_cvss))"
						>
							{{ formatCvss(g.max_cvss) }}
						</span>
						<span class="font-medium">{{ g.name }}</span>
						<span class="sub-text item-meta">
							<template v-if="g.overdue_count"
								>{{ g.overdue_count }} overdue ·
							</template>
							{{ g.sites.length }} site{{
								g.sites.length === 1 ? "" : "s"
							}}
						</span>
					</RouterLink>
				</li>
			</ul>
			<p v-else class="sub-text watching">
				No site runs a plugin or core version with a known
				vulnerability.
			</p>
			<p
				v-if="
					vulnStore.summary &&
					(vulnStore.summary.muted || vulnStore.summary.needs_review)
				"
				class="sub-text watching"
			>
				{{ vulnStore.summary.muted }} muted<template
					v-if="vulnStore.summary.needs_review"
					>, {{ vulnStore.summary.needs_review }} need
					review</template
				>.
			</p>
			<footer class="area-foot">
				<RouterLink
					:to="{
						path: '/monitoring',
						query: { tab: 'vulnerabilities' },
					}"
					class="btn btn-text btn-sm"
				>
					Open Vulnerabilities →
				</RouterLink>
			</footer>
		</section>
	</div>
</template>

<style scoped>
.summary-grid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
	gap: var(--space-4);
	align-items: start;
}

.area-card {
	display: flex;
	flex-direction: column;
	gap: var(--space-3);
}

.area-head {
	display: flex;
	align-items: center;
	gap: var(--space-3);

	h2 {
		margin: 0;
		padding: 0;
		border: none;
		font-size: 16px;
	}
}

.area-count {
	padding: 2px 10px;
	border-radius: 999px;
	font-size: 12px;
	font-weight: 600;
}

.count-alert {
	background: var(--error-bg);
	color: var(--error-text);
}

.count-clear {
	background: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.area-items {
	list-style: none;
	margin: 0;
	padding: 0;
	display: flex;
	flex-direction: column;
}

.area-items li + li {
	border-top: 1px solid var(--border-color);
}

.item-link {
	display: flex;
	align-items: center;
	gap: var(--space-2);
	padding: var(--space-2) 0;
	color: var(--text-heading);
	text-decoration: none;
	min-width: 0;
}

.item-link:hover .font-medium {
	text-decoration: underline;
}

.item-meta {
	margin-left: auto;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	min-width: 0;
}

.watching {
	margin: 0;
}

.area-foot {
	margin-top: auto;
	display: flex;
	justify-content: flex-end;
}
</style>
