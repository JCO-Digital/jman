<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { useAssetStore } from "../../stores/assetStore";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type { Asset, AssetType, BillingFrequency } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";

const assetStore = useAssetStore();
const authStore = useAuthStore();
const toast = useToastStore();
const route = useRoute();

const searchQuery = ref((route.query.search as string) || "");
const showModal = ref(false);
const editingAsset = ref<Asset | null>(null);

const assetForm = ref({
	type: "Plugin" as AssetType,
	name: "",
	identifier: "",
	description: "",
	default_price: 0,
	default_freq: "Yearly" as BillingFrequency,
	active: true,
});

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
		default_price: 0,
		default_freq: "Yearly",
		active: true,
	};
	showModal.value = true;
};

const openEditModal = (asset: Asset) => {
	editingAsset.value = asset;
	assetForm.value = {
		type: asset.type,
		name: asset.name,
		identifier: asset.identifier || "",
		description: asset.description || "",
		default_price: asset.default_price || 0,
		default_freq: asset.default_freq || "Yearly",
		active: asset.active,
	};
	showModal.value = true;
};

const handleSubmit = async () => {
	try {
		if (editingAsset.value) {
			await assetStore.updateAsset(editingAsset.value.id, assetForm.value);
		} else {
			await assetStore.createAsset(assetForm.value);
		}
		showModal.value = false;
		await loadAssets();
	} catch (e: any) {
		toast.addToast("Failed to save asset: " + e.message, "error");
	}
};

const handleDelete = async (id: number) => {
	if (!confirm("Are you sure you want to delete this asset template?")) return;
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
		<ViewHeader
			title="Asset Templates"
			:back-button="{
				text: 'Back to Assets',
				onClick: () => $router.push({ name: 'assets' }),
			}"
		>
			<template #actions>
				<button
					v-if="authStore.canEdit"
					class="btn btn-primary"
					@click="openAddModal"
				>
					Create Template
				</button>
			</template>
		</ViewHeader>

		<div class="controls">
			<input
				type="text"
				v-model="searchQuery"
				placeholder="Search templates by name, type or identifier..."
				class="search-input"
			/>
		</div>

		<main class="content">
			<div v-if="assetStore.isLoading" class="loading-container">
				<LoadingSpinner message="Loading asset templates..." />
			</div>

			<div v-else class="asset-grid">
				<div v-if="filteredAssets.length === 0" class="card empty-state">
					No asset templates found.
				</div>
				<div
					v-for="asset in filteredAssets"
					:key="asset.id"
					class="card asset-card"
				>
					<div class="asset-card-header">
						<span
							:class="['status-badge', asset.active ? 'active' : 'inactive']"
						>
							{{ asset.active ? "Active" : "Inactive" }}
						</span>
						<div v-if="authStore.canEdit" class="row-actions">
							<button
								class="icon-btn-sm"
								@click="openEditModal(asset)"
								title="Edit"
							>
								<svg
									xmlns="http://www.w3.org/2000/svg"
									width="16"
									height="16"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
								>
									<path
										d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"
									></path>
									<path
										d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"
									></path>
								</svg>
							</button>
							<button
								class="icon-btn-sm delete"
								@click="handleDelete(asset.id)"
								title="Delete"
							>
								<svg
									xmlns="http://www.w3.org/2000/svg"
									width="16"
									height="16"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
								>
									<polyline points="3 6 5 6 21 6"></polyline>
									<path
										d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
									></path>
								</svg>
							</button>
						</div>
					</div>

					<div class="asset-card-content">
						<h3>{{ asset.name }}</h3>
						<div class="asset-meta">
							<span class="type-tag">{{ asset.type }}</span>
							<code v-if="asset.identifier" class="identifier-code">{{
								asset.identifier
							}}</code>
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
		<div class="modal-overlay" v-if="showModal" @click.self="showModal = false">
			<div class="modal-content card">
				<h2>{{ editingAsset ? "Edit Template" : "New Asset Template" }}</h2>
				<form @submit.prevent="handleSubmit" class="form-layout">
					<div class="form-group">
						<label for="name">Name</label>
						<input type="text" id="name" v-model="assetForm.name" required />
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
							<label for="identifier">System Identifier (Slug/TLD)</label>
							<input
								type="text"
								id="identifier"
								v-model="assetForm.identifier"
								placeholder="e.g. wp-rocket or .com"
							/>
						</div>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="price">Default Price (€)</label>
							<input
								type="number"
								id="price"
								step="0.01"
								:value="(assetForm.default_price / 100).toFixed(2)"
								@input="
									(e) =>
										(assetForm.default_price = Math.round(
											parseFloat((e.target as HTMLInputElement).value || '0') *
												100,
										))
								"
							/>
						</div>
						<div class="form-group">
							<label for="freq">Default Frequency</label>
							<select id="freq" v-model="assetForm.default_freq">
								<option v-for="freq in freqOptions" :key="freq" :value="freq">
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
							<input type="checkbox" v-model="assetForm.active" />
							Active (available for linking)
						</label>
					</div>

					<div class="form-actions">
						<button type="button" class="back-btn" @click="showModal = false">
							Cancel
						</button>
						<button type="submit" class="btn btn-primary">
							{{ editingAsset ? "Update Template" : "Create Template" }}
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>

