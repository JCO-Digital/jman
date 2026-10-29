<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useVulnStore } from "../../stores/vulns";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import { useRouteSection } from "../../composables/useRouteSection";
import LoadingSpinner from "../LoadingSpinner.vue";
import TaskFormModal from "../TaskFormModal.vue";
import SectionBar from "./SectionBar.vue";
import PanelToolbar from "./PanelToolbar.vue";
import EmptyState from "./EmptyState.vue";
import VulnGroupCard from "./VulnGroupCard.vue";
import VulnMuteModal from "./VulnMuteModal.vue";
import { exposureTaskDefaults, formatDate, type MuteTarget } from "./vuln";
import type { Section } from "./sections";
import type {
	VulnExposureGroup,
	VulnExposureSite,
	VulnMute,
} from "../../types";

type SectionKey = "open" | "resolved" | "muted";
const sectionKeys: SectionKey[] = ["open", "resolved", "muted"];

const vulnStore = useVulnStore();
const authStore = useAuthStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

const section = useRouteSection(sectionKeys, "open");
const sections = computed<Section<SectionKey>[]>(() => [
	{ key: "open", label: "Open", count: vulnStore.summary?.overdue ?? 0 },
	{ key: "resolved", label: "Resolved" },
	{
		key: "muted",
		label: "Muted",
		count: vulnStore.summary?.needs_review ?? 0,
	},
]);

const loading = ref(false);
const search = ref("");
const fixFilter = ref<"" | "fix_available" | "no_fix">("");
const closedOnly = ref(false);
const canMute = computed(() => authStore.canExecute);

const muteTarget = ref<MuteTarget | null>(null);
const taskDefaults = ref<ReturnType<typeof exposureTaskDefaults> | null>(null);

onMounted(refresh);
watch(section, () => refresh());

async function refresh() {
	loading.value = true;
	try {
		switch (section.value) {
			case "open":
				await Promise.all([
					vulnStore.fetchOpenGroups(),
					vulnStore.fetchSummary(),
				]);
				break;
			case "resolved":
				await vulnStore.fetchResolvedGroups();
				break;
			case "muted":
				await Promise.all([
					vulnStore.fetchMutes(),
					vulnStore.fetchSummary(),
				]);
				break;
		}
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to load vulnerabilities",
			"error",
		);
	} finally {
		loading.value = false;
	}
}

defineExpose({ refresh });

/** Groups with only the sites matching the search and filters. */
function filtered(groups: VulnExposureGroup[]): VulnExposureGroup[] {
	const q = search.value.trim().toLowerCase();
	const result: VulnExposureGroup[] = [];
	for (const g of groups) {
		if (closedOnly.value && !g.closed) continue;
		const groupMatches =
			!q ||
			g.name.toLowerCase().includes(q) ||
			g.slug.toLowerCase().includes(q);
		const sites = g.sites.filter(
			(s) =>
				(!fixFilter.value || s.fix_state === fixFilter.value) &&
				(groupMatches || s.site_name.toLowerCase().includes(q)),
		);
		if (sites.length) result.push({ ...g, sites });
	}
	return result;
}

const openGroups = computed(() => filtered(vulnStore.openGroups));
const resolvedGroups = computed(() => filtered(vulnStore.resolvedGroups));

function groupKey(g: VulnExposureGroup) {
	return `${g.kind}:${g.slug}:${g.version ?? ""}`;
}

function openMute(g: VulnExposureGroup, s: VulnExposureSite, mute?: VulnMute) {
	muteTarget.value = {
		kind: g.kind,
		slug: g.slug,
		siteId: s.site_id,
		name: g.name,
		siteName: s.site_name,
		fixState: s.fix_state,
		fixedIn: s.fixed_in,
		mute,
	};
}

function confirmFromList(m: VulnMute) {
	muteTarget.value = {
		kind: m.kind,
		slug: m.slug,
		siteId: m.site_id,
		name: m.name || m.slug,
		siteName: m.site_name || m.site_id,
		fixState: m.fix_state_at_mute,
		mute: m,
	};
}

async function unmute(m: VulnMute, name: string, siteName: string) {
	if (
		!(await confirm(
			`Unmute ${name} on ${siteName}? It counts as overdue again once its fix has been available long enough.`,
			{ confirmLabel: "Unmute" },
		))
	) {
		return;
	}
	try {
		await vulnStore.unmute(m.id);
		await refresh();
	} catch (e) {
		toastStore.addToast(
			e instanceof Error ? e.message : "Failed to unmute",
			"error",
		);
	}
}

async function afterMute() {
	muteTarget.value = null;
	await refresh();
}
</script>

