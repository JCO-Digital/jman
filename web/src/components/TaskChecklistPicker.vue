<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { apiFetch, BASE_URL, handleErrorResponse } from "../utils/api";
import type {
	InventoryItem,
	NewTaskChecklistItem,
	TaskChecklistRefType,
} from "../types";
import AppIcon from "./AppIcon.vue";
import SearchableSelect from "./SearchableSelect.vue";

// Picks sites, servers, plugins or inventory items to add to a checklist,
// one item each. Sites can be narrowed to a server, an organization or the
// sites running a plugin, and every site shown added at once.

const props = defineProps<{
	/** "type:id" of the records already on the checklist. */
	existing: Set<string>;
}>();

const emit = defineEmits<{
	(e: "add", items: NewTaskChecklistItem[]): void;
	(e: "close"): void;
}>();

const dataStore = useDataStore();
const orgStore = useOrganizationStore();

type Tab = "site" | "server" | "plugin" | "inventory_item";
const tabs: { key: Tab; label: string; icon: string }[] = [
	{ key: "site", label: "Sites", icon: "site" },
	{ key: "server", label: "Servers", icon: "box" },
	{ key: "plugin", label: "Plugins", icon: "plugin" },
	{ key: "inventory_item", label: "Inventory", icon: "tag" },
];
const tab = ref<Tab>("site");
const search = ref("");

// Site filters.
const serverFilter = ref<string>("");
const orgFilter = ref<number | "">("");
const pluginFilter = ref<string>("");

// Inventory items are read here rather than through the inventory store,
// whose list belongs to the inventory page.
const inventory = ref<InventoryItem[]>([]);
const inventoryLoading = ref(false);
const inventoryError = ref<string | null>(null);
const inventoryType = ref("");

async function loadInventory() {
	if (inventory.value.length || inventoryLoading.value) return;
	inventoryLoading.value = true;
	inventoryError.value = null;
	try {
		const res = await apiFetch(`${BASE_URL}/inventory/items?status=active`);
		if (!res.ok) await handleErrorResponse(res);
		inventory.value = await res.json();
	} catch (e: any) {
		inventoryError.value = e.message;
	} finally {
		inventoryLoading.value = false;
	}
}

onMounted(() => {
	if (!dataStore.isLoaded) dataStore.initData();
	if (orgStore.organizations.length === 0) orgStore.fetchOrganizations();
});

watch(tab, (t) => {
	selected.value = new Set();
	search.value = "";
	if (t === "inventory_item") loadInventory();
});

interface Row {
	id: string;
	text: string;
	detail: string;
}

const orgName = computed(() => {
	const m = new Map<number, string>();
	for (const o of orgStore.organizations) m.set(o.id, o.name);
	return m;
});

const allRows = computed<Row[]>(() => {
	switch (tab.value) {
		case "site": {
			let sites = dataStore.sites;
			if (serverFilter.value)
				sites = sites.filter((s) => s.server_id === serverFilter.value);
			if (orgFilter.value !== "")
				sites = sites.filter(
					(s) => s.organization_id === orgFilter.value,
				);
			if (pluginFilter.value) {
				const on = new Set(
					(
						dataStore.pluginsBySlugMap.get(pluginFilter.value) ?? []
					).map((p) => p.site_id),
				);
				sites = sites.filter((s) => on.has(s.id));
			}
			return sites
				.map((s) => ({
					id: s.id,
					text: s.domain,
					detail: [
						s.server_name,
						s.organization_id != null
							? orgName.value.get(s.organization_id)
							: "",
					]
						.filter(Boolean)
						.join(" · "),
				}))
				.sort((a, b) => a.text.localeCompare(b.text));
		}
		case "server":
			return dataStore.servers
				.map((s) => {
					const n = dataStore.sites.filter(
						(site) => site.server_id === s.id,
					).length;
					return {
						id: s.id,
						text: s.name,
						detail: `${n} ${n === 1 ? "site" : "sites"}`,
					};
				})
				.sort((a, b) => a.text.localeCompare(b.text));
		case "plugin":
			return dataStore.enrichedPlugins
				.filter((p) => p.count > 0)
				.map((p) => ({
					id: p.slug,
					text: p.name,
					detail: `${p.slug} · ${p.count} ${p.count === 1 ? "site" : "sites"}`,
				}))
				.sort((a, b) => a.text.localeCompare(b.text));
		case "inventory_item":
			return inventory.value
				.filter(
					(i) =>
						!inventoryType.value || i.type === inventoryType.value,
				)
				.map((i) => ({
					id: i.id,
					text: i.name,
					detail: [i.type, ...(i.tags ?? [])].join(" · "),
				}))
				.sort((a, b) => a.text.localeCompare(b.text));
	}
	return [];
});

