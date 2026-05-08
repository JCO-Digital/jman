<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useDataStore } from "../stores/data";
import { useAssetStore } from "../stores/assetStore";
import type { OrganizationAsset } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import StatCard from "../components/StatCard.vue";
import VulnerabilityWidget from "../components/VulnerabilityWidget.vue";

const dataStore = useDataStore();
const assetStore = useAssetStore();

const upcomingRenewals = ref<OrganizationAsset[]>([]);
const isRenewalsLoading = ref(false);

const loadRenewals = async () => {
	isRenewalsLoading.value = true;
	try {
		const thirtyDaysFromNow = new Date();
		thirtyDaysFromNow.setDate(thirtyDaysFromNow.getDate() + 30);

		upcomingRenewals.value = await assetStore.fetchAllOrganizationAssets({
			status: "active",
			before: thirtyDaysFromNow.toISOString(),
		});

		// Sort by next_billing
		upcomingRenewals.value.sort((a, b) => {
			if (!a.next_billing) return 1;
			if (!b.next_billing) return -1;
			return (
				new Date(a.next_billing).getTime() -
				new Date(b.next_billing).getTime()
			);
		});
	} catch (e) {
		console.error("Failed to load renewals", e);
	} finally {
		isRenewalsLoading.value = false;
	}
};

onMounted(() => {
	loadRenewals();
});

const formatCurrency = (cents: number) => {
	return new Intl.NumberFormat("de-DE", {
		style: "currency",
		currency: "EUR",
	}).format(cents / 100);
};

const formatDate = (dateString: string | null) => {
	if (!dateString) return "-";
	return new Date(dateString).toLocaleDateString("de-DE");
};
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
					color:
						dataStore.vulnerabilities.length > 0
							? '#d32f2f'
							: 'inherit',
				}"
			/>
		</main>

		<VulnerabilityWidget />

		<section
			v-if="upcomingRenewals.length > 0 || isRenewalsLoading"
			class="card renewals-widget"
		>
			<div class="card-header">
				<h2>Upcoming Renewals (30 Days)</h2>
			</div>
			<div v-if="isRenewalsLoading" class="loading-state">
				<p>Loading renewals...</p>
			</div>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Organization</th>
							<th>Asset</th>
							<th>Price</th>
							<th>Due Date</th>
						</tr>
					</thead>
					<tbody>
						<tr v-for="oa in upcomingRenewals" :key="oa.id">
							<td>{{ oa.organization_name }}</td>
							<td>
								<strong>{{
									oa.asset_name || oa.identifier
								}}</strong>
							</td>
							<td>{{ formatCurrency(oa.price) }}</td>
							<td
								:class="{
									overdue:
										new Date(oa.next_billing || '') <
										new Date(),
								}"
							>
								{{ formatDate(oa.next_billing) }}
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</section>
	</div>
</template>

<style scoped>
.dashboard-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
	gap: 24px;
	margin-top: 24px;
}

.renewals-widget {
	margin-top: 32px;
}

.renewals-widget h2 {
	font-size: 1.25rem;
	margin: 0;
}

.loading-state {
	padding: 2rem;
	text-align: center;
	color: var(--text-muted);
}

.overdue {
	color: var(--error-text);
	font-weight: 600;
}
</style>
