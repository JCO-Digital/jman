<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useMonitorStore } from "../../stores/monitor";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import { BASE_URL } from "../../utils/api";
import AppIcon from "../AppIcon.vue";
import type { MonitorCheck, MonitorCheckPayload } from "../../types";

interface Props {
	visible: boolean;
	/** The heartbeat check to edit; null creates a new one. */
	editCheck?: MonitorCheck | null;
}

const props = withDefaults(defineProps<Props>(), { editCheck: null });

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved", check: MonitorCheck): void;
}>();

const monitorStore = useMonitorStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

type Unit = "minutes" | "hours" | "days";
const unitSeconds: Record<Unit, number> = {
	minutes: 60,
	hours: 3600,
	days: 86400,
};

const name = ref("");
const intervalValue = ref<number | "">(1);
const intervalUnit = ref<Unit>("days");
const graceValue = ref<number | "">(1);
const graceUnit = ref<Unit>("hours");
const failThreshold = ref<number | "">(1);

const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);
/** A token just issued: shown once, until the modal closes. */
const issuedToken = ref<string | null>(null);
/** The check as last saved, for its token details. */
const current = ref<MonitorCheck | null>(null);

const isEdit = computed(() => !!current.value);
const title = computed(() =>
	isEdit.value
		? `Edit check-in ${current.value?.name ?? ""}`
		: "Add Check-in",
);

/** The check-in URL, absolute so it can be pasted into a script. */
const checkInBase = computed(() => {
	const base = BASE_URL.startsWith("http")
		? BASE_URL
		: `${window.location.origin}${BASE_URL}`;
	return `${base}/checkin`;
});

const curlSnippet = computed(() => {
	const token = issuedToken.value ?? "$TOKEN";
	const auth = `-H "Authorization: Bearer ${token}"`;
	return [
		`curl -fsS -X POST ${auth} ${checkInBase.value}/start`,
		`run-backup && s=ok || s=fail`,
		`curl -fsS -X POST ${auth} ${checkInBase.value}/$s --data "exit $?"`,
	].join("\n");
});

/** Splits seconds into the largest unit that divides them evenly. */
function splitSeconds(seconds: number): { value: number; unit: Unit } {
	for (const unit of ["days", "hours", "minutes"] as Unit[]) {
		if (seconds > 0 && seconds % unitSeconds[unit] === 0) {
			return { value: seconds / unitSeconds[unit], unit };
		}
	}
	return { value: Math.round(seconds / 60), unit: "minutes" };
}

watch(
	() => props.visible,
	(open) => {
		if (!open) return;
		const c = props.editCheck;
		current.value = c;
		errorMessage.value = null;
		issuedToken.value = null;
		name.value = c?.name ?? "";
		const interval = splitSeconds(c?.interval_seconds ?? 86400);
		intervalValue.value = interval.value;
		intervalUnit.value = interval.unit;
		const grace =
			c && !c.grace_seconds
				? { value: 0, unit: "minutes" as Unit }
				: splitSeconds(c?.grace_seconds ?? 3600);
		graceValue.value = grace.value;
		graceUnit.value = grace.unit;
		failThreshold.value = c?.fail_threshold ?? 1;
	},
);

function toSeconds(value: number | "", unit: Unit): number {
	return Math.round(Number(value || 0) * unitSeconds[unit]);
}

async function handleSubmit() {
	errorMessage.value = null;
	const payload: MonitorCheckPayload = {
		name: name.value.trim(),
		interval_seconds: toSeconds(intervalValue.value, intervalUnit.value),
		grace_seconds: toSeconds(graceValue.value, graceUnit.value),
		fail_threshold: Number(failThreshold.value || 1),
	};
	isSubmitting.value = true;
	try {
		if (current.value) {
			const saved = await monitorStore.updateCheck(
				current.value.id,
				payload,
			);
			toastStore.addToast("Check-in saved", "success");
			emit("saved", saved);
			emit("close");
			return;
		}
		const created = await monitorStore.createCheck({
			...payload,
			type: "heartbeat",
		});
		// Stay open to show the token, which can't be seen again.
		current.value = created;
		issuedToken.value = created.token ?? null;
		toastStore.addToast("Check-in added", "success");
		emit("saved", created);
	} catch (e) {
		errorMessage.value =
			e instanceof Error ? e.message : "Failed to save the check-in";
	} finally {
		isSubmitting.value = false;
	}
}

async function rotateToken() {
	if (!current.value) return;
	if (
		!(await confirm(
			"Issue a new check-in token? The current token stops working at once, so update the job with the new one.",
			{ confirmLabel: "Issue new token", danger: true },
		))
	) {
		return;
	}
	try {
		const updated = await monitorStore.issueCheckToken(current.value.id);
		current.value = updated;
		issuedToken.value = updated.token ?? null;
	} catch (e) {
		errorMessage.value =
			e instanceof Error ? e.message : "Failed to issue a token";
	}
}

