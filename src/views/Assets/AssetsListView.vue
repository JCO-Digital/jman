<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useOrganizationStore } from "../../stores/organization";
import { useDataStore } from "../../stores/data";
import type {
	OrganizationAsset,
	OrganizationAssetStatus,
	Site,
} from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import Pagination from "../../components/Pagination.vue";
import AppIcon from "../../components/AppIcon.vue";

import AssetEditModal from "../../components/AssetEditModal.vue";
import { useToastStore } from "../../stores/toast";

const props = defineProps<{
	page?: number;
	rowsPerPage?: number;
}>();

const router = useRouter();
const assetStore = useAssetStore();
const organizationStore = useOrganizationStore();
const dataStore = useDataStore();
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

const currentPage = ref(props.page || 1);
const rowsPerPage = ref(props.rowsPerPage || 50);

const sortKey = ref<
	| "organization_name"
	| "asset_type"
	| "asset_name"
	| "price"
	| "next_billing"
	| "status"
>("organization_name");
const sortOrder = ref<"asc" | "desc">("asc");

watch(
	() => props.page,
	(newVal) => {
		currentPage.value = newVal || 1;
	},
);

watch(
	() => props.rowsPerPage,
	(newVal) => {
		rowsPerPage.value = newVal || 50;
	},
);

const updateRoute = (page: number, rpp: number) => {
	router.push({
		name: "assets",
		params: {
			page: page.toString(),
			rowsPerPage: rpp.toString(),
		},
	});
};

const prevPage = () => {
	if (currentPage.value > 1) {
		updateRoute(currentPage.value - 1, rowsPerPage.value);
	}
};

const nextPage = () => {
	if (currentPage.value < totalPages.value) {
		updateRoute(currentPage.value + 1, rowsPerPage.value);
	}
};

const handleRowsPerPageUpdate = (newRpp: number) => {
	updateRoute(1, newRpp);
};

const handleSort = (key: typeof sortKey.value) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
};

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
	await dataStore.initData();
});

const getLinkedSite = (siteId: number | null) => {
	if (!siteId) return null;
	return dataStore.getSiteById(siteId) || null;
};

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

const sortedAssets = computed(() => {
	const result = [...filteredAssets.value];

	result.sort((a, b) => {
		let valA: any = a[sortKey.value];
		let valB: any = b[sortKey.value];

		// Handle undefined/null values
		if (valA === undefined || valA === null) valA = "";
		if (valB === undefined || valB === null) valB = "";

		if (typeof valA === "string") {
			valA = valA.toLowerCase();
		}
		if (typeof valB === "string") {
			valB = valB.toLowerCase();
		}

		if (valA < valB) return sortOrder.value === "asc" ? -1 : 1;
		if (valA > valB) return sortOrder.value === "asc" ? 1 : -1;
		return 0;
	});

	return result;
});

const totalPages = computed(
	() => Math.ceil(sortedAssets.value.length / rowsPerPage.value) || 1,
);

const paginatedAssets = computed(() => {
	const start = (currentPage.value - 1) * rowsPerPage.value;
	const end = start + rowsPerPage.value;
	return sortedAssets.value.slice(start, end);
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
				@input="updateRoute(1, rowsPerPage)"
			/>
			<select
				v-model="typeFilter"
				class="type-select"
				@change="updateRoute(1, rowsPerPage)"
			>
				<option value="all">All Types</option>
				<option value="Plugin">Plugin</option>
				<option value="Domain">Domain</option>
				<option value="Hosting Package">Hosting Package</option>
				<option value="Service Package">Service Package</option>
				<option value="General">General</option>
			</select>
			<select
				v-model="statusFilter"
				class="status-select"
				@change="updateRoute(1, rowsPerPage)"
			>
				<option value="all">All Statuses</option>
				<option value="active">Active</option>
				<option value="paused">Paused</option>
				<option value="cancelled">Cancelled</option>
			</select>
		</div>

		<main class="table-container">
			<div v-if="isLoading" class="loading-container">
				<LoadingSpinner message="Loading all assets..." />
			</div>

			<template v-else>
				<table class="data-table sortable">
					<thead>
						<tr>
							<th @click="handleSort('organization_name')">
								Customer
								<span v-if="sortKey === 'organization_name'">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
							<th @click="handleSort('asset_type')">
								Type
								<span v-if="sortKey === 'asset_type'">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
							<th @click="handleSort('asset_name')">
								Asset
								<span v-if="sortKey === 'asset_name'">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
							<th @click="handleSort('price')">
								Price
								<span v-if="sortKey === 'price'">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
							<th @click="handleSort('next_billing')">
								Next Billing
								<span v-if="sortKey === 'next_billing'">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
							<th @click="handleSort('status')">
								Status
								<span v-if="sortKey === 'status'">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
							<th style="width: 40px"></th>
						</tr>
					</thead>
					<tbody>
						<tr v-if="paginatedAssets.length === 0">
							<td colspan="7" class="empty-state">
								No assets found matching your filters.
							</td>
						</tr>
						<tr
							v-for="asset in paginatedAssets"
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
								<div
									v-if="
										asset.identifier ||
										(getLinkedSite(asset.site_id) &&
											getLinkedSite(asset.site_id)
												?.domain !== asset.identifier)
									"
									class="sub-text"
									style="
										display: flex;
										align-items: center;
										flex-wrap: wrap;
										gap: 6px;
									"
								>
									<span
										v-if="asset.identifier"
										class="identifier"
										>{{ asset.identifier }}</span
									>
									<span
										v-if="
											asset.identifier &&
											getLinkedSite(asset.site_id) &&
											getLinkedSite(asset.site_id)
												?.domain !== asset.identifier
										"
										class="separator"
										>•</span
									>
									<span
										v-if="
											getLinkedSite(asset.site_id) &&
											getLinkedSite(asset.site_id)
												?.domain !== asset.identifier
										"
										class="linked-site text-muted"
										style="
											display: inline-flex;
											align-items: center;
											gap: 4px;
										"
									>
										<AppIcon name="site" size="14" />
										{{
											getLinkedSite(asset.site_id)?.domain
										}}
									</span>
								</div>
								<div
									v-if="asset.description"
									class="sub-text text-muted"
									style="margin-top: 4px"
								>
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
				<Pagination
					:current-page="currentPage"
					:total-pages="totalPages"
					:rows-per-page="rowsPerPage"
					@update:rows-per-page="handleRowsPerPageUpdate"
					@prev="prevPage"
					@next="nextPage"
				/>
			</template>
		</main>

		<AssetEditModal
			v-model="showEditModal"
			v-model:organization-id="selectedOrgId"
			:asset="editingAsset"
			:sites="organizationSites"
			:organizations="organizations"
			@saved="handleAssetSaved"
			@update:organization-id="handleOrgChange"
		/>
	</div>
</template>

<style scoped></style>
