<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useMonitorStore } from "../../stores/monitor";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import AppIcon from "../AppIcon.vue";
import LoadingSpinner from "../LoadingSpinner.vue";
import PanelToolbar from "./PanelToolbar.vue";
import MonitorRuleFormModal from "./MonitorRuleFormModal.vue";
import type { MonitorCheckRule } from "../../types";

const monitorStore = useMonitorStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

const loading = ref(false);
const formVisible = ref(false);
const busyId = ref<string | null>(null);

onMounted(refresh);

async function refresh() {
	loading.value = true;
	try {
		await monitorStore.fetchRules();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load rules",
			"error",
		);
	} finally {
		loading.value = false;
	}
}

defineExpose({ refresh });

function describe(r: MonitorCheckRule): string {
	if (r.match_type === "server") return r.server_name || r.value;
	return r.value;
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
	<div class="card">
		<PanelToolbar>
			<p class="sub-text section-hint">
				A new site on a server, or whose domain matches a pattern, gets
				its default check created paused. Rules don't change existing
				checks, and a paused check can be resumed as usual.
			</p>
			<template #actions>
				<button
					v-if="authStore.canEdit"
					class="btn btn-primary btn-sm"
					@click="formVisible = true"
				>
					Add Rule
				</button>
			</template>
		</PanelToolbar>

		<div
			v-if="loading && !monitorStore.rules.length"
			class="loading-container"
		>
			<LoadingSpinner />
		</div>
		<p v-else-if="monitorStore.rules.length === 0" class="sub-text empty">
			No rules yet.
		</p>
		<div v-else class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Matches</th>
						<th>Note</th>
						<th>Holds paused</th>
						<th>Added</th>
						<th v-if="authStore.canEdit"></th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="r in monitorStore.rules" :key="r.id">
						<td>
							<span class="sub-text text-xs match-type">
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
						</td>
						<td class="sub-text">{{ r.note || "-" }}</td>
						<td>{{ r.paused_checks }}</td>
						<td class="sub-text">
							{{ new Date(r.created_at).toLocaleDateString() }}
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

		<MonitorRuleFormModal
			:visible="formVisible"
			@close="formVisible = false"
		/>
	</div>
</template>

<style scoped>
.section-hint {
	margin: 0;
	flex: 1 1 320px;
}

.empty {
	text-align: center;
	padding: var(--space-6);
}

.match-type {
	margin-right: 6px;
}

.text-xs {
	font-size: 11px;
}
</style>