const rows = computed(() => {
	const q = search.value.trim().toLowerCase();
	if (!q) return allRows.value;
	return allRows.value.filter(
		(r) =>
			r.text.toLowerCase().includes(q) ||
			r.detail.toLowerCase().includes(q),
	);
});

const inList = (r: Row) => props.existing.has(`${tab.value}:${r.id}`);

const selected = ref(new Set<string>());
const selectable = computed(() => rows.value.filter((r) => !inList(r)));
const allSelected = computed(
	() =>
		selectable.value.length > 0 &&
		selectable.value.every((r) => selected.value.has(r.id)),
);

function toggle(r: Row) {
	const next = new Set(selected.value);
	if (next.has(r.id)) next.delete(r.id);
	else next.add(r.id);
	selected.value = next;
}

function toggleAll() {
	const next = new Set(selected.value);
	if (allSelected.value) {
		for (const r of selectable.value) next.delete(r.id);
	} else {
		for (const r of selectable.value) next.add(r.id);
	}
	selected.value = next;
}

function add() {
	const refType: TaskChecklistRefType = tab.value;
	// In the order shown.
	const items = allRows.value
		.filter((r) => selected.value.has(r.id))
		.map((r) => ({ text: r.text, ref_type: refType, ref_id: r.id }));
	if (!items.length) return;
	emit("add", items);
	selected.value = new Set();
}

const serverOptions = computed(() => [
	{ value: "", label: "Any server" },
	...dataStore.servers
		.map((s) => ({ value: s.id, label: s.name }))
		.sort((a, b) => a.label.localeCompare(b.label)),
]);
const orgOptions = computed(() => [
	{ value: "", label: "Any organization" },
	...orgStore.organizations
		.map((o) => ({ value: o.id, label: o.name }))
		.sort((a, b) => a.label.localeCompare(b.label)),
]);
const pluginOptions = computed(() => [
	{ value: "", label: "Any plugin" },
	...dataStore.enrichedPlugins
		.filter((p) => p.count > 0)
		.map((p) => ({ value: p.slug, label: `${p.name} (${p.count})` }))
		.sort((a, b) => a.label.localeCompare(b.label)),
]);
const inventoryTypes = computed(() =>
	[...new Set(inventory.value.map((i) => i.type))].sort(),
);

const loading = computed(() =>
	tab.value === "inventory_item"
		? inventoryLoading.value
		: !dataStore.isLoaded && dataStore.isLoading,
);
</script>

