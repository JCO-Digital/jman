<script setup lang="ts">
import { computed, ref } from "vue";
import AppIcon from "../AppIcon.vue";
import { severityClass } from "./scan";
import { closedLabel, cvssSeverity, formatCvss, formatDate } from "./vuln";
import type {
	VulnExposureGroup,
	VulnExposureSite,
	VulnMute,
} from "../../types";

const props = defineProps<{
	group: VulnExposureGroup;
	/** May mute, confirm and unmute (execute level). */
	canMute: boolean;
	/** Resolved exposures: no due dates or actions. */
	resolved?: boolean;
	initiallyOpen?: boolean;
}>();

const emit = defineEmits<{
	(e: "mute", site: VulnExposureSite): void;
	(e: "confirm", site: VulnExposureSite, mute: VulnMute): void;
	(e: "unmute", site: VulnExposureSite, mute: VulnMute): void;
	(e: "task", site: VulnExposureSite): void;
}>();

const open = ref(props.initiallyOpen ?? false);
const showMuted = ref(false);

const unmutedSites = computed(() => props.group.sites.filter((s) => !s.mute));
const mutedSites = computed(() => props.group.sites.filter((s) => s.mute));
const visibleSites = computed(() =>
	showMuted.value || props.resolved ? props.group.sites : unmutedSites.value,
);

/** "Fix available", "No fix", or both counts when the sites differ. */
const fixLabel = computed(() => {
	const g = props.group;
	if (!g.no_fix_count) return { text: "Fix available", cls: "fix" };
	if (!g.fix_available_count) return { text: "No fix", cls: "nofix" };
	return {
		text: `${g.fix_available_count} fixable · ${g.no_fix_count} no fix`,
		cls: "nofix",
	};
});

function siteCount(n: number) {
	return `${n} site${n === 1 ? "" : "s"}`;
}
</script>

<template>
	<div class="card group-card" :class="{ overdue: group.overdue_count > 0 }">
		<div class="group-head">
			<button class="group-toggle" @click="open = !open">
				<AppIcon
					name="chevron-right"
					size="14"
					:class="{ rotated: open }"
				/>
				<span
					class="status-badge badge-sm"
					:class="severityClass(cvssSeverity(group.max_cvss))"
				>
					{{ formatCvss(group.max_cvss) }}
				</span>
				<span class="font-medium">{{ group.name }}</span>
				<span v-if="group.kind === 'plugin'" class="sub-text">{{
					group.slug
				}}</span>
			</button>
			<span v-if="!resolved" class="label" :class="fixLabel.cls">{{
				fixLabel.text
			}}</span>
			<span
				v-if="group.closed"
				class="label closed"
				:title="
					group.closed_date
						? `Closed on wordpress.org ${group.closed_date}`
						: 'Closed on wordpress.org'
				"
				>{{ closedLabel(group) }}</span
			>
			<span class="spacer"></span>
			<span class="sub-text">{{ siteCount(group.sites.length) }}</span>
			<span v-if="group.overdue_count" class="label overdue-label">
				{{ group.overdue_count }} overdue
			</span>
			<span v-if="group.muted_count && !resolved" class="sub-text">
				{{ group.muted_count }} muted
			</span>
		</div>

		<div v-if="open" class="group-body">
			<p v-if="group.closed && group.last_updated" class="sub-text note">
				Closed on wordpress.org{{
					group.closed_date ? ` on ${group.closed_date}` : ""
				}}; last updated {{ formatDate(group.last_updated) }}. A fix is
				unlikely; plan to remove or replace it.
			</p>

			<ul class="vuln-list">
				<li v-for="v in group.vulnerabilities" :key="v.uuid">
					<span
						class="status-badge badge-sm"
						:class="severityClass(cvssSeverity(v.cvss))"
						>{{ v.cvss == null ? "-" : v.cvss.toFixed(1) }}</span
					>
					<a
						v-if="v.link"
						:href="v.link"
						target="_blank"
						rel="noopener"
						>{{ v.name }}</a
					>
					<span v-else>{{ v.name }}</span>
					<span class="sub-text">
						{{
							v.fixed_in
								? `fixed in ${v.fixed_in}`
								: v.unfixed
									? "unfixed"
									: ""
						}}
					</span>
				</li>
			</ul>

			<div class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Site</th>
							<th>Version</th>
							<th>Fix</th>
							<th>{{ resolved ? "Resolved" : "Due" }}</th>
							<th v-if="!resolved" class="text-right">Actions</th>
						</tr>
					</thead>
					<tbody>
						<tr
							v-for="s in visibleSites"
							:key="s.id"
							:class="{
								'row-overdue': s.overdue && !s.mute,
								'row-muted': s.mute,
							}"
						>
							<td>
								<RouterLink
									:to="`/site/${s.site_id}`"
									class="font-medium"
									>{{ s.site_name }}</RouterLink
								>
								<div v-if="s.mute" class="sub-text mute-note">
									<span
										v-if="s.mute.status === 'needs_review'"
										class="status-badge badge-sm warning"
										>Needs review</span
									>
									Muted by {{ s.mute.created_by }}:
									{{ s.mute.reason }}
								</div>
							</td>
							<td>
								<code>{{ s.version }}</code>
							</td>
							<td>
								<template
									v-if="s.fix_state === 'fix_available'"
								>
									Update to <code>{{ s.fixed_in }}</code>
								</template>
								<template v-else>
									<span class="sub-text">No fix</span>
									<div
										v-if="s.possible_fix_version"
										class="hint-note"
										:title="`Version ${s.possible_fix_version} is outside every affected range, but no fix has been confirmed`"
									>
										{{ s.possible_fix_version }} available,
										might fix it
									</div>
								</template>
							</td>
							<td>
								<template v-if="resolved">{{
									formatDate(s.resolved_at)
								}}</template>
								<template v-else-if="s.due_at">
									<span
										:class="{
											'overdue-text':
												s.overdue && !s.mute,
										}"
										>{{
											s.overdue ? "Overdue since" : "Due"
										}}
										{{ formatDate(s.due_at) }}</span
									>
								</template>
								<span v-else class="sub-text">-</span>
							</td>
							<td v-if="!resolved" class="text-right">
								<div class="actions-group">
									<template v-if="canMute">
										<button
											v-if="!s.mute"
											class="btn btn-sm btn-outline"
											@click="emit('mute', s)"
										>
											Mute
										</button>
										<template v-else>
											<button
												class="btn btn-sm btn-outline"
												@click="
													emit('confirm', s, s.mute)
												"
											>
												Keep muted
											</button>
											<button
												class="btn btn-sm btn-outline"
												@click="
													emit('unmute', s, s.mute)
												"
											>
												Unmute
											</button>
										</template>
									</template>
									<button
										class="btn btn-sm btn-text"
										title="Create a task for this site"
										@click="emit('task', s)"
									>
										Task
									</button>
								</div>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
			<button
				v-if="!resolved && mutedSites.length"
				class="btn btn-text btn-sm"
				@click="showMuted = !showMuted"
			>
				{{ showMuted ? "Hide" : "Show" }} {{ mutedSites.length }} muted
				site{{ mutedSites.length === 1 ? "" : "s" }}
			</button>
		</div>
	</div>
