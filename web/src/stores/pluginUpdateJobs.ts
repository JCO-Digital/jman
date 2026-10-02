import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";
import { useToastStore } from "./toast";
import type { PluginUpdateJob, PluginUpdateResult } from "../types";
import { BASE_URL, handleErrorResponse } from "../utils/api";

/** How often the job list is polled while any job is queued or running. */
const POLL_INTERVAL_MS = 3000;

const isActive = (job: PluginUpdateJob) =>
	job.status === "queued" || job.status === "running";

/**
 * Background plugin updates. jman-api runs each job (one site, one or more
 * plugins) on its own; this store queues jobs, polls while any are active,
 * and applies finished results to the data store. Job state lives in
 * jman-api, so spinners survive reloads and show other users' updates too.
 */
export const usePluginUpdateJobsStore = defineStore("pluginUpdateJobs", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();
	const toastStore = useToastStore();

	const jobs = ref<PluginUpdateJob[]>([]);
	// Jobs seen while active, so a completion is handled exactly once and
	// jobs that had already finished before this tab loaded aren't toasted.
	const watched = new Set<number>();
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	const activeJobs = computed(() => jobs.value.filter(isActive));

	/** Site ID → names of plugins currently queued or updating there. */
	const updatingBySite = computed(() => {
		const map = new Map<string, Set<string>>();
		for (const job of activeJobs.value) {
			const names = map.get(job.site_id) ?? new Set<string>();
			for (const p of job.plugins) names.add(p.name);
			map.set(job.site_id, names);
		}
		return map;
	});

	function isUpdating(siteId: string, pluginName: string): boolean {
		return updatingBySite.value.get(siteId)?.has(pluginName) ?? false;
	}

	function isSiteUpdating(siteId: string): boolean {
		return updatingBySite.value.has(siteId);
	}

	/**
	 * The latest finished result for a plugin on a site among the given
	 * jobs, so a modal only shows outcomes of updates it started.
	 */
	function resultFor(
		siteId: string,
		pluginName: string,
		jobIds: ReadonlySet<number>,
	): PluginUpdateResult | undefined {
		for (let i = jobs.value.length - 1; i >= 0; i--) {
			const job = jobs.value[i]!;
			if (job.site_id !== siteId || isActive(job)) continue;
			if (!jobIds.has(job.id)) continue;
			const result = job.results.find((r) => r.name === pluginName);
			if (result) return result;
		}
		return undefined;
	}

	function handleFinished(job: PluginUpdateJob) {
		const site = dataStore.getSiteById(job.site_id);
		const siteName = site?.domain ?? job.site_id;
		for (const r of job.results) {
			if (r.status === "Updated") {
				dataStore.applyPluginUpdate(job.site_id, r.name, r.new_version);
			}
		}
		const failed = job.results.filter((r) => r.status === "failed");
		if (job.status === "interrupted") {
			toastStore.addToast(
				`Plugin update on ${siteName} was interrupted by a jman-api restart. Check the site's plugin versions.`,
				"error",
				10000,
			);
		} else if (failed.length > 0) {
			const names = failed.map((r) => r.name).join(", ");
			toastStore.addToast(
				`Failed to update ${names} on ${siteName}.`,
				"error",
				10000,
			);
		} else if (job.error) {
			// Every plugin succeeded but something needs attention (e.g. the
			// site was left in maintenance mode).
			toastStore.addToast(`${siteName}: ${job.error}`, "error", 10000);
		}
	}

	function merge(incoming: PluginUpdateJob[]) {
		const byId = new Map(jobs.value.map((j) => [j.id, j]));
		for (const job of incoming) {
			byId.set(job.id, job);
			if (isActive(job)) {
				watched.add(job.id);
			} else if (watched.delete(job.id)) {
				handleFinished(job);
			}
		}
		jobs.value = [...byId.values()].sort((a, b) => a.id - b.id);
	}

	async function fetchJobs() {
		const res = await fetch(`${BASE_URL}/plugin-update-jobs`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		const listed: PluginUpdateJob[] = await res.json();

		// A watched job missing from the list finished longer ago than the
		// API's recent window (e.g. the tab was asleep); fetch it directly.
		const listedIds = new Set(listed.map((j) => j.id));
		const missing = [...watched].filter((id) => !listedIds.has(id));
		const fetched = await Promise.all(
			missing.map(async (id) => {
				const r = await fetch(`${BASE_URL}/plugin-update-jobs/${id}`, {
					headers: authStore.authHeader,
				});
				if (r.status === 404) {
					watched.delete(id);
					return null;
				}
				return r.ok ? ((await r.json()) as PluginUpdateJob) : null;
			}),
		);
		merge([
			...listed,
			...fetched.filter((j): j is PluginUpdateJob => j !== null),
		]);
	}

	function schedulePoll() {
		if (pollTimer || activeJobs.value.length === 0) return;
		pollTimer = setTimeout(async () => {
			pollTimer = null;
			try {
				await fetchJobs();
			} catch (e) {
				console.error("Failed to poll plugin update jobs", e);
			}
			schedulePoll();
		}, POLL_INTERVAL_MS);
	}

	/** Loads the current jobs, e.g. on app start, and polls if any are active. */
	async function initialize() {
		try {
			await fetchJobs();
		} catch (e) {
			console.error("Failed to load plugin update jobs", e);
		}
		schedulePoll();
	}

	/**
	 * Queues updates: one job per site, each updating all its plugins in one
	 * WP-CLI call.
	 */
	async function enqueue(
		requests: { site_id: string; plugins: string[] }[],
	): Promise<PluginUpdateJob[]> {
		const res = await fetch(`${BASE_URL}/plugin-update-jobs`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ jobs: requests }),
		});
		if (!res.ok) await handleErrorResponse(res);
		const created: PluginUpdateJob[] = await res.json();
		merge(created);
		schedulePoll();
		return created;
	}

	function reset() {
		if (pollTimer) clearTimeout(pollTimer);
		pollTimer = null;
		watched.clear();
		jobs.value = [];
	}

	return {
		jobs,
		activeJobs,
		isUpdating,
		isSiteUpdating,
		resultFor,
		initialize,
		enqueue,
		fetchJobs,
		reset,
	};
});