async function copy(text: string, what: string) {
	try {
		await navigator.clipboard.writeText(text);
		toastStore.addToast(`${what} copied`, "success");
	} catch {
		toastStore.addToast(`Couldn't copy the ${what.toLowerCase()}`, "error");
	}
}

function formatDate(iso?: string): string {
	if (!iso) return "-";
	return new Date(iso).toLocaleString(undefined, {
		year: "numeric",
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card heartbeat-form-modal">
				<header class="modal-header">
					<h2>{{ title }}</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div v-if="errorMessage" class="error-banner">
					<p>{{ errorMessage }}</p>
				</div>

				<section v-if="issuedToken" class="token-issued">
					<p>
						<strong>Copy this token now.</strong> It isn't shown
						again; if it's lost, issue a new one.
					</p>
					<div class="copy-row">
						<code class="token-value">{{ issuedToken }}</code>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							@click="copy(issuedToken, 'Token')"
						>
							<AppIcon name="copy" size="14" />
							Copy
						</button>
					</div>
					<p class="hint">Example job script:</p>
					<div class="copy-row">
						<pre class="snippet">{{ curlSnippet }}</pre>
						<button
							type="button"
							class="btn btn-outline btn-sm"
							@click="copy(curlSnippet, 'Script')"
						>
							<AppIcon name="copy" size="14" />
							Copy
						</button>
					</div>
					<div class="form-actions">
						<span class="spacer"></span>
						<button
							type="button"
							class="btn btn-primary"
							@click="emit('close')"
						>
							Done
						</button>
					</div>
				</section>

				<form v-else @submit.prevent="handleSubmit">
					<div class="form-group">
						<label for="hb-name">Name</label>
						<input
							id="hb-name"
							v-model="name"
							type="text"
							placeholder="Nightly backup"
							maxlength="200"
							required
						/>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="hb-interval">Expected every</label>
							<div class="duration">
								<input
									id="hb-interval"
									v-model="intervalValue"
									type="number"
									min="1"
									step="any"
									required
								/>
								<select v-model="intervalUnit">
									<option value="minutes">minutes</option>
									<option value="hours">hours</option>
									<option value="days">days</option>
								</select>
							</div>
						</div>
						<div class="form-group">
							<label for="hb-grace">Grace</label>
							<div class="duration">
								<input
									id="hb-grace"
									v-model="graceValue"
									type="number"
									min="0"
									step="any"
								/>
								<select v-model="graceUnit">
									<option value="minutes">minutes</option>
									<option value="hours">hours</option>
									<option value="days">days</option>
								</select>
							</div>
						</div>
					</div>

					<div class="form-group">
						<label for="hb-threshold">Alert after</label>
						<div class="duration">
							<input
								id="hb-threshold"
								v-model="failThreshold"
								type="number"
								min="1"
								max="100"
								step="1"
								required
							/>
							<span class="hint"
								>failed check-in(s) in a row</span
							>
						</div>
					</div>
					<p class="hint">
						The check goes down when no check-in finishes within the
						interval plus grace, or after the failed check-ins
						above. It doesn't alert until its first check-in.
					</p>

					<fieldset v-if="current" class="token">
						<legend>Check-in token</legend>
						<p v-if="current.token_prefix" class="hint">
							<code>{{ current.token_prefix }}…</code>, issued
							{{ formatDate(current.token_created_at) }}
						</p>
						<p v-else class="hint">No token issued.</p>
						<div>
							<button
								type="button"
								class="btn btn-outline btn-sm"
								@click="rotateToken"
							>
								Issue new token
							</button>
						</div>
					</fieldset>

					<div class="form-actions mt-4">
						<span class="spacer"></span>
						<button
							type="button"
							class="btn btn-outline"
							@click="emit('close')"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="isSubmitting"
						>
							{{
								isSubmitting
									? "Saving..."
									: isEdit
										? "Save Changes"
										: "Add Check-in"
							}}
						</button>
					</div>
				</form>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.heartbeat-form-modal {
	max-width: 640px;
}

form,
.token-issued {
	display: flex;
	flex-direction: column;
	gap: 12px;
}

.hint {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}

.duration {
	display: flex;
	align-items: center;
	gap: 8px;
}

.duration input {
	max-width: 110px;
}

fieldset {
	border: 1px solid var(--border-color);
	border-radius: 6px;
	padding: 12px;
	display: flex;
	flex-direction: column;
	gap: 8px;
}

legend {
	font-size: 13px;
	font-weight: 600;
	padding: 0 4px;
	color: var(--text-heading);
}

.copy-row {
	display: flex;
	align-items: flex-start;
	gap: 8px;
}

.token-value,
.snippet {
	flex: 1;
	min-width: 0;
	margin: 0;
	padding: 8px 10px;
	border-radius: 6px;
	background: var(--bg-body);
	border: 1px solid var(--border-color);
	font-size: 12px;
	word-break: break-all;
	white-space: pre-wrap;
}

.form-actions {
	display: flex;
	align-items: center;
	gap: 8px;
}

.spacer {
	flex: 1;
}
</style>
