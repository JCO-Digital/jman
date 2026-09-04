<script setup lang="ts">
import { onMounted } from "vue";
import { useRouter } from "vue-router";
import { useReportsStore } from "../../stores/reports";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";

const router = useRouter();
const reportsStore = useReportsStore();

onMounted(() => {
	reportsStore.fetchReports();
});

const openReport = (id: string) => {
	router.push({ name: "report-runner", params: { id } });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Reports" />

		<div
			v-if="reportsStore.isLoading && reportsStore.reports.length === 0"
			class="loading-container"
		>
			<LoadingSpinner message="Loading reports..." />
		</div>

		<div v-else-if="reportsStore.reports.length === 0" class="empty-state">
			No reports available.
		</div>

		<div v-else class="asset-grid">
			<div
				v-for="report in reportsStore.reports"
				:key="report.id"
				class="card clickable-row"
				@click="openReport(report.id)"
			>
				<div class="flex-row gap-2 items-center">
					<AppIcon name="report" size="20" />
					<strong>{{ report.name }}</strong>
				</div>
				<p class="sub-text text-muted" style="margin-top: 8px">
					{{ report.description }}
				</p>
			</div>
		</div>
	</div>
</template>

<style scoped></style>
