<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
	InventoryItem,
	InventoryItemPayload,
	InventoryItemType,
	InventoryTypeInfo,
} from "../types";

const props = defineProps<{
	item: InventoryItem | null;
	types: InventoryTypeInfo[];
}>();

const emit = defineEmits<{
	(e: "save", payload: InventoryItemPayload): void;
	(e: "close"): void;
}>();

const form = ref({
	type: "server" as InventoryItemType,
	name: "",
	description: "",
	tags: "",
	details: "{}",
	site_id: "",
	server_id: "",
});

const detailsError = ref<string | null>(null);

// On a provider-linked item the sync owns type and name.
const typeNameLocked = computed(
	() => !!props.item && (props.item.links?.length ?? 0) > 0,
);

const detailHint = computed(() => {
	const info = props.types.find((t) => t.type === form.value.type);
	return info?.detail_fields ?? [];
});

watch(
	() => props.item,
	(item) => {
		detailsError.value = null;
		if (item) {
			form.value = {
				type: item.type,
				name: item.name,
				description: item.description ?? "",
				tags: (item.tags ?? []).join(", "),
				details: JSON.stringify(item.details ?? {}, null, 2),
				site_id: item.site_id ?? "",
				server_id: item.server_id ?? "",
			};
		} else {
			form.value = {
				type: props.types[0]?.type ?? "server",
				name: "",
				description: "",
				tags: "",
				details: "{}",
				site_id: "",
				server_id: "",
			};
		}
	},
	{ immediate: true },
);

const submit = () => {
	let details: Record<string, unknown>;
	try {
		const parsed: unknown = JSON.parse(form.value.details || "{}");
		if (
			typeof parsed !== "object" ||
			parsed === null ||
			Array.isArray(parsed)
		) {
			detailsError.value = "Details must be a JSON object";
			return;
		}
		details = parsed as Record<string, unknown>;
		detailsError.value = null;
	} catch {
		detailsError.value = "Details is not valid JSON";
		return;
	}
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
					<label for="inv-details">Details (JSON object)</label>
					<textarea
						id="inv-details"
						v-model="form.details"
						rows="8"
						class="font-mono"
					></textarea>
					<p v-if="detailHint.length" class="text-muted">
						Suggested for {{ form.type }}:
						{{ detailHint.join(", ") }}. No secrets — at most
						usernames.
					</p>
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