</template>

<style scoped>
.group-card {
	margin-bottom: var(--space-3);
}

.group-card.overdue {
	border-left: 3px solid var(--error-border);
}

.group-head {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--space-3);
}

.group-toggle {
	display: inline-flex;
	align-items: center;
	gap: var(--space-2);
	padding: 0;
	background: none;
	border: none;
	color: var(--text-heading);
	cursor: pointer;

	.rotated {
		transform: rotate(90deg);
	}
}

.spacer {
	flex: 1;
}

.label {
	padding: 2px 8px;
	border-radius: 999px;
	font-size: 12px;
	font-weight: 600;
	white-space: nowrap;
}

.label.fix {
	background: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.label.nofix {
	background: var(--warning-bg);
	color: var(--warning-text);
}

.label.closed {
	background: var(--badge-default-bg);
	color: var(--badge-default-text);
	border: 1px solid var(--border-color);
}

.overdue-label {
	background: var(--error-bg);
	color: var(--error-text);
}

.group-body {
	margin-top: var(--space-3);
	display: flex;
	flex-direction: column;
	gap: var(--space-3);
	align-items: flex-start;
}

.group-body > .table-container {
	align-self: stretch;
}

.note {
	margin: 0;
}

.vuln-list {
	list-style: none;
	margin: 0;
	padding: 0;
	display: flex;
	flex-direction: column;
	gap: 4px;
	font-size: 13px;

	li {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
}

.row-overdue td:first-child {
	box-shadow: inset 3px 0 0 var(--error-border);
}

.row-muted {
	opacity: 0.7;
}

.mute-note {
	margin-top: 2px;
	font-size: 12px;
}

.hint-note {
	font-size: 12px;
	color: var(--warning-text);
}

.overdue-text {
	color: var(--error-text);
	font-weight: 600;
}

.actions-group {
	display: inline-flex;
	gap: 6px;
}
</style>
