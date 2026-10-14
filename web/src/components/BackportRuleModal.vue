<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useVulnBackportStore } from "../stores/vulnBackports";
import { useToastStore } from "../stores/toast";
import type { VulnBackportType } from "../types";

/**
 * Marks a vulnerability as fixed by a backport release: versions within a
 * branch from a minimum version up are treated as not affected. Prefilled
 * from the affected version the user clicked.
 */
const props = defineProps<{
	modelValue: boolean;
	type: VulnBackportType;
	/** Plugin slug; empty for WordPress core. */
	slug: string;
	vulnUuid: string;
	vulnName: string;
	/** The installed version the action was started from. */
	version: string;
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: boolean): void;
}>();

const backportStore = useVulnBackportStore();
const toast = useToastStore();

const branch = ref("");
const minVersion = ref("");
const reason = ref("");
const saving = ref(false);

watch(
	() => props.modelValue,
	(open) => {
		if (!open) return;
		// "11.1.2" suggests branch "11.1" with 11.1.2 as the first fixed release.
		const segments = props.version.split(".");
		branch.value =
			segments.length > 1 ? segments.slice(0, -1).join(".") : "";
		minVersion.value = props.version;
		reason.value = "";
	},
	// The parent may mount this already open.
	{ immediate: true },
);

const componentName = computed(() =>
	props.type === "core" ? "WordPress" : props.slug,
);

const canSave = computed(
	() =>
		!saving.value &&
		branch.value.trim() !== "" &&
		minVersion.value.trim() !== "" &&
		reason.value.trim() !== "",
);

const close = () => emit("update:modelValue", false);

async function save() {
	saving.value = true;
	try {
		const { rule, warnings } = await backportStore.addRule({
			type: props.type,
			slug: props.type === "core" ? "" : props.slug,
			vuln_uuid: props.vulnUuid,
			branch: branch.value.trim(),
			min_version: minVersion.value.trim(),
			reason: reason.value.trim(),
		});
		const n = rule.matched_sites;
		toast.addToast(
			`Rule saved: ${n} ${n === 1 ? "site" : "sites"} now marked as patched`,
			"success",
		);
		for (const warning of warnings) {
			toast.addToast(warning, "info", 10000);
		}
		close();
	} catch (e: any) {
		toast.addToast(`Failed to save rule: ${e.message}`, "error");
	} finally {
		saving.value = false;
	}
}
</script>

<template>
	<div v-if="modelValue" class="modal-overlay" @click.self="close">
		<div class="modal-content card">
			<h2>Mark patched in branch</h2>
			<p class="text-muted">
				{{ vulnName }}
			</p>
			<p class="text-muted">
				Use this when {{ componentName }} fixed the vulnerability in a
				backport release the vulnerability database doesn't list. Sites
				running {{ componentName }} {{ branch || "…" }}.x at
				{{ minVersion || "…" }} or later will no longer be reported as
				affected.
			</p>
			<form class="form-layout" @submit.prevent="save">
				<div class="grid-2-cols gap-4">
					<div class="form-group">
						<label for="backport-branch">Branch</label>
						<input
							id="backport-branch"
							v-model="branch"
							type="text"
							placeholder="E.g. 11.1"
						/>
					</div>
					<div class="form-group">
						<label for="backport-min">First patched version</label>
						<input
							id="backport-min"
							v-model="minVersion"
							type="text"
							placeholder="E.g. 11.1.2"
						/>
					</div>
				</div>
				<div class="form-group">
					<label for="backport-reason">Why is it patched?</label>
					<textarea
						id="backport-reason"
						v-model="reason"
						rows="3"
						maxlength="1000"
						placeholder="E.g. fix backported in 11.1.2, see the vendor changelog"
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
						:disabled="!canSave"
					>
						{{ saving ? "Saving..." : "Save rule" }}
					</button>
				</div>
			</form>
		</div>
	</div>
</template>
