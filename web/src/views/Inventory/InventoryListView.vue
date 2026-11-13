<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { useInventoryStore } from "../../stores/inventory";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type { InventoryItem, InventoryItemPayload } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import InventoryItemModal from "../../components/InventoryItemModal.vue";
import { useConfirm } from "../../composables/useConfirm";
import { displayDomain } from "../../utils/format";

const inventoryStore = useInventoryStore();
const authStore = useAuthStore();
const toast = useToastStore();
const router = useRouter();
const { confirm } = useConfirm();

const searchQuery = ref("");
const typeFilter = ref("");
const showModal = ref(false);
const editingItem = ref<InventoryItem | null>(null);

const typeTabs = computed(() => [
	{ value: "", label: "All" },
	...inventoryStore.types.map((t) => ({ value: t.type, label: t.type })),
]);

const load = async () => {
	await inventoryStore.fetchItems({
		type: typeFilter.value || undefined,
		search: searchQuery.value || undefined,
	});
};

// Typing waits for a pause before it searches; the store drops the response
// of any request a newer one has replaced.
let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch(typeFilter, () => {
	clearTimeout(searchTimer);
	load();
});
watch(searchQuery, () => {
	clearTimeout(searchTimer);
	searchTimer = setTimeout(load, 250);
});
onBeforeUnmount(() => clearTimeout(searchTimer));
onMounted(async () => {
	await Promise.all([
		inventoryStore.fetchTypes(),
		inventoryStore.fetchProviders(),
		load(),
	]);
});

const openAddModal = () => {
	editingItem.value = null;
	showModal.value = true;
};

const openEditModal = (item: InventoryItem) => {
	editingItem.value = item;
	showModal.value = true;
};

const handleSave = async (payload: InventoryItemPayload) => {
	if (editingItem.value) {
		const updated = await inventoryStore.updateItem(
			editingItem.value.id,
			payload,
		);
		if (!updated) {
			toast.addToast("Failed to update item", "error");
			return;
		}
		toast.addToast("Item updated", "success");
	} else {
		const res = await inventoryStore.createItem(payload);
		if (res.error) {
			toast.addToast(res.error, "error");
			return;
		}
		if (res.conflictItemId) {
			// An item with this type+name exists; it may be linkable there.
			showModal.value = false;
			toast.addToast(
				"An item with this type and name already exists — opened it so you can link or dismiss",
				"info",
			);
			router.push({
				name: "inventory-detail",
				params: { id: res.conflictItemId },
			});
			return;
		}
		toast.addToast("Item added", "success");
	}
	showModal.value = false;
	await load();
};

const handleDelete = async (item: InventoryItem) => {
	if (
		!(await confirm(`Delete inventory item "${item.name}"?`, {
			danger: true,
		}))
	)
		return;
	const err = await inventoryStore.deleteItem(item.id);
	if (err) {
		toast.addToast("Failed to delete: " + err, "error");
		return;
	}
	toast.addToast("Item deleted", "success");
	await load();
};

const syncAll = async () => {
	for (const provider of inventoryStore.providers) {
		const res = await inventoryStore.syncProvider(provider);
		if (!res) {
			toast.addToast(`Sync of ${provider} failed`, "error");
			continue;
		}
		if (res.skipped) {
			toast.addToast(
				`Sync of ${provider} skipped (not configured or empty)`,
				"info",
			);
		} else {
			toast.addToast(
				`Synced ${provider}: ${res.created} new, ${res.updated} updated, ${res.removed} removed`,
				"success",
			);
		}
	}
	await load();
};

const providerLabel = (item: InventoryItem) => {
	const links = item.links ?? [];
	if (links.length === 0) return "manual";
	return [...new Set(links.map((l) => l.provider))].join(", ");
};

// Domain names are shown in Unicode; the stored (Punycode) name is the
// cell's tooltip.
const displayName = (item: InventoryItem) =>
	item.type === "domain" ? displayDomain(item.name) : item.name;

const formatDate = (dateStr: string) => new Date(dateStr).toLocaleString();
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Inventory">
			<template #actions>
				<div class="flex-row gap-2 items-center">
					<button
						v-if="inventoryStore.providers.length > 0"
						class="btn btn-secondary"
						:disabled="!authStore.canEdit"
						@click="syncAll"
					>
						<AppIcon name="refresh" size="18" />
						Sync providers
					</button>
					<button
						v-if="authStore.canEdit"
						class="btn btn-primary"
						@click="openAddModal"
					>
						<AppIcon name="plus-circle" size="18" />
						Add item
					</button>
				</div>
			</template>
		</ViewHeader>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search by name or description..."
				class="search-input"
			/>
			<div class="tabs">
				<button
					v-for="tab in typeTabs"
					:key="tab.value"
					:class="['tab', { active: typeFilter === tab.value }]"
					@click="typeFilter = tab.value"
				>
					{{ tab.label }}
				</button>
			</div>
		</div>

		<main class="content">
			<div
				v-if="
					inventoryStore.isLoading &&
					inventoryStore.items.length === 0
				"
				class="loading-container"
			>
				<LoadingSpinner message="Loading inventory..." />
			</div>

			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Name</th>
							<th>Type</th>
							<th>Status</th>
							<th>Provider</th>
							<th>Tags</th>
							<th>Updated</th>
							<th v-if="authStore.canEdit"></th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="item in inventoryStore.items"
							:key="item.id"
							class="clickable-row"
							@click="
								router.push({
									name: 'inventory-detail',
									params: { id: item.id },
								})
							"
						>
							<td
								:title="
									displayName(item) !== item.name
										? item.name
										: undefined
								"
							>
								{{ displayName(item) }}
							</td>
							<td>
								<span class="status-badge info">{{
									item.type
								}}</span>
							</td>
							<td>
								<span
									:class="[
										'status-badge',
										item.status === 'active'
											? 'active'
											: 'inactive',
									]"
								>
									{{ item.status }}
								</span>
							</td>
							<td>{{ providerLabel(item) }}</td>
							<td>{{ (item.tags ?? []).join(", ") }}</td>
							<td>{{ formatDate(item.updated_at) }}</td>
							<td v-if="authStore.canEdit" @click.stop>
								<button
									class="icon-btn icon-btn-sm"
									title="Edit"
									@click="openEditModal(item)"
								>
									<AppIcon name="edit" size="16" />
								</button>
								<button
									class="icon-btn icon-btn-sm danger"
									title="Delete"
									@click="handleDelete(item)"
								>
									<AppIcon name="trash" size="16" />
								</button>
							</td>
						</tr>
						<tr v-if="inventoryStore.items.length === 0">
							<td
								:colspan="authStore.canEdit ? 7 : 6"
								class="empty-state"
							>
								No inventory items found.
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</main>

		<InventoryItemModal
			v-if="showModal"
			:item="editingItem"
			:types="inventoryStore.types"
			:initial-type="typeFilter || undefined"
			@save="handleSave"
			@close="showModal = false"
		/>
	</div>
</template>
