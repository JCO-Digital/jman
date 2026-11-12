<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useInventoryStore } from "../../stores/inventory";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type {
	InventoryItemPayload,
	InventoryItemResponse,
	InventoryLinkPayload,
} from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import InventoryItemModal from "../../components/InventoryItemModal.vue";
import NotesWidget from "../../components/NotesWidget.vue";
import { useConfirm } from "../../composables/useConfirm";

const route = useRoute();
const router = useRouter();
const inventoryStore = useInventoryStore();
const authStore = useAuthStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const item = ref<InventoryItemResponse | null>(null);
const showModal = ref(false);

const id = computed(() => String(route.params.id));

const load = async () => {
	item.value = await inventoryStore.getItem(id.value);
};

onMounted(async () => {
	await inventoryStore.fetchTypes();
	await load();
});

const detailEntries = computed(() =>
	Object.entries(item.value?.details ?? {}).map(([key, value]) => ({
		key,
		value:
			typeof value === "object" ? JSON.stringify(value) : String(value),
	})),
);

const providerDataEntries = computed(() =>
	Object.entries(item.value?.provider_data ?? {}).map(([key, value]) => ({
		key,
		value:
			typeof value === "object" ? JSON.stringify(value) : String(value),
	})),
);

// The link/dismiss prompt: for every undismissed match, its first link.
const matchPrompts = computed(() =>
	(item.value?.matches ?? [])
		.map((m) => ({ match: m, link: (m.links ?? [])[0] }))
		.filter((p) => p.link !== undefined),
);

const handleLink = async (link: InventoryLinkPayload) => {
	if (!item.value) return;
	const updated = await inventoryStore.linkItem(item.value.id, link);
	if (!updated) {
		toast.addToast("Failed to link", "error");
		return;
	}
	toast.addToast("Linked to provider entity", "success");
	await load();
};

const handleDismiss = async (link: InventoryLinkPayload) => {
	if (!item.value) return;
	await inventoryStore.dismissMatch(item.value.id, link);
	await load();
};

const openEdit = () => {
	showModal.value = true;
};

const handleSave = async (payload: InventoryItemPayload) => {
	if (!item.value) return;
	const updated = await inventoryStore.updateItem(item.value.id, payload);
	if (!updated) {
		toast.addToast("Failed to update item", "error");
		return;
	}
	showModal.value = false;
	toast.addToast("Item updated", "success");
	await load();
};

const handleDelete = async () => {
	if (!item.value) return;
	if (
		!(await confirm(`Delete inventory item "${item.value.name}"?`, {
			danger: true,
		}))
	)
		return;
	const err = await inventoryStore.deleteItem(item.value.id);
	if (err) {
		toast.addToast("Failed to delete: " + err, "error");
		return;
	}
	router.push({ name: "inventory" });
};

const handleUnlink = async (link: InventoryLinkPayload) => {
	if (!item.value) return;
	const updated = await inventoryStore.unlinkItem(item.value.id, link);
	if (!updated) {
		toast.addToast("Failed to unlink", "error");
		return;
	}
	toast.addToast("Unlinked", "success");
	await load();
};

const formatDate = (dateStr: string) => new Date(dateStr).toLocaleString();
</script>

