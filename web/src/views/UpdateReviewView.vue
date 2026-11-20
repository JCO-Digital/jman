<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { useDataStore } from "../stores/data";
import { hasTextDiffFindings, useUpdateJobsStore } from "../stores/updateJobs";
import { useAuthStore } from "../stores/auth";
import type { UpdateJob } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import TextDiffReport from "../components/TextDiffReport.vue";
import AppIcon from "../components/AppIcon.vue";

/**
 * The update text diffs of the last week (docs/update-text-diff.md),
 * grouped into runs: jobs one person queued close together, like a bulk
 * update. Sites whose pages broke or changed come first; unchanged sites
 * are hidden unless asked for.
 */
const props = defineProps<{
	jobId?: string;
}>();

const dataStore = useDataStore();
const jobsStore = useUpdateJobsStore();
const authStore = useAuthStore();

/** Jobs queued by one person within this long of each other are one run. */
const RUN_GAP_MS = 15 * 60 * 1000;

const jobs = ref<UpdateJob[]>([]);
const loading = ref(true);
const error = ref("");
const showUnchanged = ref(false);
const onlyMine = ref(false);
/** Expanded jobs, with their full diff once loaded. */
const expanded = ref(new Map<number, UpdateJob | null>());

async function load() {
	error.value = "";
	try {
		jobs.value = await jobsStore.fetchTextDiffJobs();
	} catch (e: any) {
		error.value = e.message || "Failed to load update checks";
	} finally {
		loading.value = false;
	}
	// Refresh expanded diffs that were still running.
	for (const [id, job] of expanded.value) {
		if (job?.text_diff?.status === "running") void loadJob(id);
	}
}

async function loadJob(id: number) {
	try {
		const job = await jobsStore.fetchJob(id);
		expanded.value = new Map(expanded.value).set(id, job);
	} catch (e: any) {
		error.value = e.message || "Failed to load the update check";
	}
}

function toggle(id: number) {
	const next = new Map(expanded.value);
	if (next.has(id)) {
		next.delete(id);
	} else {
		next.set(id, null);
		void loadJob(id);
	}
	expanded.value = next;
}

onMounted(async () => {
	await load();
	if (props.jobId) await openJob(Number(props.jobId));
});

watch(
	() => props.jobId,
	(id) => {
		if (id) void openJob(Number(id));
	},
);

async function openJob(id: number) {
	if (!expanded.value.has(id)) toggle(id);
	await nextTick();
	document
		.getElementById(`update-job-${id}`)
		?.scrollIntoView({ block: "start", behavior: "smooth" });
}

// Reload while checks run, and when jobs finish in this tab.
watch(
	() => [
		jobsStore.busyJobs.length,
		jobsStore.jobs.map((j) => j.text_diff_summary?.status).join(),
	],
	() => void load(),
);

interface Run {
	key: string;
	by: string;
	start: number;
	jobs: UpdateJob[];
}

const visibleJobs = computed(() =>
	jobs.value.filter((j) => {
		if (onlyMine.value && j.created_by !== authStore.user?.username) {
			return false;
		}
		if (showUnchanged.value || String(j.id) === props.jobId) return true;
		return hasTextDiffFindings(j) || j.text_diff_summary?.status !== "done";
	}),
);

/** Visible jobs grouped into runs, newest run first. */
const runs = computed<Run[]>(() => {
	const out: Run[] = [];
	const last = new Map<string, Run>();
	// jobs are newest first; a run grows backwards in time.
	for (const job of visibleJobs.value) {
		const at = Date.parse(job.created_at);
		const run = last.get(job.created_by);
		if (run && run.start - at <= RUN_GAP_MS) {
			run.jobs.push(job);
			run.start = at;
			continue;
		}
		const fresh: Run = {
			key: `${job.created_by}-${job.id}`,
			by: job.created_by,
			start: at,
			jobs: [job],
		};
		out.push(fresh);
		last.set(job.created_by, fresh);
	}
	for (const run of out) run.jobs.sort(compareJobs);
	return out;
});

const hiddenCount = computed(
	() => jobs.value.length - visibleJobs.value.length,
);

/** Broken pages first, then changed, then the rest. */
function rank(job: UpdateJob) {
	const s = job.text_diff_summary;
	if (!s) return 4;
	if (s.status === "running") return 0;
	if (s.errors) return 1;
	if (s.changed) return 2;
	if (s.no_baseline || s.status === "interrupted") return 3;
	return 4;
}

function compareJobs(a: UpdateJob, b: UpdateJob) {
	return rank(a) - rank(b) || domain(a).localeCompare(domain(b));
}

function domain(job: UpdateJob) {
	return dataStore.getSiteById(job.site_id)?.domain ?? job.site_id;
}

function runSummary(run: Run) {
	const count = (f: (j: UpdateJob) => boolean) => run.jobs.filter(f).length;
	const parts = [
		`${run.jobs.length} site${run.jobs.length === 1 ? "" : "s"}`,
	];
	const broken = count((j) => !!j.text_diff_summary?.errors);
	const changed = count(
		(j) => !j.text_diff_summary?.errors && !!j.text_diff_summary?.changed,
	);
	if (broken) parts.push(`${broken} broken`);
	if (changed) parts.push(`${changed} changed`);
	return parts.join(" · ");
}

