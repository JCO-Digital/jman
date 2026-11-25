<script setup lang="ts">
import { computed, nextTick, ref } from "vue";
import { RouterLink, type RouteLocationRaw } from "vue-router";
import type { NewTaskChecklistItem, TaskChecklistRefType } from "../types";
import { useUserStore } from "../stores/user";
import { useDataStore } from "../stores/data";
import AppIcon from "./AppIcon.vue";
import TaskChecklistPicker from "./TaskChecklistPicker.vue";

/** One row: a saved item, or a draft one in the new-task form. */
export interface ChecklistRow {
	id: number;
	text: string;
	done: boolean;
	done_by?: string | null;
	done_at?: string | null;
	ref_type?: TaskChecklistRefType;
	ref_id?: string;
}

// Shows a checklist and reports what the user does with it; the parent
// saves the changes (to the task, or to the draft of a new one).

const props = defineProps<{
	items: ChecklistRow[];
	/** May add, rename, remove and reorder items. */
	canEdit: boolean;
	/** May tick items. */
	canTick: boolean;
	/** A draft has nothing to tick yet. */
	draft?: boolean;
}>();

const emit = defineEmits<{
	(e: "toggle", item: ChecklistRow): void;
	(e: "rename", item: ChecklistRow, text: string): void;
	(e: "remove", item: ChecklistRow): void;
	(e: "add", items: NewTaskChecklistItem[]): void;
	(e: "reorder", ids: number[]): void;
}>();

const userStore = useUserStore();
const dataStore = useDataStore();

const pickerOpen = ref(false);
const existingRefs = computed(
	() =>
		new Set(
			props.items
				.filter((i) => i.ref_type && i.ref_id)
				.map((i) => `${i.ref_type}:${i.ref_id}`),
		),
);

// A site or server that is no longer there; other kinds aren't loaded here.
function isGone(item: ChecklistRow) {
	if (!item.ref_id || !dataStore.isLoaded) return false;
	if (item.ref_type === "site")
		return !dataStore.sitesByIdMap.has(item.ref_id);
	if (item.ref_type === "server")
		return !dataStore.serversByIdMap.has(item.ref_id);
	return false;
}

const doneCount = computed(() => props.items.filter((i) => i.done).length);
const percent = computed(() =>
	props.items.length
		? Math.round((doneCount.value / props.items.length) * 100)
		: 0,
);

// Adding: Enter adds the line; pasting several lines adds one item each.
const newText = ref("");
function addNew() {
	const text = newText.value.trim();
	if (!text) return;
	emit("add", [{ text }]);
	newText.value = "";
}
function onPaste(e: ClipboardEvent) {
	const pasted = e.clipboardData?.getData("text") ?? "";
	const lines = pasted
		.split(/\r?\n/)
		.map((l) => l.replace(/^\s*(?:[-*•]|\[[ xX]?\])\s*/, "").trim())
		.filter(Boolean);
	if (lines.length < 2) return;
	e.preventDefault();
	emit(
		"add",
		lines.map((text) => ({ text })),
	);
}

// Renaming in place.
const editingId = ref<number | null>(null);
const editText = ref("");
const editInput = ref<HTMLInputElement[] | null>(null);
function startEdit(item: ChecklistRow) {
	if (!props.canEdit) return;
	editingId.value = item.id;
	editText.value = item.text;
	nextTick(() => editInput.value?.[0]?.focus());
}
function finishEdit(item: ChecklistRow) {
	if (editingId.value !== item.id) return;
	const text = editText.value.trim();
	editingId.value = null;
	if (text && text !== item.text) emit("rename", item, text);
}

// Reordering by dragging the handle.
const dragId = ref<number | null>(null);
const overId = ref<number | null>(null);
function onDragStart(e: DragEvent, item: ChecklistRow) {
	dragId.value = item.id;
	e.dataTransfer?.setData("text/plain", String(item.id));
	if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
}
function onDrop(target: ChecklistRow) {
	const from = dragId.value;
	dragId.value = overId.value = null;
	if (from == null || from === target.id) return;
	const ids = props.items.map((i) => i.id).filter((id) => id !== from);
	ids.splice(ids.indexOf(target.id), 0, from);
	emit("reorder", ids);
}

