<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { RouterLink, useRouter, type RouteLocationNamedRaw } from "vue-router";
import { useInventoryStore } from "../../stores/inventory";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type {
	InventoryImportResult,
	InventoryItem,
	InventoryItemPayload,
} from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import ListToolbar from "../../components/ListToolbar.vue";
import SortableTh from "../../components/SortableTh.vue";
import Pagination from "../../components/Pagination.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import InventoryItemModal from "../../components/InventoryItemModal.vue";
import InventoryImportModal from "../../components/InventoryImportModal.vue";
import { useConfirm } from "../../composables/useConfirm";
import { useListState } from "../../composables/useListState";
import { useClientList } from "../../composables/useClientList";
import { displayDomain, providerName } from "../../utils/format";
import {
	EXPIRY_WARN_DAYS,
	expiryBadge,
	itemExpiry,
	matchesExpiryFilter,
	serverSummary,
} from "../../utils/inventory";

// The type is in the path, the rest of the list's state in the query, so
// browser back and the detail view's back button return to the same list:
// /inventory[/<type>][?q=&sort=&dir=&page=&per=&expiring=soon|expired]
const props = defineProps<{
	type?: string;
}>();

const inventoryStore = useInventoryStore();
const authStore = useAuthStore();
const toast = useToastStore();
const router = useRouter();
const { confirm } = useConfirm();

type SortKey = "name" | "type" | "status" | "info" | "provider";

const list = useListState<SortKey, { expiring: readonly string[] }>({
	sortKeys: ["name", "type", "status", "info", "provider"],
	defaultSort: "name",
	filters: { expiring: ["soon", "expired"] },
	// The expiring domains are most useful soonest first.
	impliedSort: (f) => (f.expiring ? { sort: "info", dir: "asc" } : null),
});

const typeFilter = computed(() => props.type ?? "");
const expiring = computed(
	() => list.filters.value.expiring as "" | "soon" | "expired",
);

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

const setType = (event: Event) => {
	const type = (event.target as HTMLSelectElement).value;
	const to = list.location({ page: 1 }) as RouteLocationNamedRaw;
	router.push({ ...to, params: { type } });
};

const load = async () => {
	await inventoryStore.fetchItems({
		type: typeFilter.value || undefined,
		search: list.query.value || undefined,
	});
};

// The search runs on the server.
watch([typeFilter, list.query], load);
onMounted(async () => {
	await Promise.all([
		inventoryStore.fetchTypes(),
		inventoryStore.fetchProviders(),
		load(),
	]);
});

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
		case "info": {
			// Domains by expiry date (unknown ones after them), then servers
			// by their summary.
			if (item.type === "domain") {
				const expiry = itemExpiry(item);
				return expiry ? "0" + expiry : "1";
			}
			return "2" + serverSummary(item).toLowerCase();
		}
		default:
			return String(item[key]);
	}
};

// Items that sort the same stay in name order.
const itemsByName = computed(() =>
	inventoryStore.items
		.filter((item) => matchesExpiryFilter(item, expiring.value))
		.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0)),
);

const items = useClientList(itemsByName, list, { sortValue });

const formatDay = (date: string) =>
	new Date(date).toLocaleDateString("de-DE", { dateStyle: "short" });

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

		<ListToolbar :list="list" placeholder="Search by name or description">
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
			<select
				:value="expiring"
				aria-label="Expiry"
				@change="
					list.setFilter(
						'expiring',
						($event.target as HTMLSelectElement).value,
					)
				"
			>
				<option value="">Any expiry</option>
				<option value="soon">
					Expiring within {{ EXPIRY_WARN_DAYS }} days
				</option>
				<option value="expired">Expired</option>
			</select>
		</ListToolbar>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<SortableTh
							:list="list"
							sort-key="name"
							class="col-expand"
						>
							Name
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="type"
							class="col-narrow"
						>
							Type
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="status"
							class="col-narrow"
						>
							Status
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="info"
							class="col-wide"
						>
							Info
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="provider"
							class="col-medium"
						>
							Provider
						</SortableTh>
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
						<td :colspan="authStore.canEdit ? 7 : 6">
							<LoadingSpinner message="Loading inventory..." />
						</td>
					</tr>
					<tr v-else-if="items.pageItems.value.length === 0">
						<td
							:colspan="authStore.canEdit ? 7 : 6"
							class="empty-state"
						>
							<span v-if="list.query.value"
								>No inventory items found matching "{{
									list.query.value
								}}".</span
							>
							<span v-else-if="expiring === 'expired'"
								>No expired domains.</span
							>
							<span v-else-if="expiring"
								>No domains expire within
								{{ EXPIRY_WARN_DAYS }} days.</span
							>
							<span v-else>No inventory items found.</span>
						</td>
					</tr>
					<tr
						v-for="item in items.pageItems.value"
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
							<RouterLink
								:to="{
									name: 'inventory-detail',
									params: { id: item.id },
								}"
								class="row-link"
								@click.stop
							>
								{{ displayName(item) }}
							</RouterLink>
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
						<td class="col-wide truncate">
							<template v-if="item.type === 'domain'">
								<template v-if="itemExpiry(item)">
									Expires
									{{ formatDay(itemExpiry(item)!) }}
									<span
										v-if="expiryBadge(itemExpiry(item))"
										:class="[
											'status-badge',
											'badge-sm',
											expiryBadge(itemExpiry(item))!.cls,
										]"
									>
										{{
											expiryBadge(itemExpiry(item))!.text
										}}
									</span>
								</template>
								<span v-else class="text-muted"
									>Expiry unknown</span
								>
								<div
									v-if="item.domain_lookup?.registrar"
									class="text-muted font-xs truncate"
									:title="item.domain_lookup.registrar"
								>
									{{ item.domain_lookup.registrar }}
								</div>
							</template>
							<template v-else>{{
								serverSummary(item)
							}}</template>
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
				:list="list"
				:page="items.page.value"
				:total="items.total.value"
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
