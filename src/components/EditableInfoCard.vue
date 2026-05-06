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
			<div v-if="!isLoading && editable !== false" class="header-actions">
				<button
					v-if="!isEditing"
					class="text-btn"
					@click="startEditing"
				>
					Edit
				</button>
				<template v-else>
					<button
						class="text-btn cancel"
						:disabled="isSaving"
						@click="cancelEditing"
					>
						Cancel
					</button>
					<button
						class="primary-btn-sm"
						:disabled="isSaving"
						@click="handleSave"
					>
						{{ isSaving ? "Saving..." : "Save" }}
					</button>
				</template>
			</div>
		</div>

		<div v-if="!isEditing" class="info-grid">
			<div
				v-for="(item, index) in items"
				:key="item.key"
				class="info-item"
			>
				<span class="label">{{ item.label }}:</span>
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

		<div v-else class="edit-form">
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
.card-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 20px;
}

.card-header h2 {
	margin: 0;
	font-size: 1.1rem;
}

.header-actions {
	display: flex;
	gap: 12px;
}

.info-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
	gap: 20px;
}

.info-item {
	display: flex;
	flex-direction: column;
	gap: 4px;
}

.label {
	font-size: 0.85em;
	color: var(--text-muted);
	font-weight: 500;
}

.value-container {
	display: flex;
	align-items: center;
	gap: 8px;
}

.value {
	font-weight: 500;
	word-break: break-word;
	position: relative;
}

.value.copyable {
	cursor: pointer;
	transition: color 0.2s;
}

.value.copyable:hover {
	color: var(--primary);
}

.copy-feedback {
	position: absolute;
	bottom: 100%;
	left: 50%;
	transform: translateX(-50%);
	background: var(--bg-card);
	color: var(--primary);
	padding: 2px 6px;
	border-radius: 4px;
	font-size: 0.7em;
	box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
	pointer-events: none;
	white-space: nowrap;
	z-index: 10;
}

.edit-form {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
	gap: 20px;
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-group.full-width {
	grid-column: 1 / -1;
}

.form-group label {
	font-size: 0.85rem;
	font-weight: 600;
	color: var(--text-muted);
}

.form-group input,
.form-group select,
.form-group textarea {
	padding: 10px 12px;
	border: 1px solid var(--border-input);
	border-radius: 6px;
	background: var(--bg-main);
	color: var(--text-main);
	font-size: 0.95rem;
	transition: border-color 0.2s;
}

.form-group input:focus,
.form-group select:focus,
.form-group textarea:focus {
	outline: none;
	border-color: var(--primary);
}

.form-group textarea {
	min-height: 100px;
	resize: vertical;
}

.text-btn {
	background: none;
	border: none;
	color: var(--primary);
	font-weight: 600;
	cursor: pointer;
	padding: 6px 12px;
	font-size: 0.9rem;
	border-radius: 4px;
	transition: background-color 0.2s;
}

.text-btn:hover:not(:disabled) {
	background-color: var(--bg-hover);
}

.text-btn.cancel {
	color: var(--text-muted);
}

.text-btn:disabled {
	opacity: 0.5;
	cursor: not-allowed;
}

.primary-btn-sm {
	padding: 6px 16px;
	background-color: var(--primary);
	color: white;
	border: none;
	border-radius: 4px;
	cursor: pointer;
	font-size: 0.9rem;
	font-weight: 600;
	transition: filter 0.2s;
}

.primary-btn-sm:hover:not(:disabled) {
	filter: brightness(1.1);
}

.primary-btn-sm:disabled {
	opacity: 0.7;
	cursor: not-allowed;
}
</style>
