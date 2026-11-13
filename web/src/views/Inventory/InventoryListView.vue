<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter, type RouteLocationRaw } from "vue-router";
import { useInventoryStore } from "../../stores/inventory";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type {
	InventoryImportResult,
	InventoryItem,
	InventoryItemPayload,
} from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import Pagination from "../../components/Pagination.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import InventoryItemModal from "../../components/InventoryItemModal.vue";
import InventoryImportModal from "../../components/InventoryImportModal.vue";
import { useConfirm } from "../../composables/useConfirm";
import { displayDomain, providerName } from "../../utils/format";

// The list's state lives in the URL, so browser back and the detail view's
// back button return to the same type, page and search:
// /inventory[/<type|all>[/<page>[/<rowsPerPage>]]][?search=...]
const props = defineProps<{
	type?: string;
	page?: number;
	rowsPerPage?: number;
}>();

const DEFAULT_ROWS = 50;

const inventoryStore = useInventoryStore();
const authStore = useAuthStore();
const toast = useToastStore();
const route = useRoute();
const router = useRouter();
const { confirm } = useConfirm();

const typeFilter = computed(() =>
	props.type && props.type !== "all" ? props.type : "",
);
const currentPage = computed(() => props.page || 1);
const rowsPerPage = computed(() => props.rowsPerPage || DEFAULT_ROWS);
const routeSearch = computed(() =>
	typeof route.query.search === "string" ? route.query.search : "",
);

const searchQuery = ref(routeSearch.value);
const showModal = ref(false);
const editingItem = ref<InventoryItem | null>(null);

const TYPE_LABELS: Record<string, string> = {
	domain: "Domains",
	server: "Servers",
};
const typeOptions = computed(() => [
	{ value: "", label: "All types" },
	...inventoryStore.types.map((t) => ({
		value: t.type,
		label: TYPE_LABELS[t.type] ?? t.type,
	})),
]);

// listLocation is the URL of one state of the list. The defaults are left
// out, so the first page of all items is plain /inventory.
const listLocation = (opts: {
	type?: string;
	page?: number;
	rowsPerPage?: number;
	search?: string;
}): RouteLocationRaw => {
	const type = opts.type ?? typeFilter.value;
	const page = opts.page ?? 1;
	const rpp = opts.rowsPerPage ?? rowsPerPage.value;
	const search = opts.search ?? searchQuery.value;
	const params: Record<string, string> = {};
	if (page !== 1 || rpp !== DEFAULT_ROWS) {
		params.type = type || "all";
		params.page = String(page);
		params.rowsPerPage = String(rpp);
	} else if (type) {
		params.type = type;
	}
	return {
		name: "inventory",
		params,
		query: search ? { search } : {},
	};
};

const setType = (event: Event) => {
	router.push(
		listLocation({ type: (event.target as HTMLSelectElement).value }),
	);
};

const load = async () => {
	await inventoryStore.fetchItems({
		type: typeFilter.value || undefined,
		search: routeSearch.value || undefined,
	});
};

// Typing waits for a pause, then replaces the URL (no history entry per
// keystroke) and goes back to the first page; the URL change loads.
let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch(searchQuery, (value) => {
	clearTimeout(searchTimer);
	if (value === routeSearch.value) return;
	searchTimer = setTimeout(
		() => router.replace(listLocation({ search: value })),
		250,
	);
});
// Back/forward to a URL with another search shows it in the box.
watch(routeSearch, (value) => {
	if (value !== searchQuery.value) searchQuery.value = value;
});
watch([typeFilter, routeSearch], load);
onBeforeUnmount(() => clearTimeout(searchTimer));
onMounted(async () => {
	await Promise.all([
		inventoryStore.fetchTypes(),
		inventoryStore.fetchProviders(),
		load(),
	]);
});

type SortKey = "name" | "type" | "status" | "provider";
const sortKey = ref<SortKey>("name");
const sortOrder = ref<"asc" | "desc">("asc");

const handleSort = (key: SortKey) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
};

const sortIndicator = (key: SortKey) =>
	sortKey.value === key ? (sortOrder.value === "asc" ? "↑" : "↓") : "";

const providerLabel = (item: InventoryItem) => {
	const links = item.links ?? [];
	if (links.length === 0) return "manual";
	return [...new Set(links.map((l) => providerName(l.provider)))].join(", ");
};

// Domain names are shown in Unicode; the stored (Punycode) name is the
// cell's tooltip.
const displayName = (item: InventoryItem) =>
	item.type === "domain" ? displayDomain(item.name) : item.name;

const sortValue = (item: InventoryItem, key: SortKey): string => {
	switch (key) {
		case "name":
			return displayName(item).toLowerCase();
		case "provider":
			return providerLabel(item).toLowerCase();
		default:
			return String(item[key]);
	}
};