/** What the job changed. */
function what(job: UpdateJob): string {
	if (job.kind === "core") {
		const r = job.results[0];
		return r?.new_version && r.old_version !== r.new_version
			? `WordPress ${r.old_version} → ${r.new_version}`
			: "WordPress core";
	}
	if (job.kind === "install") {
		return job.payload.plugin
			? `${job.payload.plugin} from ZIP`
			: `Install ${job.payload.source}`;
	}
	if (job.kind === "mu_install") {
		return `Mu-plugin ${job.payload.file}`;
	}
	if ("plugins" in job.payload) {
		const updated = job.results.filter((r) => r.status === "Updated");
		const list = updated.length
			? updated.map(
					(r) => `${r.name} ${r.old_version} → ${r.new_version}`,
				)
			: job.payload.plugins.map((p) => p.name);
		return list.join(", ");
	}
	return job.kind;
}

function badges(job: UpdateJob): { label: string; cls: string }[] {
	const s = job.text_diff_summary;
	if (!s) return [];
	if (s.status === "running") return [{ label: "Checking…", cls: "info" }];
	const out: { label: string; cls: string }[] = [];
	if (s.status === "interrupted") {
		out.push({ label: "Interrupted", cls: "" });
	}
	if (s.errors) out.push({ label: `${s.errors} broken`, cls: "error" });
	if (s.changed) out.push({ label: `${s.changed} changed`, cls: "warning" });
	if (s.no_baseline) {
		out.push({ label: `${s.no_baseline} not checked`, cls: "" });
	}
	if (out.length === 0) out.push({ label: "Unchanged", cls: "success" });
	return out;
}

function formatTime(ms: number) {
	return new Date(ms).toLocaleString(undefined, {
		dateStyle: "medium",
		timeStyle: "short",
	});
}
</script>

<template>
	<div class="update-review">
		<ViewHeader title="Update review">
			<template #actions>
				<label class="checkbox-label">
					<input v-model="onlyMine" type="checkbox" />
					Only mine
				</label>
				<label class="checkbox-label">
					<input v-model="showUnchanged" type="checkbox" />
					Show unchanged sites
				</label>
				<button class="btn btn-outline btn-sm" @click="load">
					<AppIcon name="refresh" size="14" /> Refresh
				</button>
			</template>
		</ViewHeader>

		<p class="text-muted review-intro">
			Plugin, ZIP and core updates compare each site's visible text before
			and after the update: the frontpage and the pages set on the site's
			Plugins &amp; core tab. Text that changes on its own is left out.
		</p>

		<LoadingSpinner v-if="loading" message="Loading update checks..." />
		<p v-else-if="error && !jobs.length" class="form-error">
			{{ error }}
		</p>
		<p v-else-if="!runs.length" class="card text-muted">
			Nothing to review from the last 7 days<span v-if="hiddenCount"
				>: {{ hiddenCount }} update{{
					hiddenCount === 1 ? "" : "s"
				}}
				left every page unchanged</span
			>.
		</p>

		<section v-for="run in runs" :key="run.key" class="card review-run">
			<header class="review-run-header">
				<h2>{{ formatTime(run.start) }}</h2>
				<span class="text-muted"
					>by {{ run.by || "—" }} · {{ runSummary(run) }}</span
				>
			</header>
			<ul class="review-jobs">
				<li
					v-for="job in run.jobs"
					:id="`update-job-${job.id}`"
					:key="job.id"
					class="review-job"
				>
					<button
						class="review-job-row"
						:aria-expanded="expanded.has(job.id)"
						@click="toggle(job.id)"
					>
						<AppIcon
							:name="
								expanded.has(job.id)
									? 'chevron-down'
									: 'chevron-right'
							"
							size="16"
						/>
						<span class="review-job-site">{{ domain(job) }}</span>
						<span class="review-job-what sub-text">{{
							what(job)
						}}</span>
						<span class="review-job-badges">
							<span
								v-for="b in badges(job)"
								:key="b.label"
								:class="['status-badge', 'badge-sm', b.cls]"
								>{{ b.label }}</span
							>
						</span>
					</button>
					<div v-if="expanded.has(job.id)" class="review-job-body">
						<LoadingSpinner
							v-if="!expanded.get(job.id)?.text_diff"
							message="Loading..."
						/>
						<TextDiffReport
							v-else
							:diff="expanded.get(job.id)!.text_diff!"
							:site-id="job.site_id"
							:domain="dataStore.getSiteById(job.site_id)?.domain"
						/>
						<RouterLink
							:to="{
								name: 'site-detail',
								params: { id: job.site_id, tab: 'updates' },
							}"
							class="review-site-link"
							>Site's compared pages and mutes</RouterLink
						>
					</div>
				</li>
			</ul>
		</section>
	</div>
</template>
