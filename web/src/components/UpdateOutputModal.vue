<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { useDataStore } from "../stores/data";
import type { UpdateJob } from "../types";
import AppIcon from "./AppIcon.vue";
import { failureCauseLabel } from "../utils/format";

// The full wp-cli output of an update job that failed, or whose update
// didn't change a version: what the short message in the task center
// leaves out (PHP notices, the whole stderr).
const props = defineProps<{
	/** The job to show; null hides the modal. */
	jobId: number | null;
}>();

const emit = defineEmits<{
	(e: "close"): void;
}>();

const jobsStore = useUpdateJobsStore();
const dataStore = useDataStore();

const job = ref<UpdateJob | null>(null);
const loading = ref(false);
const error = ref("");

watch(
	() => props.jobId,
	async (id) => {
		job.value = null;
		error.value = "";
		if (id === null) return;
		loading.value = true;
		try {
			const loaded = await jobsStore.fetchJob(id);
			if (props.jobId === id) job.value = loaded;
		} catch (e) {
			error.value =
				e instanceof Error ? e.message : "Failed to load the job";
		} finally {
			loading.value = false;
		}
	},
	{ immediate: true },
);

const siteName = computed(() =>
	job.value
		? (dataStore.getSiteById(job.value.site_id)?.domain ??
			job.value.site_id)
		: "",
);

// Results worth explaining: failures, and updates with a note (such as a
// version that didn't change).
const flagged = computed(() =>
	(job.value?.results ?? []).filter(
		(r) => r.status === "failed" || r.error || r.note,
	),
);

function seconds(ms: number): string {
	return `${(ms / 1000).toFixed(1)} s`;
}
</script>

<template>
	<Teleport to="body">
		<div
			v-if="jobId !== null"
			class="modal-overlay"
			@click.self="emit('close')"
		>
			<div class="modal-content card output-modal">
				<header class="modal-header">
					<h2>wp-cli output · {{ siteName }}</h2>
					<button
						class="modal-close"
						aria-label="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<p v-if="loading" class="text-muted">Loading…</p>
				<p v-else-if="error" class="form-error">{{ error }}</p>
				<template v-else-if="job">
					<ul v-if="flagged.length" class="output-results">
						<li v-for="r in flagged" :key="r.name">
							<strong>{{ r.name }}</strong>
							<span class="text-muted">
								{{ r.old_version || "?" }} →
								{{ r.new_version || "?" }} ({{
									r.status
								}})</span
							>
							<div
								v-if="failureCauseLabel(r)"
								class="output-cause"
							>
								{{ failureCauseLabel(r) }}
							</div>
							<div v-if="r.error">{{ r.error }}</div>
							<div v-if="r.note" class="text-muted">
								{{ r.note }}
							</div>
						</li>
					</ul>
					<p v-if="!job.calls?.length" class="text-muted">
						This job kept no output.
					</p>
					<section
						v-for="(call, i) in job.calls"
						:key="i"
						class="output-call"
					>
						<div class="output-command">
							<code>wp {{ call.args.join(" ") }}</code>
							<span class="text-muted">{{
								seconds(call.duration_ms)
							}}</span>
						</div>
						<p v-if="call.error" class="form-error">
							{{ call.error }}
						</p>
						<template v-if="call.stderr">
							<h3>stderr</h3>
							<pre>{{ call.stderr }}</pre>
						</template>
						<template v-if="call.stdout">
							<h3>stdout</h3>
							<pre>{{ call.stdout }}</pre>
						</template>
						<p
							v-if="!call.stderr && !call.stdout"
							class="text-muted"
						>
							No output.
						</p>
					</section>
				</template>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.output-modal {
	max-width: 900px;
	width: 100%;
	max-height: 90vh;
	overflow: auto;
}

.output-cause {
	font-weight: 600;
	color: var(--warning-text);
}

.output-results {
	margin: 0 0 var(--space-4);
	padding-left: var(--space-5);

	li + li {
		margin-top: var(--space-2);
	}

	strong {
		margin-right: var(--space-2);
	}
}

.output-call + .output-call {
	margin-top: var(--space-4);
	padding-top: var(--space-4);
	border-top: 1px solid var(--border-color);
}

.output-command {
	display: flex;
	justify-content: space-between;
	gap: var(--space-3);

	code {
		word-break: break-all;
	}
}

h3 {
	margin: var(--space-3) 0 var(--space-1);
	font-size: var(--font-size-xs);
	color: var(--text-muted);
	text-transform: uppercase;
}

pre {
	max-height: 320px;
	overflow: auto;
	margin: 0;
	padding: var(--space-3);
	background: var(--bg-body);
	border: 1px solid var(--border-color);
	border-radius: var(--radius-sm);
	font-size: var(--font-size-xs);
	white-space: pre-wrap;
	word-break: break-word;
}
</style>
