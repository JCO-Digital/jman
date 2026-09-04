<script setup lang="ts">
import { ref } from "vue";

export interface EditableInfoItem {
	label: string;
	key: string;
	value: any;
	type?: "text" | "email" | "tel" | "textarea" | "select";
	options?: { label: string; value: any }[];
	required?: boolean;
	copyable?: boolean;
}

const props = defineProps<{
	title: string;
	items: EditableInfoItem[];
	isLoading?: boolean;
	editable?: boolean;
	onSave?: (values: Record<string, any>) => Promise<void> | void;
}>();

const emit = defineEmits<{
	(e: "save", values: Record<string, any>): void;
}>();

const isEditing = ref(false);
const isSaving = ref(false);
const localValues = ref<Record<string, any>>({});
const copiedIndex = ref<number | null>(null);

const startEditing = () => {
	localValues.value = props.items.reduce(
		(acc, item) => {
			acc[item.key] = item.value;
			return acc;
		},
		{} as Record<string, any>,
	);
	isEditing.value = true;
};

const cancelEditing = () => {
	isEditing.value = false;
};

const handleSave = async () => {
	isSaving.value = true;
	try {
		const values = { ...localValues.value };
		if (props.onSave) {
			await props.onSave(values);
		}
		emit("save", values);
		isEditing.value = false;
	} catch (error) {
		console.error("Failed to save:", error);
	} finally {
		isSaving.value = false;
	}
};

const copyToClipboard = async (value: any, index: number) => {
	if (value === undefined || value === null) return;
	try {
		await navigator.clipboard.writeText(value.toString());
		copiedIndex.value = index;
		setTimeout(() => {
			copiedIndex.value = null;
		}, 2000);
	} catch (err) {
		console.error("Failed to copy: ", err);
	}
};
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>{{ title }}</h2>
			<div v-if="!isLoading && editable !== false" class="flex-row gap-3">
				<button
					v-if="!isEditing"
					class="btn btn-text"
					@click="startEditing"
				>
					Edit
				</button>
				<template v-else>
					<button
						class="btn btn-text text-muted"
						:disabled="isSaving"
						@click="cancelEditing"
					>
						Cancel
					</button>
					<button
						class="btn btn-primary btn-sm"
						:disabled="isSaving"
						@click="handleSave"
					>
						{{ isSaving ? "Saving..." : "Save" }}
					</button>
				</template>
			</div>
		</div>

		<div v-if="!isEditing" class="info-grid mt-4">
			<div
				v-for="(item, index) in items"
				:key="item.key"
				class="info-item"
			>
				<span class="label">{{ item.label }}</span>
				<div class="value-container">
					<span
						class="value"
						:class="{ copyable: item.copyable }"
						:title="item.copyable ? 'Click to copy' : ''"
						@click="
							item.copyable
								? copyToClipboard(item.value, index)
								: null
						"
					>
						{{ item.value ?? "—" }}
						<span v-if="copiedIndex === index" class="copy-feedback"
							>Copied!</span
						>
					</span>
				</div>
			</div>
		</div>

		<div v-else class="info-grid mt-4">
			<div
				v-for="item in items"
				:key="item.key"
				class="form-group"
				:class="{ 'full-width': item.type === 'textarea' }"
			>
				<label :for="item.key">{{ item.label }}</label>

				<select
					v-if="item.type === 'select'"
					:id="item.key"
					v-model="localValues[item.key]"
					:disabled="isSaving"
				>
					<option
						v-for="opt in item.options"
						:key="opt.value"
						:value="opt.value"
					>
						{{ opt.label }}
					</option>
				</select>

				<textarea
					v-else-if="item.type === 'textarea'"
					:id="item.key"
					v-model="localValues[item.key]"
					:disabled="isSaving"
				></textarea>

				<input
					v-else
					:id="item.key"
					v-model="localValues[item.key]"
					:type="item.type || 'text'"
					:required="item.required"
					:disabled="isSaving"
				/>
			</div>
		</div>
	</section>
</template>

<style scoped>
.form-group.full-width {
	grid-column: 1 / -1;
}
</style>
