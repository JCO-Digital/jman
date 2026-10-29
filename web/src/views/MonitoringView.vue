<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useIncidentStore } from "../stores/incidents";
import { useVulnStore } from "../stores/vulns";
import ViewHeader from "../components/ViewHeader.vue";
import AppIcon from "../components/AppIcon.vue";
import MonitoringSummary from "../components/monitoring/MonitoringSummary.vue";
import UptimePanel from "../components/monitoring/UptimePanel.vue";
import SecurityPanel from "../components/monitoring/SecurityPanel.vue";
import VulnerabilitiesPanel from "../components/monitoring/VulnerabilitiesPanel.vue";

const incidentStore = useIncidentStore();
const vulnStore = useVulnStore();
const route = useRoute();
const router = useRouter();

type Tab = "summary" | "uptime" | "security" | "vulnerabilities";
const tabs: { key: Tab; label: string }[] = [
	{ key: "summary", label: "Summary" },
	{ key: "uptime", label: "Uptime" },
	{ key: "security", label: "Security" },
	{ key: "vulnerabilities", label: "Vulnerabilities" },
];

// Tabs from before the areas had sections, still in bookmarks and old
// links, and the area and section each now opens.
const legacyTabs: Record<string, { tab: Tab; section?: string }> = {
	active: { tab: "uptime" },
	checks: { tab: "uptime", section: "checks" },
	history: { tab: "uptime", section: "history" },
};

// The tab, section and site filter live in the URL
// (?tab=uptime&section=checks&site=<id>), so other pages and Slack alerts
// can link straight to them. The summary is the default and left out.
const activeTab = computed<Tab>(() => {
	const t = route.query.tab;
	return tabs.some((x) => x.key === t) ? (t as Tab) : "summary";
});
const siteFilter = computed(() =>
	typeof route.query.site === "string" ? route.query.site : undefined,
);

watch(
	() => route.query.tab,
	(t) => {
		const legacy = typeof t === "string" ? legacyTabs[t] : undefined;
		if (!legacy) return;
		router.replace({
			query: { ...route.query, tab: legacy.tab, section: legacy.section },
		});
	},
	{ immediate: true },
);

function tabCount(tab: Tab): number {
	if (tab === "uptime") return incidentStore.activeByType.uptime ?? 0;
	if (tab === "security") return incidentStore.activeByType.security ?? 0;
	if (tab === "vulnerabilities") return vulnStore.summary?.overdue ?? 0;
	return 0;
}

function switchTab(tab: Tab) {
	if (tab === activeTab.value) return;
	router.replace({ query: { tab: tab === "summary" ? undefined : tab } });
}

const panel = ref<{ refresh: () => unknown } | null>(null);

function refresh() {
	incidentStore.fetchActiveCount();
	vulnStore.fetchSummary();
	panel.value?.refresh();
}
</script>

<template>
	<div class="view-container monitoring-view">
		<ViewHeader title="Monitoring">
			<template #actions>
				<button class="btn btn-outline btn-sm" @click="refresh">
					<AppIcon name="refresh" size="14" />
					Refresh
				</button>
			</template>
		</ViewHeader>

		<div class="area-tabs" role="tablist">
			<button
				v-for="t in tabs"
				:key="t.key"
				class="tab-btn"
				role="tab"
				:aria-selected="activeTab === t.key"
				:class="{ active: activeTab === t.key }"
				@click="switchTab(t.key)"
			>
				{{ t.label }}
				<span v-if="tabCount(t.key) > 0" class="tab-badge">
					{{ tabCount(t.key) }}
				</span>
			</button>
		</div>

		<MonitoringSummary v-if="activeTab === 'summary'" ref="panel" />
		<UptimePanel
			v-else-if="activeTab === 'uptime'"
			ref="panel"
			:site-id="siteFilter"
			@clear-site="
				router.replace({ query: { ...route.query, site: undefined } })
			"
		/>
		<SecurityPanel v-else-if="activeTab === 'security'" ref="panel" />
		<VulnerabilitiesPanel
			v-else-if="activeTab === 'vulnerabilities'"
			ref="panel"
		/>
	</div>
</template>

<style scoped>
.monitoring-view {
	max-width: 1400px;
}

.area-tabs {
	display: flex;
	overflow-x: auto;
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
	background: var(--danger, #dc2626);
	color: #fff;
}
</style>