<template>
	<div class="picker" @keydown.esc.stop.prevent="emit('close')">
		<header class="picker-head">
			<div class="picker-tabs" role="tablist">
				<button
					v-for="t in tabs"
					:key="t.key"
					type="button"
					role="tab"
					:aria-selected="tab === t.key"
					:class="{ active: tab === t.key }"
					@click="tab = t.key"
				>
					<AppIcon :name="t.icon" size="14" />
					{{ t.label }}
				</button>
			</div>
			<button
				type="button"
				class="icon-btn icon-btn-sm"
				title="Close"
				@click="emit('close')"
			>
				<AppIcon name="x" size="16" />
			</button>
		</header>

		<div class="picker-filters">
			<input
				v-model="search"
				type="search"
				class="picker-search"
				:placeholder="`Search ${tabs.find((t) => t.key === tab)!.label.toLowerCase()}`"
			/>
			<template v-if="tab === 'site'">
				<SearchableSelect
					:options="serverOptions"
					:model-value="serverFilter"
					placeholder="Any server"
					@update:model-value="serverFilter = String($event ?? '')"
				/>
				<SearchableSelect
					:options="orgOptions"
					:model-value="orgFilter"
					placeholder="Any organization"
					@update:model-value="
						orgFilter =
							$event === '' || $event == null
								? ''
								: Number($event)
					"
				/>
				<SearchableSelect
					:options="pluginOptions"
					:model-value="pluginFilter"
					placeholder="Any plugin"
					@update:model-value="pluginFilter = String($event ?? '')"
				/>
			</template>
			<select
				v-else-if="tab === 'inventory_item'"
				v-model="inventoryType"
				aria-label="Inventory type"
			>
				<option value="">Any type</option>
				<option v-for="t in inventoryTypes" :key="t" :value="t">
					{{ t }}
				</option>
			</select>
		</div>

		<p v-if="loading" class="picker-note">Loading…</p>
		<p
			v-else-if="tab === 'inventory_item' && inventoryError"
			class="picker-note error"
		>
			{{ inventoryError }}
		</p>
		<p v-else-if="!rows.length" class="picker-note">Nothing matches.</p>
		<template v-else>
			<label class="picker-row picker-all">
				<input
					type="checkbox"
					:checked="allSelected"
					:disabled="!selectable.length"
					@change="toggleAll"
				/>
				<span>Select all {{ selectable.length }} shown</span>
			</label>
			<ul class="picker-list">
				<li v-for="r in rows" :key="r.id">
					<label class="picker-row" :class="{ disabled: inList(r) }">
						<input
							type="checkbox"
							:checked="inList(r) || selected.has(r.id)"
							:disabled="inList(r)"
							@change="toggle(r)"
						/>
						<span class="picker-text">{{ r.text }}</span>
						<span class="picker-detail">
							{{ inList(r) ? "on the list" : r.detail }}
						</span>
					</label>
				</li>
			</ul>
		</template>

		<footer class="picker-foot">
			<button
				type="button"
				class="btn btn-outline btn-sm"
				@click="emit('close')"
			>
				Done
			</button>
			<button
				type="button"
				class="btn btn-primary btn-sm"
				:disabled="!selected.size"
				@click="add"
			>
				Add {{ selected.size || "" }}
				{{ selected.size === 1 ? "item" : "items" }}
			</button>
		</footer>
	</div>
</template>

<style scoped>
.picker {
	border: 1px solid var(--border-color);
	border-radius: 8px;
	background: var(--bg-card);
	box-shadow: var(--shadow-sm);
	margin-top: 8px;
}

.picker-head {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 6px 8px 0 8px;
	border-bottom: 1px solid var(--border-color);
}

.picker-tabs {
	display: flex;
	gap: 2px;
	overflow-x: auto;

	& button {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 8px 12px;
		background: none;
		border: none;
		border-bottom: 2px solid transparent;
		font: inherit;
		font-size: 13px;
		font-weight: 500;
		color: var(--text-muted);
		cursor: pointer;
		white-space: nowrap;

		&:hover {
			color: var(--text-main);
		}
		&.active {
			color: var(--primary);
			border-bottom-color: var(--primary);
		}
	}
}

.picker-filters {
	display: flex;
	flex-wrap: wrap;
	gap: 8px;
	padding: 10px 12px;

	& > * {
		flex: 1 1 160px;
		min-width: 0;
	}
	& .picker-search {
		flex-basis: 100%;
	}
	& input,
	& select {
		padding: 6px 10px;
		font-size: 14px;
	}
}

.picker-note {
	margin: 0;
	padding: 12px;
	font-size: 14px;
	color: var(--text-muted);

	&.error {
		color: var(--error-text);
	}
}

.picker-list {
	list-style: none;
	margin: 0;
	padding: 0 4px;
	max-height: 280px;
	overflow-y: auto;
	border-top: 1px solid var(--border-color);
}

.picker-row {
	display: flex;
	align-items: center;
	gap: 10px;
	padding: 6px 8px;
	border-radius: 4px;
	cursor: pointer;
	font-size: 14px;
	min-width: 0;

	&:hover {
		background: var(--bg-hover);
	}
	&.disabled {
		cursor: default;
		color: var(--text-muted);
	}
	& input {
		margin: 0;
		flex-shrink: 0;
	}
}

.picker-all {
	margin: 0 4px 2px;
	font-weight: 500;
}

.picker-text {
	overflow-wrap: anywhere;
}

.picker-detail {
	margin-left: auto;
	padding-left: 8px;
	font-size: 12px;
	color: var(--text-muted);
	text-align: right;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
	max-width: 50%;
}

.picker-foot {
	display: flex;
	justify-content: flex-end;
	gap: 8px;
	padding: 10px 12px;
	border-top: 1px solid var(--border-color);
}
</style>