<template>
	<div class="view-container">
		<ViewHeader
			:title="item ? item.name : 'Inventory item'"
			:back-button="{
				text: 'Inventory',
				onClick: () => router.push({ name: 'inventory' }),
			}"
		>
			<template #actions>
				<div
					v-if="item && authStore.canEdit"
					class="flex-row gap-2 items-center"
				>
					<button class="btn btn-outline" @click="openEdit">
						<AppIcon name="edit" size="18" />
						Edit
					</button>
					<button class="btn btn-danger" @click="handleDelete">
						<AppIcon name="trash" size="18" />
						Delete
					</button>
				</div>
			</template>
		</ViewHeader>

		<main class="content">
			<div v-if="!item" class="loading-container">
				<LoadingSpinner message="Loading item..." />
			</div>

			<template v-else>
				<div
					v-for="p in matchPrompts"
					:key="p.link!.provider + p.link!.provider_id"
					class="card"
				>
					<div class="card-header">
						<h3>Possible duplicate</h3>
					</div>
					<p>
						This item matches the provider-linked item
						<strong>{{ p.match.name }}</strong> ({{
							p.link!.provider
						}}/{{ p.link!.provider_type }}/{{
							p.link!.provider_id
						}}).
					</p>
					<div class="flex-row gap-2">
						<button
							v-if="authStore.canEdit"
							class="btn btn-primary"
							@click="handleLink(p.link!)"
						>
							Link
						</button>
						<button
							v-if="authStore.canEdit"
							class="btn btn-outline"
							@click="handleDismiss(p.link!)"
						>
							Dismiss
						</button>
					</div>
				</div>

				<div class="card">
					<div class="card-header">
						<h3>Item</h3>
					</div>
					<div class="table-container">
						<table class="data-table">
							<tbody>
								<tr>
									<th>Type</th>
									<td>
										<span class="status-badge info">{{
											item.type
										}}</span>
									</td>
								</tr>
								<tr>
									<th>Status</th>
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
								</tr>
								<tr>
									<th>Origin</th>
									<td>{{ item.origin }}</td>
								</tr>
								<tr>
									<th>Description</th>
									<td>{{ item.description || "—" }}</td>
								</tr>
								<tr>
									<th>Tags</th>
									<td>
										{{
											(item.tags ?? []).join(", ") || "—"
										}}
									</td>
								</tr>
								<tr>
									<th>Linked site</th>
									<td>{{ item.site_id || "—" }}</td>
								</tr>
								<tr>
									<th>Linked server</th>
									<td>{{ item.server_id || "—" }}</td>
								</tr>
								<tr>
									<th>Updated</th>
									<td>
										{{ formatDate(item.updated_at) }} by
										{{ item.updated_by }}
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>

				<div class="card">
					<div class="card-header">
						<h3>Provider links</h3>
					</div>
					<p
						v-if="(item.links ?? []).length === 0"
						class="text-muted"
					>
						Manual item — not linked to any provider.
					</p>
					<div v-else class="table-container">
						<table class="data-table">
							<thead>
								<tr>
									<th>Provider</th>
									<th>Type</th>
									<th>Provider ID</th>
									<th v-if="authStore.canEdit"></th>
								</tr>
							</thead>
							<tbody>
								<tr v-for="link in item.links" :key="link.id">
									<td>{{ link.provider }}</td>
									<td>{{ link.provider_type }}</td>
									<td>{{ link.provider_id }}</td>
									<td v-if="authStore.canEdit">
										<button
											class="btn btn-outline btn-sm"
											@click="handleUnlink(link)"
										>
											Unlink
										</button>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>

				<div class="card">
					<div class="card-header">
						<h3>Details (manual)</h3>
					</div>
					<div v-if="detailEntries.length === 0" class="text-muted">
						No details yet.
					</div>
					<div v-else class="table-container">
						<table class="data-table">
							<tbody>
								<tr
									v-for="entry in detailEntries"
									:key="entry.key"
								>
									<th>{{ entry.key }}</th>
									<td>{{ entry.value }}</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>

				<div v-if="providerDataEntries.length > 0" class="card">
					<div class="card-header">
						<h3>Provider data (read-only)</h3>
					</div>
					<div class="table-container">
						<table class="data-table">
							<tbody>
								<tr
									v-for="entry in providerDataEntries"
									:key="entry.key"
								>
									<th>{{ entry.key }}</th>
									<td>{{ entry.value }}</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>

				<NotesWidget parent-type="InventoryItem" :parent-id="item.id" />
			</template>
		</main>

		<InventoryItemModal
			v-if="showModal && item"
			:item="item"
			:types="inventoryStore.types"
			@save="handleSave"
			@close="showModal = false"
		/>
	</div>
</template>
