<script setup lang="ts">
import { computed, ref, watch } from "vue";
import AppIcon from "./AppIcon.vue";
import type {
	InventoryDetailField,
	InventoryItem,
	InventoryItemPayload,
	InventoryItemType,
	InventoryTypeInfo,
} from "../types";

const props = defineProps<{
	item: InventoryItem | null;
	types: InventoryTypeInfo[];
	/** Preselected type for a new item (e.g. the list's active type filter). */
	initialType?: InventoryItemType;
}>();

const emit = defineEmits<{
	(e: "save", payload: InventoryItemPayload): void;
	(e: "close"): void;
}>();

interface DetailRow {
	key: string;
	value: string;
	kind: "text" | "list";
	hint?: string;
	/** False for rows that come from the type's template. */
	custom: boolean;
}

const form = ref({
	type: "server" as InventoryItemType,
	name: "",
	description: "",
	tags: "",
	site_id: "",
	server_id: "",
});

const detailRows = ref<DetailRow[]>([]);
const jsonMode = ref(false);
const jsonText = ref("");
const detailsError = ref<string | null>(null);

// On a provider-linked item the sync owns type and name.
const typeNameLocked = computed(
	() => !!props.item && (props.item.links?.length ?? 0) > 0,
);

const templateFor = (type: InventoryItemType): InventoryDetailField[] =>
	props.types.find((t) => t.type === type)?.detail_fields ?? [];

// rowsFromDetails turns a stored details object into form rows: arrays
// become comma-separated "list" rows, everything else "text".
const rowsFromDetails = (details: Record<string, unknown>): DetailRow[] =>
	Object.entries(details).map(([key, value]) => {
		if (Array.isArray(value)) {
			return {
				key,
				value: value.map((v) => String(v)).join(", "),
				kind: "list",
				custom: true,
			};
		}
		return {
			key,
			value:
				typeof value === "object"
					? JSON.stringify(value)
					: String(value),
			kind: "text",
			custom: true,
		};
	});

// buildRows starts from the type's template, carrying over any values the
// current rows already have for those keys, and appends the remaining rows.
// With dropStaleTemplate (a type switch on a new item), rows that came from
// the old type's template and aren't in the new one are dropped; custom
// rows are always kept.
const buildRows = (
	type: InventoryItemType,
	existing: DetailRow[],
	dropStaleTemplate = false,
) => {
	const byKey = new Map(existing.map((r) => [r.key, r]));
	const rows: DetailRow[] = templateFor(type).map((f) => {
		const prev = byKey.get(f.key);
		byKey.delete(f.key);
		return {
			key: f.key,
			value: prev?.value ?? "",
			kind: f.kind,
			hint: f.hint,
			custom: false,
		};
	});
	for (const rest of byKey.values()) {
		if (dropStaleTemplate && !rest.custom) continue;
		rows.push(rest);
	}
	return rows;
};

// detailsFromRows builds the JSON object: empty keys and empty values are
// left out; list rows become arrays.
const detailsFromRows = (): Record<string, unknown> => {
	const details: Record<string, unknown> = {};
	for (const row of detailRows.value) {
		const key = row.key.trim();
		if (!key) continue;
		if (row.kind === "list") {
			const items = row.value
				.split(",")
				.map((v) => v.trim())
				.filter((v) => v !== "");
			if (items.length > 0) details[key] = items;
		} else if (row.value.trim() !== "") {
			details[key] = row.value;
		}
	}
	return details;
};

const parseDetails = (text: string): Record<string, unknown> | null => {
	try {
		const parsed: unknown = JSON.parse(text || "{}");
		if (
			typeof parsed !== "object" ||
			parsed === null ||
			Array.isArray(parsed)
		)
			return null;
		return parsed as Record<string, unknown>;
	} catch {
		return null;
	}
};

const toggleJsonMode = () => {
	detailsError.value = null;
	if (!jsonMode.value) {
		// Form -> JSON: serialize the current rows.
		jsonText.value = JSON.stringify(detailsFromRows(), null, 2);
		jsonMode.value = true;
		return;
	}
	// JSON -> form: parse, staying in JSON mode on invalid input.
	const parsed = parseDetails(jsonText.value);
	if (parsed === null) {
		detailsError.value = "Details is not a valid JSON object";
		return;
	}
	detailRows.value = rowsFromDetails(parsed);
	jsonMode.value = false;
};

const addField = () => {
	detailRows.value.push({ key: "", value: "", kind: "text", custom: true });
};

const removeField = (index: number) => {
	detailRows.value.splice(index, 1);
};

watch(
	() => props.item,
	(item) => {
		detailsError.value = null;
		jsonMode.value = false;
		if (item) {
			form.value = {
				type: item.type,
				name: item.name,
				description: item.description ?? "",
				tags: (item.tags ?? []).join(", "),
				site_id: item.site_id ?? "",
				server_id: item.server_id ?? "",
			};
			detailRows.value = buildRows(
				item.type,
				rowsFromDetails(item.details ?? {}),
			);
		} else {
			const type = props.initialType ?? props.types[0]?.type ?? "server";
			form.value = {
				type,
				name: "",
				description: "",
				tags: "",
				site_id: "",
				server_id: "",
			};
			detailRows.value = buildRows(type, []);
		}
	},
	{ immediate: true },
);

