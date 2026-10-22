<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useMonitorStore } from "../../stores/monitor";
import { useDataStore } from "../../stores/data";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import AppIcon from "../AppIcon.vue";
import type {
	MonitorCheckRule,
	MonitorCheckRuleMatch,
	MonitorCheckRulePayload,
} from "../../types";

interface Props {
	visible: boolean;
}

const props = defineProps<Props>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const monitorStore = useMonitorStore();
const dataStore = useDataStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

const matchType = ref<"server" | "host">("server");
const serverId = ref("");
const pattern = ref("");
const note = ref("");
const pauseExisting = ref(false);
const matches = ref<MonitorCheckRuleMatch[] | null>(null);
const previewError = ref("");
const saving = ref(false);
const busyId = ref<string | null>(null);

const servers = computed(() =>
	[...dataStore.servers].sort((a, b) => a.name.localeCompare(b.name)),
);

const payload = computed<MonitorCheckRulePayload | null>(() => {
	const value =
		matchType.value === "server" ? serverId.value : pattern.value.trim();
	if (!value) return null;
	return { match_type: matchType.value, value };
});

/** Running checks the new rule would pause with "pause existing". */
const running = computed(() => matches.value?.filter((m) => m.enabled) ?? []);

watch(
	() => props.visible,
	(visible) => {
		if (!visible) return;
		reset();
		monitorStore
			.fetchRules()
			.catch((e) =>
				toastStore.addToast(
					e instanceof Error ? e.message : "Failed to load rules",
					"error",
				),
			);
	},
);

// Preview what the rule matches as it is typed.
let previewTimer: ReturnType<typeof setTimeout> | undefined;
watch(payload, (p) => {
	clearTimeout(previewTimer);
	matches.value = null;
	previewError.value = "";
	if (!p) return;
	previewTimer = setTimeout(async () => {
		try {
			const result = await monitorStore.previewRule(p);
			if (payload.value === p) matches.value = result;
		} catch (e) {
			if (payload.value === p) {
				previewError.value =
					e instanceof Error ? e.message : "Invalid rule";
			}
		}
	}, 300);
});

function reset() {
	matchType.value = "server";
	serverId.value = "";
	pattern.value = "";
	note.value = "";
	pauseExisting.value = false;
}

function describe(r: MonitorCheckRule): string {
	if (r.match_type === "server") return r.server_name || r.value;
	return r.value;
}

async function handleSubmit() {
	if (!payload.value) return;
	saving.value = true;
	try {
		const rule = await monitorStore.createRule({
			...payload.value,
			note: note.value.trim() || undefined,
			pause_existing: pauseExisting.value,
		});
		toastStore.addToast(
			rule.paused_checks
				? `Rule added; ${rule.paused_checks} check(s) paused`
				: "Rule added",
			"success",
		);
		reset();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to add the rule",
			"error",
		);
	} finally {
		saving.value = false;
	}
}

