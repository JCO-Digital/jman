<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useOrganizationStore } from "../../stores/organization";
import type { OrganizationAsset, OrganizationAssetStatus } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";

const router = useRouter();
const assetStore = useAssetStore();
const organizationStore = useOrganizationStore();

const showEditModal = ref(false);
const editingAsset = ref<OrganizationAsset | null>(null);

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

const openEditModal = (asset: OrganizationAsset, event: Event) => {
	event.stopPropagation();
	editingAsset.value = asset;
	showEditModal.value = true;
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
				<button class="btn btn-secondary" @click="goToTemplates">
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
									class="icon-btn"
									title="Edit Asset"
									@click="openEditModal(asset, $event)"
								>
									<AppIcon name="edit" size="18" />
								</button>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</main>

		<!-- Asset Edit Modal Placeholder -->
		<div
			v-if="showEditModal"
			class="modal-overlay"
			@click.self="showEditModal = false"
		>
			<div class="modal-content card">
				<h2>Edit Asset</h2>
				<p>
					Editing <strong>{{ editingAsset?.asset_name }}</strong> for
					{{ editingAsset?.organization_name }}
				</p>
				<!-- Add form fields here if needed, or link to org detail -->
				<button class="btn btn-outline" @click="showEditModal = false">
					Close
				</button>
			</div>
		</div>
	</div>
</template>

<style scoped>
.controls-row {
	display: flex;
	gap: 1rem;
	margin-bottom: 1.5rem;
}

.search-input {
	flex: 1;
	padding: 0.625rem;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 0.875rem;
}

.status-select {
	width: 180px;
	padding: 0.625rem;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 0.875rem;
}

.loading-container {
	padding: 3rem;
	display: flex;
	justify-content: center;
}

.font-medium {
	font-weight: 500;
}

.sub-text {
	font-size: 0.8rem;
	color: var(--text-muted);
	margin-top: 0.2rem;
	max-width: 250px;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.overdue {
	color: var(--error-text);
	font-weight: 600;
}

.status-badge.active {
	background-color: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.status-badge.paused {
	background-color: var(--badge-drop-in-bg);
	color: var(--badge-drop-in-text);
}

.status-badge.cancelled {
	background-color: var(--badge-inactive-bg);
	color: var(--badge-inactive-text);
}

code {
	background: var(--bg-body);
	padding: 0.2rem 0.4rem;
	border-radius: 4px;
	font-size: 0.9em;
	color: var(--primary);
}

.btn-secondary {
	display: flex;
	align-items: center;
	gap: 8px;
	background-color: transparent;
	border: 1px solid var(--border-input);
	color: var(--text-main);
}

.btn-secondary:hover {
	background-color: var(--bg-hover);
}

@media (max-width: 768px) {
	.controls-row {
		flex-direction: column;
	}
	.status-select {
		width: 100%;
	}
}
</style>