function refLink(item: ChecklistRow): RouteLocationRaw | null {
	if (!item.ref_id || isGone(item)) return null;
	switch (item.ref_type) {
		case "site":
			return { name: "site-detail", params: { id: item.ref_id } };
		case "plugin":
			return { name: "plugin-detail", params: { name: item.ref_id } };
		case "inventory_item":
			return { name: "inventory-detail", params: { id: item.ref_id } };
	}
	return null;
}

const refIcon: Record<TaskChecklistRefType, string> = {
	site: "site",
	server: "box",
	plugin: "plugin",
	inventory_item: "tag",
};

function doneTitle(item: ChecklistRow) {
	if (!item.done || !item.done_by) return undefined;
	const when = item.done_at
		? new Date(item.done_at).toLocaleString("de-DE", {
				dateStyle: "medium",
				timeStyle: "short",
			})
		: "";
	return `Done by ${userStore.resolveDisplayName(item.done_by)} ${when}`;
}
</script>

<template>
	<section class="checklist">
		<header class="checklist-head">
			<h3>Checklist</h3>
			<template v-if="items.length && !draft">
				<span class="checklist-count"
					>{{ doneCount }} / {{ items.length }}</span
				>
				<div
					class="checklist-bar"
					role="progressbar"
					:aria-valuenow="percent"
					aria-valuemin="0"
					aria-valuemax="100"
				>
					<span
						:class="{ complete: percent === 100 }"
						:style="{ width: percent + '%' }"
					></span>
				</div>
			</template>
		</header>

		<ul v-if="items.length" class="checklist-items">
			<li
				v-for="item in items"
				:key="item.id"
				:class="{
					done: item.done,
					dragging: dragId === item.id,
					'drag-over': overId === item.id && dragId !== item.id,
				}"
				@dragover.prevent="overId = item.id"
				@dragleave="overId = overId === item.id ? null : overId"
				@drop.prevent="onDrop(item)"
			>
				<span
					v-if="canEdit && items.length > 1"
					class="drag"
					draggable="true"
					title="Drag to reorder"
					@dragstart="onDragStart($event, item)"
					@dragend="dragId = overId = null"
				>
					<AppIcon name="drag-handle" size="14" />
				</span>
				<input
					v-if="!draft"
					type="checkbox"
					:checked="item.done"
					:disabled="!canTick"
					:title="doneTitle(item)"
					:aria-label="`Done: ${item.text}`"
					@change="emit('toggle', item)"
				/>
				<span v-else class="bullet" aria-hidden="true"></span>

				<input
					v-if="editingId === item.id"
					ref="editInput"
					v-model="editText"
					class="item-edit"
					maxlength="1000"
					@keydown.enter.prevent="finishEdit(item)"
					@keydown.esc.stop.prevent="editingId = null"
					@blur="finishEdit(item)"
				/>
				<span
					v-else
					class="item-text"
					:class="{ editable: canEdit }"
					:title="canEdit ? 'Click to edit' : undefined"
					@click="startEdit(item)"
				>
					<AppIcon
						v-if="item.ref_type"
						:name="refIcon[item.ref_type]"
						size="13"
						class="ref-icon"
					/>
					{{ item.text }}
					<span
						v-if="isGone(item)"
						class="gone"
						title="No longer in jman"
						>removed</span
					>
				</span>

				<RouterLink
					v-if="refLink(item)"
					:to="refLink(item)!"
					class="icon-btn icon-btn-sm"
					title="Open"
				>
					<AppIcon name="external-link" size="13" />
				</RouterLink>
				<button
					v-if="canEdit"
					type="button"
					class="icon-btn icon-btn-sm danger remove"
					title="Remove item"
					@click="emit('remove', item)"
				>
					<AppIcon name="x" size="13" />
				</button>
			</li>
		</ul>

		<div v-if="canEdit" class="checklist-add">
			<AppIcon name="plus-circle" size="14" />
			<input
				v-model="newText"
				type="text"
				placeholder="Add an item — Enter to add, paste lines to add several"
				maxlength="1000"
				@keydown.enter.prevent="addNew"
				@paste="onPaste"
			/>
			<button
				type="button"
				class="btn btn-outline btn-sm"
				:class="{ active: pickerOpen }"
				title="Add sites, servers, plugins or inventory items"
				@click="pickerOpen = !pickerOpen"
			>
				<AppIcon name="site" size="14" />
				Add from…
			</button>
		</div>
		<TaskChecklistPicker
			v-if="canEdit && pickerOpen"
			:existing="existingRefs"
			@add="emit('add', $event)"
			@close="pickerOpen = false"
		/>
		<p v-else-if="!items.length" class="empty-hint">No checklist.</p>
	</section>
