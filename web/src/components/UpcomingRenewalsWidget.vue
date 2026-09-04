<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useAssetStore } from "../stores/assetStore";
import type { OrganizationAsset } from "../types";

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

onMounted(loadRenewals);
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Upcoming Renewals (30 Days)</h2>
		</div>
		<div v-if="isRenewalsLoading" class="loading-state p-4">
			<p>Loading renewals...</p>
		</div>
		<div v-else-if="upcomingRenewals.length === 0" class="p-4 text-muted">
			<p>No upcoming renewals in the next 30 days.</p>
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
</template>

<style scoped>
.loading-state,
.p-4 {
	padding: var(--space-5);
	text-align: center;
}
</style>