// Switching the type re-templates the fields (linked items have it locked):
// the old type's template fields are dropped, custom fields are kept.
watch(
	() => form.value.type,
	(type) => {
		if (!props.item)
			detailRows.value = buildRows(type, detailRows.value, true);
	},
);

const submit = () => {
	const details = jsonMode.value
		? parseDetails(jsonText.value)
		: detailsFromRows();
	if (details === null) {
		detailsError.value = jsonMode.value
			? "Details is not a valid JSON object"
			: "Details could not be built from the fields";
		return;
	}
	detailsError.value = null;
	emit("save", {
		type: form.value.type,
		name: form.value.name,
		description: form.value.description,
		tags: form.value.tags
			.split(",")
			.map((t) => t.trim())
			.filter((t) => t !== ""),
		details,
		site_id: form.value.site_id || null,
		server_id: form.value.server_id || null,
	});
};
</script>

<template>
	<div class="modal-overlay" @click.self="emit('close')">
		<div class="modal-content card">
			<h2>{{ item ? "Edit item" : "Add inventory item" }}</h2>
			<form class="form-layout" @submit.prevent="submit">
				<div class="form-group">
					<label for="inv-type">Type</label>
					<select
						id="inv-type"
						v-model="form.type"
						:disabled="typeNameLocked"
					>
						<option
							v-for="t in types"
							:key="t.type"
							:value="t.type"
						>
							{{ t.type }}
						</option>
					</select>
				</div>
				<div class="form-group">
					<label for="inv-name">
						{{ form.type === "domain" ? "Domain" : "Name" }}
					</label>
					<input
						id="inv-name"
						v-model="form.name"
						type="text"
						required
						:disabled="typeNameLocked"
						:placeholder="
							form.type === 'domain'
								? 'www.example.com'
								: 'web1.internal'
						"
					/>
					<p v-if="form.type === 'domain'" class="text-muted">
						Stored as the apex domain (www.example.com →
						example.com).
					</p>
				</div>
				<div class="form-group">
					<label for="inv-desc">Description</label>
					<input
						id="inv-desc"
						v-model="form.description"
						type="text"
					/>
				</div>
				<div class="form-group">
					<label for="inv-tags">Tags (comma-separated)</label>
					<input id="inv-tags" v-model="form.tags" type="text" />
				</div>
				<div class="form-group">
					<div class="flex-between">
						<label>Details</label>
						<div class="flex-row gap-2">
							<button
								v-if="!jsonMode"
								type="button"
								class="btn btn-outline btn-sm"
								@click="addField"
							>
								<AppIcon name="plus-circle" size="16" />
								Add field
							</button>
							<button
								type="button"
								class="btn btn-outline btn-sm"
								@click="toggleJsonMode"
							>
								{{ jsonMode ? "Back to form" : "Edit JSON" }}
							</button>
						</div>
					</div>

					<template v-if="!jsonMode">
						<div
							v-for="(row, i) in detailRows"
							:key="i"
							class="flex-row gap-2"
						>
							<input
								v-model="row.key"
								type="text"
								:placeholder="'field'"
								:aria-label="'Field name ' + (i + 1)"
								class="detail-key-input"
							/>
							<input
								v-model="row.value"
								type="text"
								:placeholder="
									row.kind === 'list'
										? 'comma-separated values'
										: row.hint || 'value'
								"
								:aria-label="'Value for ' + row.key"
								class="detail-value-input"
							/>
							<button
								type="button"
								class="icon-btn icon-btn-sm danger"
								:title="'Remove ' + (row.key || 'field')"
								@click="removeField(i)"
							>
								<AppIcon name="x" size="14" />
							</button>
						</div>
						<p class="text-muted">
							Fields left empty are not saved. List fields are
							comma-separated. No secrets — at most usernames.
						</p>
					</template>

					<textarea
						v-else
						v-model="jsonText"
						rows="10"
						aria-label="Details as JSON"
					></textarea>
					<p v-if="detailsError" class="form-error">
						{{ detailsError }}
					</p>
				</div>
				<div class="form-group">
					<label for="inv-site">Linked site ID (optional UUID)</label>
					<input id="inv-site" v-model="form.site_id" type="text" />
				</div>
				<div class="form-group">
					<label for="inv-server"
						>Linked server ID (optional UUID)</label
					>
					<input
						id="inv-server"
						v-model="form.server_id"
						type="text"
					/>
				</div>
				<div class="form-actions">
					<button
						type="button"
						class="btn btn-outline"
						@click="emit('close')"
					>
						Cancel
					</button>
					<button type="submit" class="btn btn-primary">
						{{ item ? "Save" : "Add" }}
					</button>
				</div>
			</form>
		</div>
	</div>
</template>

<style scoped>
/* Unique to this component: key/value sizing in the details field rows. */
.detail-key-input {
	flex: 0 0 180px;
}
.detail-value-input {
	flex: 1;
	min-width: 0;
}
</style>