</template>

<style scoped>
.checklist-head {
	display: flex;
	align-items: center;
	gap: 10px;
	margin-bottom: 8px;

	& h3 {
		margin: 0;
		font-size: 14px;
		font-weight: 600;
		color: var(--text-heading);
	}
}

.checklist-count {
	font-size: 12px;
	color: var(--text-muted);
	font-variant-numeric: tabular-nums;
}

.checklist-bar {
	flex: 1;
	height: 6px;
	border-radius: 3px;
	background: var(--badge-default-bg);
	overflow: hidden;

	& span {
		display: block;
		height: 100%;
		background: var(--primary);
		transition: width 0.2s;

		&.complete {
			background: var(--badge-active-text);
		}
	}
}

.checklist-items {
	list-style: none;
	margin: 0;
	padding: 0;

	& li {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 5px 6px;
		border-radius: 6px;
		border-top: 2px solid transparent;
		min-height: 34px;

		&:hover {
			background: var(--bg-hover);
		}
		&.dragging {
			opacity: 0.4;
		}
		&.drag-over {
			border-top-color: var(--primary);
		}
		&.done .item-text {
			color: var(--text-muted);
			text-decoration: line-through;
		}
		& input[type="checkbox"] {
			width: 16px;
			height: 16px;
			margin: 0;
			flex-shrink: 0;
			cursor: pointer;
		}
	}
}

.drag {
	cursor: grab;
	color: var(--text-muted);
	display: inline-flex;
	opacity: 0;
}
.checklist-items li:hover .drag {
	opacity: 1;
}

.bullet {
	width: 6px;
	height: 6px;
	border-radius: 50%;
	background: var(--text-muted);
	margin: 0 5px;
	flex-shrink: 0;
}

.item-text {
	flex: 1;
	min-width: 0;
	font-size: 14px;
	line-height: 1.4;
	overflow-wrap: anywhere;

	&.editable {
		cursor: text;
	}
}

.ref-icon {
	color: var(--text-muted);
	vertical-align: -2px;
	margin-right: 2px;
}

.item-edit {
	flex: 1;
	min-width: 0;
	padding: 3px 6px;
	font-size: 14px;
}

.remove {
	opacity: 0;
}
.checklist-items li:hover .remove,
.remove:focus-visible {
	opacity: 1;
}

@media (hover: none) {
	.drag,
	.remove {
		opacity: 1;
	}
}

.checklist-add {
	display: flex;
	align-items: center;
	gap: 8px;
	padding: 4px 6px;
	color: var(--text-muted);

	& input {
		flex: 1;
		border: 1px dashed var(--border-input);
		background: transparent;
		padding: 6px 8px;
		font-size: 14px;

		&:focus {
			border-style: solid;
		}
	}
}

.checklist-add .btn {
	flex-shrink: 0;
	color: var(--text-main);

	&.active {
		border-color: var(--primary);
		color: var(--primary);
	}
}

.gone {
	margin-left: 6px;
	padding: 0 6px;
	border-radius: 9999px;
	font-size: 11px;
	background: var(--badge-default-bg);
	color: var(--text-muted);
	text-decoration: none;
	display: inline-block;
}

.empty-hint {
	margin: 0;
	font-size: 13px;
	color: var(--text-muted);
}
</style>
