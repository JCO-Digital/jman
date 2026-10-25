<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useDataStore } from "../../stores/data";
import AppIcon from "../AppIcon.vue";
import type { ScanFinding } from "../../types";
import { severityClass, isStateRule } from "./scan";

interface Props {
	findings: ScanFinding[];
	/** Whether the user may triage (execute level). */
	canTriage: boolean;
	/** Shows each finding's site, for lists spanning sites. */
	showSite?: boolean;
}

const props = defineProps<Props>();

const emit = defineEmits<{
	(e: "allowlist", finding: ScanFinding): void;
	(e: "acknowledge", findings: ScanFinding[]): void;
	(e: "dismiss", findings: ScanFinding[]): void;
}>();

const dataStore = useDataStore();

const expanded = ref(new Set<number>());
const selected = ref(new Set<number>());

watch(
	() => props.findings,
	() => {
		const ids = new Set(props.findings.map((f) => f.id));
		selected.value = new Set(
			[...selected.value].filter((id) => ids.has(id)),
		);
	},
);

const openFindings = computed(() =>
	props.findings.filter((f) => f.status === "open"),
);
const selectedFindings = computed(() =>
	props.findings.filter((f) => selected.value.has(f.id)),
);
const allSelected = computed(
	() =>
		openFindings.value.length > 0 &&
		openFindings.value.every((f) => selected.value.has(f.id)),
);
const selectionIsStateRules = computed(() =>
	selectedFindings.value.every((f) => isStateRule(f.rule)),
);

function toggle(set: Set<number>, id: number) {
	if (set.has(id)) set.delete(id);
	else set.add(id);
}

function toggleAll() {
	selected.value = allSelected.value
		? new Set()
		: new Set(openFindings.value.map((f) => f.id));
}

function siteDomain(siteId: string): string {
	return dataStore.sites.find((s) => s.id === siteId)?.domain ?? siteId;
}

