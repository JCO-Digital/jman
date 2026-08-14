<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type { Asset, AssetType, BillingFrequency } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import { useConfirm } from "../../composables/useConfirm";

const assetStore = useAssetStore();
const authStore = useAuthStore();
const toast = useToastStore();
const route = useRoute();
const { confirm } = useConfirm();

const searchQuery = ref((route.query.search as string) || "");
const showModal = ref(false);
const editingAsset = ref<Asset | null>(null);

const assetForm = ref({
	type: "Plugin" as AssetType,
	name: "",
	identifier: "",
	description: "",
	default_freq: "Yearly" as BillingFrequency,
	active: true,
});
const priceInput = ref("0.00");

const normalizePriceInput = () => {
	priceInput.value = (parseFloat(priceInput.value) || 0).toFixed(2);
};

const assetTypeOptions: AssetType[] = [
	"Plugin",
	"Domain",
	"Hosting Package",
	"Service Package",
	"General",
];
const freqOptions: BillingFrequency[] = [
	"Yearly",
	"Quarterly",
	"Monthly",
	"One-time",
];

const filteredAssets = computed(() => {
	if (!searchQuery.value) return assetStore.assets;
	const query = searchQuery.value.toLowerCase();
	return assetStore.assets.filter(
		(a) =>
			a.name.toLowerCase().includes(query) ||
			a.identifier?.toLowerCase().includes(query) ||
			a.type.toLowerCase().includes(query),
	);
});

const loadAssets = async () => {
	await assetStore.fetchAssets();

	if (route.query.create === "true") {
		openAddModal();
		assetForm.value.type = (route.query.type as AssetType) || "Plugin";
		assetForm.value.identifier = (route.query.identifier as string) || "";
		assetForm.value.name = (route.query.name as string) || "";
	}
};

onMounted(loadAssets);

const openAddModal = () => {
	editingAsset.value = null;
	assetForm.value = {
		type: "Plugin",
		name: "",
		identifier: "",
		description: "",
		default_freq: "Yearly",
		active: true,
	};
	priceInput.value = "0.00";
	showModal.value = true;
};

const openEditModal = (asset: Asset) => {
	editingAsset.value = asset;
	assetForm.value = {
		type: asset.type,
		name: asset.name,
		identifier: asset.identifier || "",
		description: asset.description || "",
		default_freq: asset.default_freq || "Yearly",
		active: asset.active,
	};
	priceInput.value = ((asset.default_price || 0) / 100).toFixed(2);
	showModal.value = true;
};

const handleSubmit = async () => {
	try {
		const payload = {
			...assetForm.value,
			default_price: Math.round(
				(parseFloat(priceInput.value) || 0) * 100,
			),
		};
		if (editingAsset.value) {
			await assetStore.updateAsset(editingAsset.value.id, payload);
		} else {
			await assetStore.createAsset(payload);
		}
		showModal.value = false;
		await loadAssets();
	} catch (e: any) {
		toast.addToast("Failed to save asset: " + e.message, "error");
	}
};

const handleDelete = async (id: number) => {
	if (
		!(await confirm(
			"Are you sure you want to delete this asset template?",
			{ danger: true },
		))
	)
		return;
	try {
		await assetStore.deleteAsset(id);
		await loadAssets();
	} catch (e: any) {
		toast.addToast("Failed to delete asset: " + e.message, "error");
	}
};