const sortedItems = computed(() => {
	const dir = sortOrder.value === "asc" ? 1 : -1;
	return [...inventoryStore.items].sort((a, b) => {
		const va = sortValue(a, sortKey.value);
		const vb = sortValue(b, sortKey.value);
		if (va !== vb) return va < vb ? -dir : dir;
		return a.name < b.name ? -1 : 1;
	});
});

const totalPages = computed(
	() => Math.ceil(sortedItems.value.length / rowsPerPage.value) || 1,
);
// A page past the end (after a search narrowed the list) shows the last.
const shownPage = computed(() => Math.min(currentPage.value, totalPages.value));
const pageItems = computed(() => {
	const start = (shownPage.value - 1) * rowsPerPage.value;
	return sortedItems.value.slice(start, start + rowsPerPage.value);
});

const prevPage = () => {
	if (shownPage.value > 1) {
		router.push(listLocation({ page: shownPage.value - 1 }));
	}
};
const nextPage = () => {
	if (shownPage.value < totalPages.value) {
		router.push(listLocation({ page: shownPage.value + 1 }));
	}
};
const handleRowsPerPageUpdate = (rpp: number) => {
	router.push(listLocation({ rowsPerPage: rpp }));
};

const goToItem = (id: string) => {
	router.push({ name: "inventory-detail", params: { id } });
};

const showImport = ref(false);

const handleImported = async (res: InventoryImportResult) => {
	showImport.value = false;
	toast.addToast(
		`Imported domains: ${res.created} new, ${res.updated} updated`,
		"success",
	);
	await load();
};

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
						class="btn btn-outline"
						@click="showImport = true"
					>
						<AppIcon name="upload" size="18" />
						Import domains
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
			<select
				:value="typeFilter"
				aria-label="Item type"
				@change="setType"
			>
				<option
					v-for="t in typeOptions"
					:key="t.value"
					:value="t.value"
				>
					{{ t.label }}
				</option>
			</select>
		</div>

		<main class="table-container">
			<table class="data-table sortable">
				<thead>
					<tr>
						<th class="col-expand" @click="handleSort('name')">
							Name {{ sortIndicator("name") }}
						</th>
						<th class="col-narrow" @click="handleSort('type')">
							Type {{ sortIndicator("type") }}
						</th>
						<th class="col-narrow" @click="handleSort('status')">
							Status {{ sortIndicator("status") }}
						</th>
						<th class="col-medium" @click="handleSort('provider')">
							Provider {{ sortIndicator("provider") }}
						</th>
						<th class="col-medium hide-mobile">Tags</th>
						<th v-if="authStore.canEdit" class="col-narrow"></th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							inventoryStore.isLoading &&
							inventoryStore.items.length === 0
						"
					>
						<td :colspan="authStore.canEdit ? 6 : 5">
							<LoadingSpinner message="Loading inventory..." />
						</td>
					</tr>
					<tr v-else-if="pageItems.length === 0">
						<td
							:colspan="authStore.canEdit ? 6 : 5"
							class="empty-state"
						>
							<span v-if="routeSearch"
								>No inventory items found matching "{{
									routeSearch
								}}".</span
							>
							<span v-else>No inventory items found.</span>
						</td>
					</tr>
					<tr
						v-for="item in pageItems"
						:key="item.id"
						class="clickable-row"
						@click="goToItem(item.id)"
					>
						<td
							class="font-medium truncate col-expand"
							:title="
								displayName(item) !== item.name
									? item.name
									: undefined
							"
						>
							{{ displayName(item) }}
						</td>
						<td class="col-narrow">
							<span class="status-badge badge-sm info">{{
								item.type
							}}</span>
						</td>
						<td class="col-narrow">
							<span
								:class="[
									'status-badge',
									'badge-sm',
									item.status === 'active'
										? 'active'
										: 'inactive',
								]"
							>
								{{ item.status }}
							</span>
						</td>
						<td class="col-medium truncate">
							{{ providerLabel(item) }}
						</td>
						<td class="col-medium truncate hide-mobile">
							{{ (item.tags ?? []).join(", ") }}
						</td>
						<td
							v-if="authStore.canEdit"
							class="col-narrow"
							@click.stop
						>
							<div class="flex-row gap-2">
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
							</div>
						</td>
					</tr>
				</tbody>
			</table>
			<Pagination
				:current-page="shownPage"
				:total-pages="totalPages"
				:rows-per-page="rowsPerPage"
				@update:rows-per-page="handleRowsPerPageUpdate"
				@prev="prevPage"
				@next="nextPage"
			/>
		</main>

		<InventoryImportModal
			v-if="showImport"
			@imported="handleImported"
			@close="showImport = false"
		/>
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
