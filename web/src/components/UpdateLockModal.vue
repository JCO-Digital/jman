<script setup lang="ts">
import { ref, watch } from "vue";
import { useDataStore } from "../stores/data";
import { useToastStore } from "../stores/toast";

/**
 * Locks a site (no plugin) or one of its plugins to fix-release updates,
 * with a comment saying why. Also edits the comment of an existing lock.
 */
const props = defineProps<{
	modelValue: boolean;
	siteId: string;
	siteName: string;
	/** Plugin slug, or "" to lock the whole site. */
	plugin: string;
	/** The existing lock's comment, when editing one. */
	comment?: string;
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: boolean): void;
}>();

const dataStore = useDataStore();
const toast = useToastStore();

const draft = ref("");
const saving = ref(false);

watch(
	() => props.modelValue,
	(open) => {
		if (open) draft.value = props.comment ?? "";
	},
);

const close = () => emit("update:modelValue", false);

async function save() {
	saving.value = true;
	try {
		await dataStore.lockUpdates(props.siteId, props.plugin, draft.value);
		close();
	} catch (e: any) {
		toast.addToast(`Failed to lock updates: ${e.message}`, "error");
	} finally {
		saving.value = false;
	}
}
</script>

<template>
	<div v-if="modelValue" class="modal-overlay" @click.self="close">
		<div class="modal-content card">
			<h2>
				Lock updates:
				{{ plugin ? `${plugin} on ${siteName}` : siteName }}
			</h2>
			<p class="text-muted">
				<template v-if="plugin">
					Updates of this plugin only install fix releases (e.g. 9.3.0
					→ 9.3.2), including bulk and vulnerability updates.
				</template>
				<template v-else>
					WordPress core and every plugin on this site only get fix
					releases (e.g. 6.6.1 → 6.6.2, 9.3.0 → 9.3.2), including bulk
					and vulnerability updates.
				</template>
				A bigger update needs a confirmed major update, and the lock
				stays in place afterwards.
			</p>
			<form class="form-layout" @submit.prevent="save">
				<div class="form-group">
					<label for="lock-comment">Why is it locked?</label>
					<textarea
						id="lock-comment"
						v-model="draft"
						rows="3"
						maxlength="1000"
						placeholder="E.g. custom checkout, test updates on staging first"
						autofocus
					></textarea>
				</div>
				<div class="form-actions">
					<button
						type="button"
						class="btn btn-outline"
						@click="close"
					>
						Cancel
					</button>
					<button
						type="submit"
						class="btn btn-primary"
						:disabled="saving"
					>
						{{ comment === undefined ? "Lock" : "Save" }}
					</button>
				</div>
			</form>
		</div>
	</div>
</template>
