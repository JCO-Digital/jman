<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useOrganizationStore } from "../../stores/organization";
import type {
	OrganizationAsset,
	OrganizationAssetStatus,
	Site,
} from "../../types";
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
const selectedOrgId = ref<number | null>(null);

const assets = ref<OrganizationAsset[]>([]);
const isLoading = ref(true);
const searchQuery = ref("");
const statusFilter = ref<OrganizationAssetStatus | "all">("all");
const typeFilter = ref<string>("all");

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

onMounted(async () => {
	loadAssets();
	await organizationStore.fetchOrganizations();
});

const organizations = computed(() => organizationStore.organizations);

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

		const matchesType =
			typeFilter.value === "all" ||
			(a.asset_type || "General") === typeFilter.value;

		return matchesSearch && matchesStatus && matchesType;
	});
});

const goToOrganization = (id: number) => {
	router.push({ name: "organization-detail", params: { id: id.toString() } });
};

const openAddModal = () => {
	editingAsset.value = null;
	selectedOrgId.value = null;
	organizationSites.value = [];
	showEditModal.value = true;
};

const openEditModal = async (asset: OrganizationAsset, event: Event) => {
	event.stopPropagation();
	editingAsset.value = asset;
	selectedOrgId.value = asset.organization_id;

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

const handleOrgChange = async () => {
	if (selectedOrgId.value) {
		try {
			organizationSites.value =
				await organizationStore.fetchOrganizationSites(
					selectedOrgId.value,
				);
		} catch (e) {
			console.error("Failed to load organization sites", e);
			organizationSites.value = [];
		}
	} else {
		organizationSites.value = [];
	}
};

const handleAssetSaved = () => {
	toast.addToast(
		editingAsset.value
			? "Asset updated successfully"
			: "Asset added successfully",
		"success",
	);
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
				<div class="flex-row gap-2 items-center">
					<button class="btn btn-primary" @click="openAddModal">
						<AppIcon name="plus-circle" size="18" />
						<span>Add Asset</span>
					</button>
					<button class="btn btn-secondary" @click="goToTemplates">
						<AppIcon name="tag" size="18" />
						<span>Manage Templates</span>
					</button>
				</div>
			</template>
		</ViewHeader>

		<div class="controls-row">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search by customer, asset or identifier..."
				class="search-input"
			/>
			<select v-model="typeFilter" class="type-select">
				<option value="all">All Types</option>
				<option value="Plugin">Plugin</option>
				<option value="Domain">Domain</option>
				<option value="Hosting Package">Hosting Package</option>
				<option value="Service Package">Service Package</option>
				<option value="General">General</option>
			</select>
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
							<th>Type</th>
							<th>Asset</th>
							<th>Price</th>
							<th>Next Billing</th>
							<th>Status</th>
							<th style="width: 40px"></th>
						</tr>
					</thead>
					<tbody>
						<tr v-if="filteredAssets.length === 0">
							<td colspan="7" class="empty-state">
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
								{{ asset.asset_type || "General" }}
							</td>
							<td
								:title="
									asset.identifier
										? `Identifier: ${asset.identifier}`
										: ''
								"
							>
								<strong>{{
									asset.asset_name || "Custom"
								}}</strong>
								<div v-if="asset.description" class="sub-text">
									{{ asset.description }}
								</div>
							</td>
							<td>
								{{ formatCurrency(asset.price) }}
								<span class="text-muted text-sm ml-1">
									{{ asset.billing_freq }}
								</span>
							</td>
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
									class="icon-btn icon-btn-sm"
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
			:organizations="organizations"
			v-model:organization-id="selectedOrgId"
			@saved="handleAssetSaved"
			@update:organization-id="handleOrgChange"
		/>
	</div>
</template>

<style scoped></style>