const formatCurrency = (cents: number) => {
	return new Intl.NumberFormat("de-DE", {
		style: "currency",
		currency: "EUR",
	}).format(cents / 100);
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Asset Templates">
			<template #actions>
				<div class="flex-row gap-2 items-center">
					<button
						v-if="authStore.canEdit"
						class="btn btn-primary"
						@click="openAddModal"
					>
						<AppIcon name="plus-circle" size="18" />
						Create Template
					</button>
					<button
						class="btn btn-secondary"
						@click="$router.push({ name: 'assets' })"
					>
						<AppIcon name="tag" size="18" />
						<span>Manage Assets</span>
					</button>
				</div>
			</template>
		</ViewHeader>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search templates by name, type or identifier..."
				class="search-input"
			/>
		</div>

		<main class="content">
			<div v-if="assetStore.isLoading" class="loading-container">
				<LoadingSpinner message="Loading asset templates..." />
			</div>

			<div v-else class="asset-grid">
				<div
					v-if="filteredAssets.length === 0"
					class="card empty-state"
				>
					No asset templates found.
				</div>
				<div
					v-for="asset in filteredAssets"
					:key="asset.id"
					class="card asset-card"
				>
					<div class="asset-card-header">
						<span
							:class="[
								'status-badge',
								asset.active ? 'active' : 'inactive',
							]"
						>
							{{ asset.active ? "Active" : "Inactive" }}
						</span>
						<div v-if="authStore.canEdit" class="row-actions">
							<button
								class="icon-btn icon-btn-sm"
								title="Edit"
								@click="openEditModal(asset)"
							>
								<AppIcon name="edit" size="16" />
							</button>
							<button
								class="icon-btn icon-btn-sm danger"
								title="Delete"
								@click="handleDelete(asset.id)"
							>
								<AppIcon name="trash" size="16" />
							</button>
						</div>
					</div>

					<div class="asset-card-content">
						<h3>{{ asset.name }}</h3>
						<div class="asset-meta">
							<span class="type-tag">{{ asset.type }}</span>
							<code
								v-if="asset.identifier"
								class="identifier-code"
								>{{ asset.identifier }}</code
							>
						</div>
						<p v-if="asset.description" class="description">
							{{ asset.description }}
						</p>
					</div>

					<div class="asset-card-footer">
						<div class="price">
							{{ formatCurrency(asset.default_price || 0) }}
						</div>
						<div class="freq">{{ asset.default_freq }}</div>
					</div>
				</div>
			</div>
		</main>

		<!-- Asset Template Modal -->
		<div
			v-if="showModal"
			class="modal-overlay"
			@click.self="showModal = false"
		>
			<div class="modal-content card">
				<h2>
					{{ editingAsset ? "Edit Template" : "New Asset Template" }}
				</h2>
				<form class="form-layout" @submit.prevent="handleSubmit">
					<div class="form-group">
						<label for="name">Name</label>
						<input
							id="name"
							v-model="assetForm.name"
							type="text"
							required
						/>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="type">Type</label>
							<select id="type" v-model="assetForm.type">
								<option
									v-for="type in assetTypeOptions"
									:key="type"
									:value="type"
								>
									{{ type }}
								</option>
							</select>
						</div>
						<div class="form-group">
							<label for="identifier"
								>System Identifier (Slug/TLD)</label
							>
							<input
								id="identifier"
								v-model="assetForm.identifier"
								type="text"
								placeholder="e.g. wp-rocket or .com"
							/>
						</div>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="price">Default Price (€)</label>
							<input
								id="price"
								v-model="priceInput"
								type="number"
								step="0.01"
								@blur="normalizePriceInput"
							/>
						</div>
						<div class="form-group">
							<label for="freq">Default Frequency</label>
							<select id="freq" v-model="assetForm.default_freq">
								<option
									v-for="freq in freqOptions"
									:key="freq"
									:value="freq"
								>
									{{ freq }}
								</option>
							</select>
						</div>
					</div>

					<div class="form-group">
						<label for="description">Description</label>
						<textarea
							id="description"
							v-model="assetForm.description"
							rows="3"
						></textarea>
					</div>

					<div class="form-group checkbox">
						<label>
							<input v-model="assetForm.active" type="checkbox" />
							Active (available for linking)
						</label>
					</div>

					<div class="form-actions">
						<button
							type="button"
							class="back-btn"
							@click="showModal = false"
						>
							Cancel
						</button>
						<button type="submit" class="btn btn-primary">
							{{
								editingAsset
									? "Update Template"
									: "Create Template"
							}}
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>
