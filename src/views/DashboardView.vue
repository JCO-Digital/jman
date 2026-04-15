<script setup lang="ts">
import { useDataStore } from "../stores/data";

const dataStore = useDataStore();
</script>

<template>
	<div class="view-container">
		<header class="header">
			<h1>Dashboard</h1>
			<button
				class="btn btn-primary"
				@click="dataStore.refreshData()"
				:disabled="dataStore.isLoading"
			>
				<span
					v-if="dataStore.isLoading"
					class="spinner spinner-small"
					style="margin-right: 8px; vertical-align: middle"
				></span>
				<span style="vertical-align: middle">{{
					dataStore.isLoading ? "Refreshing..." : "Refresh Data"
				}}</span>
			</button>
		</header>

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<main class="dashboard-grid">
			<div class="card">
				<h3>Sites</h3>
				<div class="stat-value">
					{{ dataStore.sites.length }}
				</div>
				<p class="stat-label">Total sites in cache</p>
			</div>

			<div class="card">
				<h3>Plugins</h3>
				<div class="stat-value">
					{{ dataStore.pluginInfo.length }}
				</div>
				<p class="stat-label">Unique plugins in cache</p>
			</div>

			<div class="card">
				<h3>Vulnerabilities</h3>
				<div
					class="stat-value"
					:style="{
						color: dataStore.vulnerabilities.length > 0 ? '#d32f2f' : 'inherit',
					}"
				>
					<span v-if="dataStore.isVulnsLoading" class="spinner"></span>
					<span v-else>{{ dataStore.vulnerabilities.length }}</span>
				</div>
				<p class="stat-label">Active vulnerabilities detected</p>
			</div>
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

.stat-value {
	font-size: 3rem;
	font-weight: 700;
	color: var(--primary-color);
	margin: 12px 0;
}

.stat-label {
	color: var(--text-muted);
	font-size: 0.9rem;
}
</style>
