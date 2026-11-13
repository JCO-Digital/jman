<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useOrganizationStore } from "../../stores/organization";
import { useDataStore } from "../../stores/data";
import type { OrganizationAsset, Site } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import ListToolbar from "../../components/ListToolbar.vue";
import SortableTh from "../../components/SortableTh.vue";
import Pagination from "../../components/Pagination.vue";
import AppIcon from "../../components/AppIcon.vue";

import AssetEditModal from "../../components/AssetEditModal.vue";
import { useToastStore } from "../../stores/toast";
import { useListState } from "../../composables/useListState";
import { useClientList } from "../../composables/useClientList";

const router = useRouter();
const assetStore = useAssetStore();
const organizationStore = useOrganizationStore();
const dataStore = useDataStore();
const toast = useToastStore();

const showEditModal = ref(false);
const editingAsset = ref<OrganizationAsset | null>(null);

const revealedKeys = ref<Record<number, boolean>>({});
const toggleReveal = (id: number) => {
	revealedKeys.value[id] = !revealedKeys.value[id];
};
const copyKey = async (keyText: string) => {
	try {
		await navigator.clipboard.writeText(keyText);
		toast.addToast("License key copied to clipboard", "success");
	} catch (err: any) {
		toast.addToast("Failed to copy to clipboard", "error");
	}
};
const organizationSites = ref<Site[]>([]);
const selectedOrgId = ref<number | null>(null);

const assets = ref<OrganizationAsset[]>([]);
const isLoading = ref(true);

const ASSET_TYPES = [
	"Plugin",
	"Domain",
	"Hosting Package",
	"Service Package",
	"General",
] as const;

// /billing?q=&sort=&dir=&page=&per=&type=&status=
const list = useListState({
	sortKeys: ["customer", "type", "asset", "price", "billing", "status"],
	defaultSort: "customer",
	descFirst: ["price"],
	filters: {
		type: ASSET_TYPES,
		status: ["active", "paused", "cancelled"],
	},
});

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

const getLinkedSite = (siteId: string | null | undefined) => {
	if (siteId == null) return null;
	return dataStore.getSiteById(siteId) || null;
};

const organizations = computed(() => organizationStore.organizations);

const assetList = useClientList(assets, list, {
	search: (a, q) =>
		Boolean(
			a.organization_name?.toLowerCase().includes(q) ||
			a.asset_name?.toLowerCase().includes(q) ||
			a.identifier?.toLowerCase().includes(q),
		),
	filter: (a) => {
		const f = list.filters.value;
		if (f.status && a.status !== f.status) return false;
		if (f.type && (a.asset_type || "General") !== f.type) return false;
		return true;
	},
	sortValue: (a, key) => {
		switch (key) {
			case "customer":
				return a.organization_name;
			case "type":
				return a.asset_type || "General";
			case "asset":
				return a.asset_name;
			case "price":
				return a.price;
			case "billing":
				return a.next_billing;
			case "status":
				return a.status;
		}
	},
});

const paginatedAssets = assetList.pageItems;

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

		<ListToolbar
			:list="list"
			placeholder="Search by customer, asset or identifier"
		>
			<select
				:value="list.filters.value.type"
				aria-label="Type"
				@change="
					list.setFilter(
						'type',
						($event.target as HTMLSelectElement).value,
					)
				"
			>
				<option value="">All types</option>
				<option v-for="t in ASSET_TYPES" :key="t" :value="t">
					{{ t }}
				</option>
			</select>
			<select
				:value="list.filters.value.status"
				aria-label="Status"
				@change="
					list.setFilter(
						'status',
						($event.target as HTMLSelectElement).value,
					)
				"
			>
				<option value="">All statuses</option>
				<option value="active">Active</option>
				<option value="paused">Paused</option>
				<option value="cancelled">Cancelled</option>
			</select>
		</ListToolbar>

		<main class="table-container">
			<div
				v-if="isLoading && assets.length === 0"
				class="loading-container"
			>
				<LoadingSpinner message="Loading all assets..." />
			</div>

			<template v-else>
				<table class="data-table">
					<thead>
						<tr>
							<SortableTh :list="list" sort-key="customer">
								Customer
							</SortableTh>
							<SortableTh :list="list" sort-key="type">
								Type
							</SortableTh>
							<SortableTh :list="list" sort-key="asset">
								Asset
							</SortableTh>
							<SortableTh :list="list" sort-key="price">
								Price
							</SortableTh>
							<SortableTh :list="list" sort-key="billing">
								Next Billing
							</SortableTh>
							<SortableTh :list="list" sort-key="status">
								Status
							</SortableTh>
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
								<div
									v-if="
										asset.asset_purchase_price ||
										asset.payment_method_name
									"
									class="sub-text text-muted"
									style="margin-top: 4px"
								>
									<template v-if="asset.asset_purchase_price">
										Purchase:
										{{
											formatCurrency(
												asset.asset_purchase_price,
											)
										}}
										<template
											v-if="
												(asset.asset_quantity || 1) > 1
											"
											>/
											{{ asset.asset_quantity }}</template
										>
									</template>
									<span v-if="asset.payment_method_name">
										<template
											v-if="asset.asset_purchase_price"
											>·
										</template>
										{{ asset.payment_method_name }}</span
									>
								</div>
								<div
									v-if="
										asset.license_key ||
										asset.asset_license_key
									"
									class="sub-text text-muted"
									style="
										margin-top: 4px;
										display: flex;
										align-items: center;
										gap: 6px;
									"
									@click.stop
								>
									<span
										>Key:
										<code style="font-family: monospace">{{
											revealedKeys[asset.id]
												? asset.license_key ||
													asset.asset_license_key
												: "••••••••••••••••"
										}}</code></span
									>
									<button
										type="button"
										class="icon-btn icon-btn-sm"
										:title="
											revealedKeys[asset.id]
												? 'Hide'
												: 'Reveal'
										"
										@click="toggleReveal(asset.id)"
										style="
											padding: 2px 4px;
											display: inline-flex;
										"
									>
										<AppIcon
											:name="
												revealedKeys[asset.id]
													? 'eye-off'
													: 'eye'
											"
											size="14"
										/>
									</button>
									<button
										type="button"
										class="icon-btn icon-btn-sm"
										title="Copy Key"
										@click="
											copyKey(
												asset.license_key ||
													asset.asset_license_key ||
													'',
											)
										"
										style="
											padding: 2px 4px;
											display: inline-flex;
										"
									>
										<AppIcon name="copy" size="14" />
									</button>
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
					:list="list"
					:page="assetList.page.value"
					:total="assetList.total.value"
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