function formatTime(iso?: string): string {
	if (!iso) return "-";
	return new Date(iso).toLocaleString(undefined, {
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
}

function canAllowlist(f: ScanFinding): boolean {
	return !!f.sha256 && f.engine !== "api";
}
</script>

<template>
	<div>
		<div
			v-if="canTriage && selectedFindings.length"
			class="flex-row gap-2 bulk-bar"
		>
			<span class="sub-text">{{ selectedFindings.length }} selected</span>
			<button
				v-if="selectionIsStateRules"
				class="btn btn-sm btn-outline"
				title="Trust these paths on their sites, whatever the files hold"
				@click="emit('acknowledge', selectedFindings)"
			>
				Acknowledge paths
			</button>
			<button
				class="btn btn-sm btn-outline"
				title="Dismiss; a finding reported again opens again"
				@click="emit('dismiss', selectedFindings)"
			>
				Dismiss
			</button>
		</div>
		<div class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th v-if="canTriage" class="select-col">
							<input
								type="checkbox"
								:checked="allSelected"
								:disabled="!openFindings.length"
								title="Select all open findings"
								@change="toggleAll"
							/>
						</th>
						<th>Severity</th>
						<th v-if="showSite">Site</th>
						<th>Rule</th>
						<th>Path</th>
						<th class="hide-mobile">First seen</th>
						<th>Status</th>
						<th v-if="canTriage" class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<template v-for="f in findings" :key="f.id">
						<tr :class="{ 'row-muted': f.status !== 'open' }">
							<td v-if="canTriage" class="select-col">
								<input
									v-if="f.status === 'open'"
									type="checkbox"
									:checked="selected.has(f.id)"
									@change="toggle(selected, f.id)"
								/>
							</td>
							<td>
								<span
									class="status-badge badge-sm"
									:class="severityClass(f.severity)"
								>
									{{ f.severity }}
								</span>
							</td>
							<td v-if="showSite" class="font-medium">
								<RouterLink
									:to="`/site/${f.site_id}`"
									class="site-link"
								>
									{{ siteDomain(f.site_id) }}
								</RouterLink>
							</td>
							<td>
								<code>{{ f.rule }}</code>
							</td>
							<td class="path-cell">
								<button
									class="btn-text path-toggle"
									:title="
										expanded.has(f.id)
											? 'Hide details'
											: 'Show details'
									"
									@click="toggle(expanded, f.id)"
								>
									<AppIcon
										name="chevron-right"
										size="12"
										:class="{ rotated: expanded.has(f.id) }"
									/>
									<code class="path">{{
										f.path || "-"
									}}</code>
								</button>
							</td>
							<td class="hide-mobile sub-text">
								{{ formatTime(f.first_seen_at) }}
							</td>
							<td>
								<span
									class="sub-text"
									:title="
										f.status_by
											? `by ${f.status_by} ${formatTime(f.status_at)}`
											: ''
									"
								>
									{{ f.status }}
								</span>
							</td>
							<td v-if="canTriage" class="text-right">
								<div
									v-if="f.status === 'open'"
									class="actions-group"
								>
									<button
										v-if="canAllowlist(f)"
										class="btn btn-sm btn-outline"
										title="Trust this file content on every site"
										@click="emit('allowlist', f)"
									>
										Allowlist
									</button>
									<button
										v-if="isStateRule(f.rule)"
										class="btn btn-sm btn-outline"
										title="Trust this path on this site, whatever the file holds"
										@click="emit('acknowledge', [f])"
									>
										Ack path
									</button>
									<button
										class="btn btn-sm btn-text"
										title="Dismiss; reported again, it opens again"
										@click="emit('dismiss', [f])"
									>
										Dismiss
									</button>
								</div>
							</td>
						</tr>
						<tr v-if="expanded.has(f.id)" class="detail-row">
							<td :colspan="99">
								<dl class="finding-detail">
									<template v-if="f.path">
										<dt>Path</dt>
										<dd>
											<code class="break-all">{{
												f.path
											}}</code>
										</dd>
									</template>
									<template v-if="f.detail">
										<dt>Detail</dt>
										<dd class="pre-line">{{ f.detail }}</dd>
									</template>
									<template v-if="f.sha256">
										<dt>SHA-256</dt>
										<dd>
											<code class="hash">{{
												f.sha256
											}}</code>
										</dd>
									</template>
									<dt>Seen</dt>
									<dd>
										{{ formatTime(f.first_seen_at) }} –
										{{ formatTime(f.last_seen_at) }}, rule
										set {{ f.rule_set_version || "-" }},
										{{ f.engine }}
									</dd>
								</dl>
								<pre v-if="f.snippet" class="snippet">{{
									f.snippet
								}}</pre>
							</td>
						</tr>
					</template>
				</tbody>
			</table>
		</div>
	</div>
</template>

<style scoped>
.bulk-bar {
	margin-bottom: var(--space-2);
}

.select-col {
	width: 32px;
}

.row-muted {
	opacity: 0.6;
}

.path-cell {
	max-width: 480px;
}

.path-toggle {
	display: inline-flex;
	align-items: center;
	gap: 4px;
	max-width: 100%;
	padding: 0;
	background: none;
	border: none;
	cursor: pointer;
	color: inherit;
	text-align: left;

	.path {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.rotated {
		transform: rotate(90deg);
	}
}

.detail-row td {
	background: var(--bg-body);
}

.finding-detail {
	display: grid;
	grid-template-columns: max-content 1fr;
	gap: var(--space-1) var(--space-3);
	margin: 0 0 var(--space-2);
	font-size: var(--font-size-xs);

	dt {
		color: var(--text-muted);
	}

	dd {
		margin: 0;
	}
}

.pre-line {
	white-space: pre-line;
}

.break-all {
	word-break: break-all;
}

.snippet {
	max-height: 320px;
	overflow: auto;
	margin: 0;
	padding: var(--space-3);
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: var(--radius-sm);
	font-size: var(--font-size-xs);
	white-space: pre-wrap;
	word-break: break-all;
}

.actions-group {
	display: inline-flex;
	gap: 6px;
}

.site-link {
	color: var(--primary);
	font-weight: 600;
	text-decoration: none;
}
</style>
