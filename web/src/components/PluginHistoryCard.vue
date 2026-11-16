<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { BASE_URL, apiFetch, handleErrorResponse } from "../utils/api";
import { compareVersions } from "../utils/vulnList";
import { useUpdateJobsStore } from "../stores/updateJobs";
import type { PluginHistoryEvent } from "../types";
import LoadingSpinner from "./LoadingSpinner.vue";

const props = defineProps<{
	slug: string;
}>();

const jobsStore = useUpdateJobsStore();

/** Ledger entries per page; a page can hold more events than this. */
const PAGE_SIZE = 50;

const events = ref<PluginHistoryEvent[]>([]);
const nextBefore = ref<number | null>(null);
const loading = ref(false);
const loadingMore = ref(false);
const error = ref("");

async function fetchPage(before?: number) {
	const params = new URLSearchParams({ limit: String(PAGE_SIZE) });
	if (before) params.set("before", String(before));
	const res = await apiFetch(
		`${BASE_URL}/plugins/${encodeURIComponent(props.slug)}/history?${params}`,
	);
	if (!res.ok) await handleErrorResponse(res);
	return (await res.json()) as {
		events: PluginHistoryEvent[];
		next_before: number | null;
	};
}

async function load() {
	loading.value = events.value.length === 0;
	error.value = "";
	try {
		const page = await fetchPage();
		events.value = page.events;
		nextBefore.value = page.next_before;
	} catch (e: any) {
		error.value = e.message || "Failed to load the history";
	} finally {
		loading.value = false;
	}
}

async function loadMore() {
	if (!nextBefore.value) return;
	loadingMore.value = true;
	try {
		const page = await fetchPage(nextBefore.value);
		events.value = [...events.value, ...page.events];
		nextBefore.value = page.next_before;
	} catch (e: any) {
		error.value = e.message || "Failed to load the history";
	} finally {
		loadingMore.value = false;
	}
}

onMounted(load);
watch(() => props.slug, load);

// Reload once jobs (which write the ledger) finish.
let reloadTimer: ReturnType<typeof setTimeout> | undefined;
watch(
	() => jobsStore.lastFinishedBySite,
	() => {
		clearTimeout(reloadTimer);
		reloadTimer = setTimeout(load, 1000);
	},
);

const actionLabels: Record<string, string> = {
	install: "Installed",
	activate: "Activated",
	deactivate: "Deactivated",
	delete: "Deleted",
	uninstall: "Uninstalled (data removed)",
};

/** What happened, e.g. "Updated 6.0 → 6.0.2". */
function describe(e: PluginHistoryEvent): string {
	const from = e.old_version || "?";
	const to = e.new_version || "?";
	if (e.action === "update") {
		switch (e.outcome) {
			case "failed":
				return `Update ${from} → ${to} failed`;
			case "skipped":
				return `Held back at ${from} by an update lock`;
			case "unchanged":
				return `Reinstalled ${to}`;
		}
		const verb =
			e.old_version &&
			e.new_version &&
			compareVersions(e.new_version, e.old_version) < 0
				? "Downgraded"
				: "Updated";
		return `${verb} ${from} → ${to}`;
	}
	if (e.action === "delete" && e.detected) {
		return `Removed${e.old_version ? ` (was ${e.old_version})` : ""}`;
	}
	let txt = actionLabels[e.action] ?? e.action;
	if (e.action === "install" && e.new_version) txt += ` ${e.new_version}`;
	if (e.outcome === "failed") txt += " (failed)";
	return txt;
}

/** Who made the change. */
function who(e: PluginHistoryEvent): string {
	if (!e.detected) return e.by || "—";
	if (e.by.includes("agent")) return "jman-agent";
	if (e.by.includes("ssh")) return "SSH refresh";
	return e.by || "—";
}

function formatTime(iso: string) {
	return new Date(iso).toLocaleString(undefined, {
		dateStyle: "short",
		timeStyle: "short",
	});
}
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Update History</h2>
		</div>

		<LoadingSpinner v-if="loading" message="Loading history..." />
		<p v-else-if="error && !events.length" class="form-error">
			{{ error }}
		</p>
		<p v-else-if="!events.length" class="text-muted">
			No changes recorded for this plugin yet.
		</p>
		<div v-else class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>When</th>
						<th>Site</th>
						<th>Change</th>
						<th>By</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="(e, i) in events" :key="`${e.ledger_id}-${i}`">
						<td class="when">{{ formatTime(e.at) }}</td>
						<td>
							<RouterLink
								:to="`/site/${e.site_id}`"
								class="font-medium"
								>{{ e.site_name || e.site_id }}</RouterLink
							>
						</td>
						<td>
							<span
								:class="{
									'text-error': e.outcome === 'failed',
								}"
								>{{ describe(e) }}</span
							>
							<span
								v-if="e.detected"
								class="status-badge badge-sm info"
								title="Changed outside jman; found by jman-agent or a refresh"
								>Outside jman</span
							>
							<div v-if="e.error" class="sub-text">
								{{ e.error }}
							</div>
							<div v-if="e.note" class="sub-text">
								{{ e.note }}
							</div>
							<div
								v-if="e.source && e.source !== slug"
								class="sub-text"
							>
								Source: {{ e.source }}
							</div>
						</td>
						<td>{{ who(e) }}</td>
					</tr>
				</tbody>
			</table>
		</div>

		<p v-if="error && events.length" class="form-error mt-2">
			{{ error }}
		</p>
		<button
			v-if="nextBefore"
			class="btn btn-text btn-sm mt-2"
			:disabled="loadingMore"
			@click="loadMore"
		>
			{{ loadingMore ? "Loading…" : "Show older" }}
		</button>
	</section>
</template>

<style scoped>
.when {
	white-space: nowrap;
}
</style>