<template>
	<div>
		<SectionBar v-model="section" :sections="sections">
			<LoadingSpinner v-if="loading" small />
		</SectionBar>

		<!-- Open exposures -->
		<template v-if="section === 'open'">
			<PanelToolbar>
				<input
					v-model="search"
					type="search"
					class="input-field"
					placeholder="Search plugins or sites"
				/>
				<select v-model="fixFilter" class="input-field">
					<option value="">All</option>
					<option value="fix_available">Fix available</option>
					<option value="no_fix">No fix</option>
				</select>
				<label class="checkbox-label">
					<input v-model="closedOnly" type="checkbox" />
					Closed plugins only
				</label>
			</PanelToolbar>

			<EmptyState
				v-if="!loading && vulnStore.openGroups.length === 0"
				class="card"
				icon="✅"
				title="No known vulnerabilities"
			>
				No site runs a plugin or core version with a known
				vulnerability.
			</EmptyState>
			<p
				v-else-if="!loading && openGroups.length === 0"
				class="card sub-text empty"
			>
				Nothing matches the filters.
			</p>
			<VulnGroupCard
				v-for="g in openGroups"
				:key="groupKey(g)"
				:group="g"
				:can-mute="canMute"
				:initially-open="g.overdue_count > 0"
				@mute="(s) => openMute(g, s)"
				@confirm="(s, m) => openMute(g, s, m)"
				@unmute="(s, m) => unmute(m, g.name, s.site_name)"
				@task="(s) => (taskDefaults = exposureTaskDefaults(g, s))"
			/>
		</template>

		<!-- Resolved in the last 30 days -->
		<template v-else-if="section === 'resolved'">
			<p class="sub-text section-hint">
				Sites that stopped running a vulnerable version in the last 30
				days: updated, removed, or covered by an ignore entry or
				backport rule.
			</p>
			<p
				v-if="!loading && resolvedGroups.length === 0"
				class="card sub-text empty"
			>
				Nothing resolved in the last 30 days.
			</p>
			<VulnGroupCard
				v-for="g in resolvedGroups"
				:key="groupKey(g)"
				:group="g"
				:can-mute="false"
				resolved
			/>
		</template>

		<!-- Mutes -->
		<template v-else-if="section === 'muted'">
			<div class="card">
				<p class="sub-text section-hint">
					A muted plugin doesn't turn overdue on its site. A mute is
					lifted when a new vulnerability applies, when a no-fix
					plugin gets a fix or a possibly fixing version, or when the
					site is no longer affected, and needs review after 90 days.
				</p>
				<p
					v-if="!loading && vulnStore.openMutes.length === 0"
					class="sub-text empty"
				>
					Nothing is muted.
				</p>
				<div v-else class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Plugin</th>
								<th>Site</th>
								<th>Reason</th>
								<th>Muted</th>
								<th>Review</th>
								<th v-if="canMute" class="text-right">
									Actions
								</th>
							</tr>
						</thead>
						<tbody>
							<tr v-for="m in vulnStore.openMutes" :key="m.id">
								<td>
									<span class="font-medium">{{
										m.name || m.slug
									}}</span>
									<code v-if="m.version" class="version">{{
										m.version
									}}</code>
									<div class="sub-text">
										{{
											m.fix_state_at_mute ===
											"fix_available"
												? "Fix was available"
												: "No fix"
										}}
									</div>
								</td>
								<td>
									<RouterLink :to="`/site/${m.site_id}`">{{
										m.site_name || m.site_id
									}}</RouterLink>
								</td>
								<td>{{ m.reason }}</td>
								<td class="sub-text">
									{{ formatDate(m.updated_at) }} by
									{{ m.updated_by || m.created_by }}
								</td>
								<td>
									<span
										v-if="m.status === 'needs_review'"
										class="status-badge badge-sm warning"
										>Needs review</span
									>
									<span v-else class="sub-text">{{
										formatDate(m.review_at)
									}}</span>
								</td>
								<td v-if="canMute" class="text-right">
									<div class="actions-group">
										<button
											class="btn btn-sm btn-outline"
											@click="confirmFromList(m)"
										>
											Keep muted
										</button>
										<button
											class="btn btn-sm btn-outline"
											@click="
												unmute(
													m,
													m.name || m.slug,
													m.site_name || m.site_id,
												)
											"
										>
											Unmute
										</button>
									</div>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</div>

			<div v-if="vulnStore.liftedMutes.length" class="card">
				<h3 class="sub-heading">Lifted in the last 30 days</h3>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Plugin</th>
								<th>Site</th>
								<th>Reason muted</th>
								<th>Lifted</th>
							</tr>
						</thead>
						<tbody>
							<tr v-for="m in vulnStore.liftedMutes" :key="m.id">
								<td class="font-medium">
									{{ m.name || m.slug }}
								</td>
								<td>
									<RouterLink :to="`/site/${m.site_id}`">{{
										m.site_name || m.site_id
									}}</RouterLink>
								</td>
								<td class="sub-text">{{ m.reason }}</td>
								<td>
									{{ m.lifted_reason }}
									<div class="sub-text">
										{{ formatDate(m.lifted_at) }}
									</div>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</div>
		</template>

		<VulnMuteModal
			:target="muteTarget"
			@close="muteTarget = null"
			@saved="afterMute"
		/>
		<TaskFormModal
			v-if="taskDefaults"
			:defaults="taskDefaults"
			@close="taskDefaults = null"
			@saved="
				taskDefaults = null;
				toastStore.addToast('Task created', 'success');
			"
		/>
	</div>
</template>

<style scoped>
.section-hint {
	margin: 0 0 var(--space-3);
}

.empty {
	text-align: center;
	padding: var(--space-6);
}

.checkbox-label {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	font-size: 13px;
	color: var(--text-heading);
	cursor: pointer;
}

.version {
	margin-left: 6px;
}

.sub-heading {
	margin: 0 0 var(--space-3);
	font-size: 14px;
}

.card + .card {
	margin-top: var(--space-4);
}

.actions-group {
	display: inline-flex;
	gap: 6px;
}
</style>
