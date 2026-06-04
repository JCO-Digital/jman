<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useOrganizationStore } from "../../stores/organization";
import type { OrganizationAsset, OrganizationAssetStatus } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";

import AssetEditModal from "../../components/AssetEditModal.vue";
import { useToastStore } from "../../stores/toast";

const router = useRouter();
const assetStore = useAssetStore();
const organizationStore = useOrganizationStore();
const toast = useToastStore();

const showEditModal = ref(false);
const editingAsset = ref<OrganizationAsset | null>(null);
const organizationSites = ref<Site[]>([]);

const assets = ref<OrganizationAsset[]>([]);
const isLoading = ref(true);
const searchQuery = ref("");
const statusFilter = ref<OrganizationAssetStatus | "all">("all");

const loadAssets = async () => {
	isLoading.value = true;
	try {
		assets.value = await assetStore.fetchAllOrganizationAssets();
	} catch (e) {
		console.error("Failed to load assets", e);
	} finally {
		isLoading.value = false;
	}
};

onMounted(() => {
	loadAssets();
	organizationStore.fetchOrganizations();
});

const filteredAssets = computed(() => {
	return assets.value.filter((a) => {
		const matchesSearch =
			!searchQuery.value ||
			a.organization_name
				?.toLowerCase()
				.includes(searchQuery.value.toLowerCase()) ||
			a.asset_name
				?.toLowerCase()
				.includes(searchQuery.value.toLowerCase()) ||
			a.identifier
				?.toLowerCase()
				.includes(searchQuery.value.toLowerCase());

		const matchesStatus =
			statusFilter.value === "all" || a.status === statusFilter.value;

		return matchesSearch && matchesStatus;
	});
});

const goToOrganization = (id: number) => {
	router.push({ name: "organization-detail", params: { id: id.toString() } });
};

const openEditModal = async (asset: OrganizationAsset, event: Event) => {
	event.stopPropagation();
	editingAsset.value = asset;

	// Load sites for this organization to allow linking asset to a site
	try {
		organizationSites.value =
			await organizationStore.fetchOrganizationSites(
				asset.organization_id,
			);
	} catch (e) {
		console.error("Failed to load organization sites", e);
		organizationSites.value = [];
	}

	showEditModal.value = true;
};

const handleAssetSaved = () => {
	toast.addToast("Asset updated successfully", "success");
	loadAssets();
};

const goToTemplates = () => {
	router.push({ name: "asset-templates" });
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
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Assets & Subscriptions">
			<template #actions>
				<button class="btn btn-outline" @click="goToTemplates">
					<AppIcon name="tag" size="18" />
					Manage Templates
				</button>
			</template>
		</ViewHeader>

		<div class="controls-row">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search by customer, asset or identifier..."
				class="search-input"
			/>
			<select v-model="statusFilter" class="status-select">
				<option value="all">All Statuses</option>
				<option value="active">Active</option>
				<option value="paused">Paused</option>
				<option value="cancelled">Cancelled</option>
			</select>
		</div>

		<main class="content">
			<div v-if="isLoading" class="loading-container">
				<LoadingSpinner message="Loading all assets..." />
			</div>

			<div v-else class="card table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Customer</th>
							<th>Asset</th>
							<th>Identifier</th>
							<th>Price</th>
							<th>Frequency</th>
							<th>Next Billing</th>
							<th>Status</th>
							<th style="width: 40px"></th>
						</tr>
					</thead>
					<tbody>
						<tr v-if="filteredAssets.length === 0">
							<td colspan="8" class="empty-state">
								No assets found matching your filters.
							</td>
						</tr>
						<tr
							v-for="asset in filteredAssets"
							:key="asset.id"
							class="clickable-row"
							@click="goToOrganization(asset.organization_id)"
						>
							<td class="font-medium">
								{{ asset.organization_name }}
							</td>
							<td>
								<strong>{{
									asset.asset_name || "Custom"
								}}</strong>
								<div v-if="asset.description" class="sub-text">
									{{ asset.description }}
								</div>
							</td>
							<td>
								<code>{{ asset.identifier || "-" }}</code>
							</td>
							<td>{{ formatCurrency(asset.price) }}</td>
							<td>{{ asset.billing_freq }}</td>
							<td
								:class="{
									overdue:
										new Date(asset.next_billing || '') <
											new Date() &&
										asset.status === 'active',
								}"
							>
								{{ formatDate(asset.next_billing) }}
							</td>
							<td>
								<span :class="['status-badge', asset.status]">
									{{ asset.status }}
								</span>
							</td>
							<td class="text-right" @click.stop>
								<button
									class="btn btn-text"
									title="Edit Asset"
									@click="openEditModal(asset, $event)"
								>
									<AppIcon name="edit" size="18" />
									<span class="show-mobile">Edit</span>
								</button>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</main>

		<AssetEditModal
			v-model="showEditModal"
			:asset="editingAsset"
			:sites="organizationSites"
			@saved="handleAssetSaved"
		/>
	</div>
</template>

<style scoped></style>
