<script setup lang="ts">
import { useDataStore } from "../stores/data";
import ViewHeader from "../components/ViewHeader.vue";
import StatCard from "../components/StatCard.vue";

const dataStore = useDataStore();
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Dashboard" />

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<main class="dashboard-grid">
			<StatCard
				title="Sites"
				:value="dataStore.sites.length"
				label="Total sites in cache"
			/>

			<StatCard
				title="Plugins"
				:value="dataStore.pluginInfo.length"
				label="Unique plugins in cache"
			/>

			<StatCard
				title="Vulnerabilities"
				:value="dataStore.vulnerabilities.length"
				label="Active vulnerabilities detected"
				:loading="dataStore.isVulnsLoading"
				:value-style="{
					color: dataStore.vulnerabilities.length > 0 ? '#d32f2f' : 'inherit',
				}"
			/>
		</main>
	</div>
</template>

<style scoped>
.dashboard-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
	gap: 24px;
	margin-top: 24px;
}
</style>
