<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useInventoryStore } from "../../stores/inventory";
import { useDataStore } from "../../stores/data";
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
import { expiryBadge } from "../../utils/inventory";
import { displayDomain, formatBytes, providerName } from "../../utils/format";
import { useBackLink } from "../../composables/useBackLink";
import { usePageTitle } from "../../composables/usePageTitle";

const route = useRoute();
const router = useRouter();
const inventoryStore = useInventoryStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const item = ref<InventoryItemResponse | null>(null);
const showModal = ref(false);

const id = computed(() => String(route.params.id));

// Domain names are shown in Unicode in the title; everywhere else on the
// page shows the stored (Punycode) form.
const displayName = computed(() =>
	item.value?.type === "domain"
		? displayDomain(item.value.name)
		: (item.value?.name ?? ""),
);

// True when the item could not be loaded (deleted, unknown ID, API error).
const loadFailed = ref(false);

const load = async () => {
	const requested = id.value;
	const loaded = await inventoryStore.getItem(requested);
	// The route moved on to another item while this one loaded.
	if (requested !== id.value) return;
	loadFailed.value = loaded === null;
	if (loaded) item.value = loaded;
};

onMounted(async () => {
	await Promise.all([
		inventoryStore.fetchTypes(),
		dataStore.initData(),
		load(),
	]);
});

// The linked site and server, when the soft link points at one jman knows.
// A link to something no longer listed falls back to showing the ID.
const linkedSite = computed(() => dataStore.getSiteById(item.value?.site_id));
const linkedServer = computed(() =>
	dataStore.getServerById(item.value?.server_id),
);

// The component is reused when the route goes from one item to another.
watch(id, () => {
	item.value = null;
	loadFailed.value = false;
	showModal.value = false;
	load();
});

// formatValue renders one details/provider_data value for the key-value
// tables: byte counts (keys ending in _total, _used, _available or _bytes)
// as sizes, timestamps (keys ending in _at or _expires) as local dates,
// arrays as comma-separated lists and objects as "key: value" pairs.
const formatValue = (key: string, value: unknown): string => {
	if (value === null || value === undefined || value === "") return "—";
	if (
		typeof value === "number" &&
		/_(total|used|available|bytes)$/.test(key)
	) {
		return formatBytes(value);
	}
	if (typeof value === "string" && /_(at|expires)$/.test(key)) {
		const date = new Date(value);
		if (!Number.isNaN(date.getTime())) return date.toLocaleString();
	}
	if (typeof value === "boolean") return value ? "yes" : "no";
	if (Array.isArray(value)) {
		return value.map((v) => formatValue("", v)).join(", ") || "—";
	}
	if (typeof value === "object") {
		return (
			Object.entries(value as Record<string, unknown>)
				.map(([k, v]) => `${formatLabel(k)}: ${formatValue(k, v)}`)
				.join(", ") || "—"
		);
	}
	return String(value);
};

const formatLabel = (key: string) => key.replace(/_/g, " ");

const toEntries = (data: Record<string, unknown> | undefined) =>
	Object.entries(data ?? {}).map(([key, value]) => ({
		key,
		label: formatLabel(key),
		value: formatValue(key, value),
	}));

const detailEntries = computed(() => toEntries(item.value?.details));

// One section per provider the item has a snapshot from.
const providerSections = computed(() =>
	Object.entries(item.value?.provider_data ?? {})
		.map(([provider, data]) => ({
			provider,
			label: providerName(provider),
			entries: toEntries(data),
		}))
		.filter((s) => s.entries.length > 0)
		.sort((a, b) => a.label.localeCompare(b.label)),
);

// The link/dismiss prompt: for every undismissed match, its first link.
const matchPrompts = computed(() =>
	(item.value?.matches ?? [])
		.map((m) => ({ match: m, link: (m.links ?? [])[0] }))
		.filter((p) => p.link !== undefined),
);

// --- Registry lookup (domain items) ---

const lookup = computed(() =>
	// A lookup made under another name (before a rename) is not shown.
	item.value?.domain_lookup?.domain === item.value?.name
		? item.value?.domain_lookup
		: undefined,
);
const lookingUp = ref(false);

const handleLookup = async () => {
	if (!item.value) return;
	lookingUp.value = true;
	try {
		const result = await inventoryStore.lookupDomain(item.value.id);
		item.value = { ...item.value, domain_lookup: result };
		if (result.error) {
			toast.addToast("Lookup failed: " + result.error, "error");
		}
	} catch (e: any) {
		toast.addToast("Lookup failed: " + e.message, "error");
	} finally {
		lookingUp.value = false;
	}
};

const lookupExpiryBadge = computed(() => expiryBadge(lookup.value?.expires_at));

const sourceLabel = (source: string) =>
	({ rdap: "RDAP", whois: "WHOIS", "rdap+whois": "RDAP and WHOIS" })[
		source
	] ?? source;

const formatDay = (dateStr: string) => new Date(dateStr).toLocaleDateString();

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

// After a delete, back to the list as it was left (or the plain list).
const { go: backToList } = useBackLink({ name: "inventory" });