<style scoped>
.loading-container {
	padding: 2rem;
	display: flex;
	justify-content: center;
}

.modal-overlay {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background-color: rgba(0, 0, 0, 0.5);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 1000;
	padding: 20px;
}

.modal-content {
	width: 100%;
	max-width: 500px;
	max-height: 90vh;
	overflow-y: auto;
}

.modal-content h2 {
	margin-top: 0;
	margin-bottom: 1.5rem;
	font-size: 1.5rem;
	border-bottom: 1px solid var(--border-color);
	padding-bottom: 0.75rem;
}

.form-layout {
	display: flex;
	flex-direction: column;
	gap: 1.25rem;
}

.form-row {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 1rem;
}

@media (max-width: 480px) {
	.form-row {
		grid-template-columns: 1fr;
	}
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 0.5rem;
}

.form-group label {
	font-weight: 600;
	font-size: 0.875rem;
	color: var(--text-muted);
}

.form-group input,
.form-group select,
.form-group textarea {
	padding: 0.625rem;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 0.875rem;
	width: 100%;
	background-color: var(--bg-card);
	color: var(--text-main);
}

.form-group textarea {
	resize: vertical;
	min-height: 80px;
	font-family: inherit;
}

.checkbox label {
	display: flex;
	align-items: center;
	gap: 0.5rem;
	cursor: pointer;
	font-size: 0.875rem;
}

.checkbox input {
	width: auto;
}

.form-actions {
	display: flex;
	justify-content: flex-end;
	gap: 1rem;
	margin-top: 0.5rem;
}

.status-badge.inactive {
	background-color: var(--badge-inactive-bg);
	color: var(--badge-inactive-text);
}

.status-badge.active {
	background-color: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.actions-cell {
	width: 80px;
}

.row-actions {
	display: flex;
	gap: 0.5rem;
	justify-content: flex-end;
}

.icon-btn-sm {
	background: none;
	border: none;
	padding: 4px;
	cursor: pointer;
	color: var(--text-muted);
	display: flex;
	align-items: center;
	justify-content: center;
	border-radius: 4px;
	transition:
		background-color 0.2s,
		color 0.2s;
}

.icon-btn-sm:hover {
	background-color: var(--bg-hover);
	color: var(--primary);
}

.icon-btn-sm.delete:hover {
	color: var(--error-text);
}

.asset-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
	gap: 1.5rem;
}

.asset-card {
	display: flex;
	flex-direction: column;
	padding: 0;
	transition:
		transform 0.2s,
		box-shadow 0.2s;
}

.asset-card:hover {
	transform: translateY(-2px);
	box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
}

.asset-card-header {
	padding: 1rem;
	display: flex;
	justify-content: space-between;
	align-items: center;
	border-bottom: 1px solid var(--border-color);
}

.asset-card-content {
	padding: 1.25rem;
	flex: 1;
}

.asset-card-content h3 {
	margin: 0 0 0.75rem 0;
	font-size: 1.125rem;
	color: var(--text-heading);
}

.asset-meta {
	display: flex;
	align-items: center;
	gap: 0.75rem;
	margin-bottom: 1rem;
}

.type-tag {
	font-size: 0.75rem;
	font-weight: 600;
	text-transform: uppercase;
	color: var(--text-muted);
	letter-spacing: 0.05em;
}

.identifier-code {
	font-size: 0.8125rem;
	background: var(--bg-body);
	padding: 0.125rem 0.375rem;
	border-radius: 4px;
	color: var(--primary);
}

.description {
	font-size: 0.875rem;
	color: var(--text-muted);
	margin: 0;
	display: -webkit-box;
	-webkit-line-clamp: 3;
	-webkit-box-orient: vertical;
	overflow: hidden;
}

.asset-card-footer {
	padding: 1rem 1.25rem;
	background-color: var(--bg-table-header);
	border-top: 1px solid var(--border-color);
	display: flex;
	justify-content: space-between;
	align-items: baseline;
	border-radius: 0 0 8px 8px;
}

.price {
	font-size: 1.25rem;
	font-weight: 700;
	color: var(--text-heading);
}

.freq {
	font-size: 0.8125rem;
	color: var(--text-muted);
	font-weight: 500;
}

code {
	background: var(--bg-muted);
	padding: 0.2rem 0.4rem;
	border-radius: 4px;
	font-size: 0.9em;
}
</style>