async function remove(r: MonitorCheckRule) {
	if (
		!(await confirm(
			`Delete the rule for ${describe(r)}? Checks it paused stay paused.`,
			{ confirmLabel: "Delete", danger: true },
		))
	) {
		return;
	}
	busyId.value = r.id;
	try {
		await monitorStore.deleteRule(r.id);
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to delete the rule",
			"error",
		);
	} finally {
		busyId.value = null;
	}
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card rules-modal">
				<header class="modal-header">
					<h2>New check rules</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<p class="hint">
					A new site on a server, or whose domain matches a pattern,
					gets its default check created paused. Rules don't change
					existing checks, and a paused check can be resumed as usual.
				</p>

				<p v-if="monitorStore.rules.length === 0" class="hint">
					No rules yet.
				</p>
				<div v-else class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Matches</th>
								<th>Holds paused</th>
								<th>Added</th>
								<th v-if="authStore.canEdit"></th>
							</tr>
						</thead>
						<tbody>
							<tr v-for="r in monitorStore.rules" :key="r.id">
								<td>
									<span class="sub-text text-xs">
										{{
											r.match_type === "server"
												? "Server"
												: "Host"
										}}
									</span>
									<code v-if="r.match_type === 'host'">{{
										r.value
									}}</code>
									<span v-else class="font-medium">{{
										describe(r)
									}}</span>
									<div v-if="r.note" class="hint">
										{{ r.note }}
									</div>
								</td>
								<td>{{ r.paused_checks }}</td>
								<td class="sub-text">
									{{
										new Date(
											r.created_at,
										).toLocaleDateString()
									}}
									<template v-if="r.created_by">
										by {{ r.created_by }}
									</template>
								</td>
								<td v-if="authStore.canEdit" class="text-right">
									<button
										class="icon-btn"
										title="Delete rule"
										:disabled="busyId === r.id"
										@click="remove(r)"
									>
										<AppIcon name="trash" size="14" />
									</button>
								</td>
							</tr>
						</tbody>
					</table>
				</div>

				<form v-if="authStore.canEdit" @submit.prevent="handleSubmit">
					<fieldset>
						<legend>Add a rule</legend>
						<div class="form-row">
							<div class="form-group">
								<label for="rule-type">Match</label>
								<select id="rule-type" v-model="matchType">
									<option value="server">Server</option>
									<option value="host">Host pattern</option>
								</select>
							</div>
							<div
								v-if="matchType === 'server'"
								class="form-group"
							>
								<label for="rule-server">Server</label>
								<select
									id="rule-server"
									v-model="serverId"
									required
								>
									<option value="" disabled>
										Choose a server
									</option>
									<option
										v-for="s in servers"
										:key="s.id"
										:value="s.id"
									>
										{{ s.name }}
									</option>
								</select>
							</div>
							<div v-else class="form-group">
								<label for="rule-pattern">Pattern</label>
								<input
									id="rule-pattern"
									v-model="pattern"
									type="text"
									placeholder="*.staging.*"
									required
								/>
							</div>
						</div>
						<p v-if="matchType === 'host'" class="hint">
							<code>*</code> matches any characters, so
							<code>*.staging.*</code> matches
							shop.staging.example.fi.
						</p>
						<div class="form-group">
							<label for="rule-note">Note</label>
							<input
								id="rule-note"
								v-model="note"
								type="text"
								placeholder="Staging sites"
								maxlength="200"
							/>
						</div>

						<p v-if="previewError" class="hint error-text">
							{{ previewError }}
						</p>
						<template v-else-if="matches">
							<p class="hint">
								Matches {{ matches.length }} existing
								site(s)<template v-if="matches.length"
									>, {{ running.length }} of them
									running</template
								>.
							</p>
							<ul v-if="matches.length" class="match-list">
								<li v-for="m in matches" :key="m.check_id">
									{{ m.domain }}
									<span v-if="!m.enabled" class="sub-text">
										(paused)
									</span>
								</li>
							</ul>
							<label v-if="running.length" class="checkbox">
								<input
									v-model="pauseExisting"
									type="checkbox"
								/>
								Also pause the {{ running.length }} running
								check(s) now
							</label>
						</template>

						<div class="form-actions">
							<span class="spacer"></span>
							<button
								type="submit"
								class="btn btn-primary btn-sm"
								:disabled="!payload || saving || !!previewError"
							>
								Add rule
							</button>
						</div>
					</fieldset>
				</form>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.rules-modal {
	max-width: 720px;
	display: flex;
	flex-direction: column;
	gap: 12px;
}

.hint {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}

.error-text {
	color: var(--error-text);
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

.match-list {
	margin: 0;
	padding-left: 18px;
	max-height: 160px;
	overflow-y: auto;
	font-size: 13px;
}

.checkbox {
	display: flex;
	align-items: center;
	gap: 6px;
	font-size: 13px;
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