usePageTitle(() => (item.value ? displayName.value : null));

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
	backToList();
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
			:title="item ? displayName : 'Inventory item'"
			:back="{ label: 'Inventory', to: { name: 'inventory' } }"
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
			<div v-if="!item && loadFailed" class="card">
				<p class="form-error">
					This inventory item could not be loaded. It may have been
					deleted.
				</p>
				<div class="flex-row gap-2">
					<button class="btn btn-outline" @click="load">
						Try again
					</button>
					<button
						class="btn btn-primary"
						@click="router.push({ name: 'inventory' })"
					>
						Back to inventory
					</button>
				</div>
			</div>
			<div v-else-if="!item" class="loading-container">
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

				<div class="detail-layout detail-aside-first">
					<aside class="detail-aside">
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
												<span
													class="status-badge info"
													>{{ item.type }}</span
												>
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
											<td>
												{{ item.description || "—" }}
											</td>
										</tr>
										<tr>
											<th>Tags</th>
											<td>
												{{
													(item.tags ?? []).join(
														", ",
													) || "—"
												}}
											</td>
										</tr>
										<tr>
											<th>Linked site</th>
											<td>
												<RouterLink
													v-if="linkedSite"
													:to="{
														name: 'site-detail',
														params: {
															id: linkedSite.id,
														},
													}"
												>
													{{ linkedSite.domain }}
												</RouterLink>
												<span v-else>{{
													item.site_id || "—"
												}}</span>
											</td>
										</tr>
										<tr>
											<th>Linked server</th>
											<td>
												{{
													linkedServer?.name ||
													item.server_id ||
													"—"
												}}
											</td>
										</tr>
										<tr>
											<th>Updated</th>
											<td>
												{{
													formatDate(item.updated_at)
												}}
												by
												{{ item.updated_by }}
											</td>
										</tr>
									</tbody>
								</table>
							</div>
						</div>
						<NotesWidget
							parent-type="InventoryItem"
							:parent-id="item.id"
						/>
					</aside>
					<div class="detail-main">
						<div v-if="item.type === 'domain'" class="card">
							<div class="card-header">
								<h3>Registration</h3>
								<button
									v-if="authStore.canEdit"
									class="btn btn-outline btn-sm"
									:disabled="lookingUp"
									@click="handleLookup"
								>
									<AppIcon name="refresh" size="16" />
									{{
										lookingUp
											? "Looking up..."
											: "Look up now"
									}}
								</button>
							</div>
							<p v-if="!lookup" class="text-muted">
								Not looked up yet. Domains are looked up at
								their registry once a day.
							</p>
							<template v-else>
								<p v-if="lookup.error" class="form-error">
									Last lookup failed: {{ lookup.error }}
									<template v-if="lookup.last_success_at">
										— showing the result from
										{{
											formatDate(lookup.last_success_at)
										}}.
									</template>
								</p>
								<div
									v-if="lookup.last_success_at"
									class="table-container"
								>
									<table class="data-table">
										<tbody>
											<tr>
												<th>Registrar</th>
												<td>
													{{
														lookup.registrar ||
														"Not published by the registry"
													}}
												</td>
											</tr>
											<tr>
												<th>Expires</th>
												<td>
													<template
														v-if="lookup.expires_at"
													>
														{{
															formatDay(
																lookup.expires_at,
															)
														}}
														<span
															v-if="
																lookupExpiryBadge
															"
															:class="[
																'status-badge',
																'badge-sm',
																lookupExpiryBadge.cls,
															]"
														>
															{{
																lookupExpiryBadge.text
															}}
														</span>
													</template>
													<template v-else>
														Not published by the
														registry
													</template>
												</td>
											</tr>
											<tr>
												<th>Nameservers</th>
												<td>
													{{
														lookup.nameservers.join(
															", ",
														) ||
														"Not published by the registry"
													}}
												</td>
											</tr>
										</tbody>
									</table>
								</div>
								<p class="text-muted">
									Checked {{ formatDate(lookup.checked_at) }}
									<template v-if="lookup.source">
										via
										{{
											sourceLabel(lookup.source)
										}}</template
									>.
								</p>
							</template>
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
										<tr
											v-for="link in item.links"
											:key="link.id"
										>
											<td>
												{{
													providerName(link.provider)
												}}
											</td>
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
							<div
								v-if="detailEntries.length === 0"
								class="text-muted"
							>
								No details yet.
							</div>
							<div v-else class="table-container">
								<table class="data-table">
									<tbody>
										<tr
											v-for="entry in detailEntries"
											:key="entry.key"
										>
											<th>{{ entry.label }}</th>
											<td>{{ entry.value }}</td>
										</tr>
									</tbody>
								</table>
							</div>
						</div>

						<div
							v-for="section in providerSections"
							:key="section.provider"
							class="card"
						>
							<div class="card-header">
								<h3>{{ section.label }} data (read-only)</h3>
							</div>
							<div class="table-container">
								<table class="data-table">
									<tbody>
										<tr
											v-for="entry in section.entries"
											:key="entry.key"
										>
											<th>{{ entry.label }}</th>
											<td>{{ entry.value }}</td>
										</tr>
									</tbody>
								</table>
							</div>
						</div>
					</div>
				</div>
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
