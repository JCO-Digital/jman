<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useIncidentStore } from "../stores/incidents";
import { useVulnStore } from "../stores/vulns";
import ViewHeader from "../components/ViewHeader.vue";
import RouteTabs, { type RouteTab } from "../components/RouteTabs.vue";
import AppIcon from "../components/AppIcon.vue";
import MonitoringSummary from "../components/monitoring/MonitoringSummary.vue";
import UptimePanel from "../components/monitoring/UptimePanel.vue";
import SecurityPanel from "../components/monitoring/SecurityPanel.vue";
import VulnerabilitiesPanel from "../components/monitoring/VulnerabilitiesPanel.vue";

// /monitoring[/<area>[/<section>]][?site=<id>]: the area and section are
// the path, so other pages and Slack alerts can link straight to them.
// The router turns the old ?tab=&section= links into these.
const props = defineProps<{
	area?: string;
	section?: string;
}>();

const incidentStore = useIncidentStore();
const vulnStore = useVulnStore();
const route = useRoute();
const router = useRouter();

type Area = "summary" | "uptime" | "security" | "vulnerabilities";

const activeArea = computed<Area>(() => (props.area as Area) || "summary");
const siteFilter = computed(() =>
	typeof route.query.site === "string" ? route.query.site : undefined,
);

const tabs = computed<RouteTab[]>(() => [
	{ key: "summary", label: "Summary", to: { name: "monitoring" } },
	{
		key: "uptime",
		label: "Uptime",
		to: { name: "monitoring", params: { area: "uptime" } },
		count: incidentStore.activeByType.uptime ?? 0,
	},
	{
		key: "security",
		label: "Security",
		to: { name: "monitoring", params: { area: "security" } },
		count: incidentStore.activeByType.security ?? 0,
	},
	{
		key: "vulnerabilities",
		label: "Vulnerabilities",
		to: { name: "monitoring", params: { area: "vulnerabilities" } },
		count: vulnStore.summary?.overdue ?? 0,
	},
]);

const panel = ref<{ refresh: () => unknown } | null>(null);

function refresh() {
	incidentStore.fetchActiveCount();
	vulnStore.fetchSummary();
	panel.value?.refresh();
}
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Monitoring">
			<template #actions>
				<button class="btn btn-outline btn-sm" @click="refresh">
					<AppIcon name="refresh" size="14" />
					Refresh
				</button>
			</template>
			<template #tabs>
				<RouteTabs :tabs="tabs" :active="activeArea" label="Areas" />
			</template>
		</ViewHeader>

		<MonitoringSummary v-if="activeArea === 'summary'" ref="panel" />
		<UptimePanel
			v-else-if="activeArea === 'uptime'"
			ref="panel"
			:site-id="siteFilter"
			@clear-site="
				router.replace({ query: { ...route.query, site: undefined } })
			"
		/>
		<SecurityPanel v-else-if="activeArea === 'security'" ref="panel" />
		<VulnerabilitiesPanel
			v-else-if="activeArea === 'vulnerabilities'"
			ref="panel"
		/>
	</div>
</template>
