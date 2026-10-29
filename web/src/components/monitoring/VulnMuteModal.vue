<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useVulnStore } from "../../stores/vulns";
import { useToastStore } from "../../stores/toast";
import AppIcon from "../AppIcon.vue";
import type { MuteTarget } from "./vuln";

const props = defineProps<{ target: MuteTarget | null }>();
const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved"): void;
}>();

const vulnStore = useVulnStore();
const toastStore = useToastStore();

const reason = ref("");
const saving = ref(false);
const confirming = computed(() => !!props.target?.mute);

watch(
	() => props.target,
	(t) => {
		reason.value = t?.mute?.reason ?? "";
	},
	{ immediate: true },
);

async function submit() {
	const t = props.target;
	if (!t || !reason.value.trim()) return;
	saving.value = true;
	try {
		if (t.mute) {
			await vulnStore.confirmMute(t.mute.id, reason.value.trim());
			toastStore.addToast(
				`Kept ${t.name} muted on ${t.siteName}`,
				"success",
			);
		} else {
			await vulnStore.mute(t.kind, t.slug, t.siteId, reason.value.trim());
			toastStore.addToast(`Muted ${t.name} on ${t.siteName}`, "success");
		}
		emit("saved");
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to save the mute",
			"error",
		);
	} finally {
		saving.value = false;
	}
}
</script>

<template>
	<Teleport to="body">
		<div v-if="target" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card mute-modal">
				<header class="modal-header">
					<h2>
						{{ confirming ? "Keep muted" : "Mute" }}
						{{ target.name }} on {{ target.siteName }}
					</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<p
					v-if="target.fixState === 'fix_available'"
					class="warning-note"
				>
					<strong>A fix is available</strong
					><template v-if="target.fixedIn">
						({{ target.fixedIn }})</template
					>. Muting stops this site from turning overdue for 90 days.
					Mute it only if it can't be updated yet, e.g. the update
					breaks the site.
				</p>
				<p class="hint">
					The mute is lifted when a new vulnerability applies<template
						v-if="target.fixState === 'no_fix'"
						>, or a fix or possibly fixing version comes
						out</template
					>, and needs review after 90 days.
				</p>

				<form @submit.prevent="submit">
					<div class="form-group">
						<label for="mute-reason">Reason</label>
						<textarea
							id="mute-reason"
							v-model="reason"
							rows="3"
							required
							placeholder="Why this is acceptable for now"
						></textarea>
					</div>
					<div class="form-actions">
						<button
							type="button"
							class="btn btn-outline btn-sm"
							@click="emit('close')"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary btn-sm"
							:disabled="saving || !reason.trim()"
						>
							{{ confirming ? "Keep muted" : "Mute" }}
						</button>
					</div>
				</form>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.mute-modal {
	max-width: 520px;
	display: flex;
	flex-direction: column;
	gap: 12px;
}

.hint {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}

.warning-note {
	margin: 0;
	padding: 8px 12px;
	border-radius: 6px;
	background: var(--warning-bg);
	color: var(--warning-text);
	border: 1px solid var(--warning-border);
	font-size: 13px;
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-actions {
	display: flex;
	justify-content: flex-end;
	gap: 8px;
	margin-top: 12px;
}
</style>
