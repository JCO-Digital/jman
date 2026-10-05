import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";
import { useNotificationStore, type NotificationType } from "./notifications";
import type { PluginActionKind, UpdateJob, UpdateJobKind } from "../types";
import { BASE_URL, handleErrorResponse } from "../utils/api";

/** How often the job list is polled while any job is queued or running. */
const POLL_INTERVAL_MS = 3000;

const isActive = (job: UpdateJob) =>
	job.status === "queued" || job.status === "running";

const truncate = (msg: string) =>
	msg.length > 150 ? msg.substring(0, 147) + "..." : msg;

/** Present and past tense of each plugin management action, for messages. */
const ACTION_VERBS: Record<PluginActionKind, [string, string]> = {
	activate: ["Activating", "Activated"],
	deactivate: ["Deactivating", "Deactivated"],
	delete: ["Deleting", "Deleted"],
	uninstall: ["Uninstalling", "Uninstalled"],
};

const isPluginAction = (kind: UpdateJobKind): kind is PluginActionKind =>
	kind in ACTION_VERBS;

/** What an install request installs: a slug or ZIP URL, or a ZIP file. */
export type InstallSource =
	| { source: string; activate: boolean }
	| { file: File; activate: boolean };

/**
 * Background plugin and core updates. jman-api runs each job (one site,
 * one or more plugins or WordPress core) on its own; this store queues
 * jobs, polls while any are active, applies finished results to the data
 * store and mirrors every job into the task center. Job state lives in
 * jman-api, so spinners survive reloads and show other users' updates too.
 */
export const useUpdateJobsStore = defineStore("updateJobs", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();
	const notifications = useNotificationStore();

	const jobs = ref<UpdateJob[]>([]);
	/**
	 * Site ID → when (ms) a job on that site last finished in this tab.
	 * Views watch this to reload data jman-api wrote, like the ledger.
	 */
	const lastFinishedBySite = ref<Record<string, number>>({});
	// Jobs seen while active, so a completion is handled exactly once and
	// jobs that had already finished before this tab loaded aren't toasted.
	const watched = new Set<number>();
	let pollTimer: ReturnType<typeof setTimeout> | null = null;

	const activeJobs = computed(() => jobs.value.filter(isActive));

	/**
	 * Site ID → plugin name → the kind of the oldest queued or running job
	 * acting on it (an update or a management action).
	 */
	const pendingBySite = computed(() => {
		const map = new Map<string, Map<string, UpdateJobKind>>();
		for (const job of activeJobs.value) {
			if (job.kind === "core" || job.kind === "install") continue;
			const names =
				map.get(job.site_id) ?? new Map<string, UpdateJobKind>();
			for (const p of job.plugins) {
				if (!names.has(p.name)) names.set(p.name, job.kind);
			}
			map.set(job.site_id, names);
		}
		return map;
	});

	/** The kind of job queued or running on a plugin, if any. */
	function pendingAction(
		siteId: string,
		pluginName: string,
	): UpdateJobKind | undefined {
		return pendingBySite.value.get(siteId)?.get(pluginName);
	}

	/** Queued or running plugin installs on a site. */
	function activeInstalls(siteId: string): UpdateJob[] {
		return activeJobs.value.filter(
			(j) => j.kind === "install" && j.site_id === siteId,
		);
	}

	/** The queued or running core update job on a site, if any. */
	function activeCoreJob(siteId: string): UpdateJob | undefined {
		return activeJobs.value.find(
			(j) => j.kind === "core" && j.site_id === siteId,
		);
	}

	function handleFinished(job: UpdateJob) {
		if (job.kind === "core") {
			if (job.core) dataStore.applyCoreUpdate(job.site_id, job.core);
		} else {
			if (job.kind === "plugins") {
				// Show new versions right away; the reload below confirms them.
				for (const r of job.results) {
					if (r.status === "Updated") {
						dataStore.applyPluginUpdate(
							job.site_id,
							r.name,
							r.new_version,
						);
					}
				}
			}
			// The job refreshed jman-api's cached plugin list for the site.
			dataStore.reloadSitePlugins(job.site_id).catch((e) => {
				console.error("Failed to reload site plugins", e);
			});
		}
		lastFinishedBySite.value = {
			...lastFinishedBySite.value,
			[job.site_id]: Date.now(),
		};
	}

	/** What the task center shows for a job in its current state. */
	function describeJob(job: UpdateJob): {
		title: string;
		message: string;
		type: NotificationType;
	} {
		const site = dataStore.getSiteById(job.site_id)?.domain ?? job.site_id;
		if (job.kind === "core") {
			return {
				title: `WordPress core · ${site}`,
				...describeCore(job, site),
			};
		}
		if (job.kind === "install") {
			return {
				title: `Install plugin · ${site}`,
				...describeInstall(job, site),
			};
		}
		if (isPluginAction(job.kind)) {
			const count = job.plugins.length;
			const what =
				count === 1 ? job.plugins[0]!.name : `${count} plugins`;
			const action = job.kind.charAt(0).toUpperCase() + job.kind.slice(1);
			return {
				title: `${action} ${what} · ${site}`,
				...describeAction(job, job.kind, site),
			};
		}
		const count = job.plugins.length;
		return {
			title: `${count} plugin${count === 1 ? "" : "s"} · ${site}`,
			...describePlugins(job, site),
		};
	}

	function describeCore(
		job: UpdateJob,
		siteName: string,
	): { message: string; type: NotificationType } {
		const target = job.target === "major" ? "major" : "minor";
		const r = job.results[0];
		switch (job.status) {
			case "queued":
				return {
					message: `WordPress ${target} update on ${siteName} is queued.`,
					type: "running",
				};
			case "running":
				return {
					message: `Updating WordPress (${target}) on ${siteName}…`,
					type: "running",
				};
			case "interrupted":
				return {
					message: `WordPress update on ${siteName} was interrupted by a jman-api restart. Check the site's WordPress version.`,
					type: "error",
				};
		}
		if (job.status === "failed" || r?.status === "failed") {
			return {
				message: `Failed to update WordPress core on ${siteName}: ${truncate(job.error || r?.error || "Unknown error")}`,
				type: "error",
			};
		}
		if (r?.status === "Up to date") {
			return {
				message: `WordPress on ${siteName} was already up to date (${r.new_version}).`,
				type: "success",
			};
		}
		const from = r?.old_version ? ` from ${r.old_version}` : "";
		return {
			message: `Updated WordPress on ${siteName}${from} to ${r?.new_version || "the latest version"}.`,
			type: "success",
		};
	}

	function describeAction(
		job: UpdateJob,
		kind: PluginActionKind,
		siteName: string,
	): { message: string; type: NotificationType } {
		const [doing, done] = ACTION_VERBS[kind];
		const requested = job.plugins.map((p) => p.name).join(", ");
		switch (job.status) {
			case "queued":
				return {
					message: `Queued: ${doing.toLowerCase()} ${requested} on ${siteName}.`,
					type: "running",
				};
			case "running":
				return {
					message: `${doing} ${requested} on ${siteName}…`,
					type: "running",
				};
			case "interrupted":
				return {
					message: `${doing} ${requested} on ${siteName} was interrupted by a jman-api restart. Check the site's plugins.`,
					type: "error",
				};
		}
		const failed = job.results.filter((r) => r.status === "failed");
		if (job.status === "failed" || failed.length > 0) {
			const names = failed.length
				? failed.map((r) => r.name).join(", ")
				: requested;
			const reason = failed[0]?.error || job.error || "Unknown error";
			return {
				message: `Failed: ${doing.toLowerCase()} ${names} on ${siteName}: ${truncate(reason)}`,
				type: "error",
			};
		}
		return {
			message: `${done} ${requested} on ${siteName}.`,
			type: "success",
		};
	}

	function describeInstall(
		job: UpdateJob,
		siteName: string,
	): { message: string; type: NotificationType } {
		const source = job.source ?? "plugin";
		switch (job.status) {
			case "queued":
				return {
					message: `Queued install of ${source} on ${siteName}.`,
					type: "running",
				};
			case "running":
				return {
					message: `Installing ${source} on ${siteName}…`,
					type: "running",
				};
			case "interrupted":
				return {
					message: `Install of ${source} on ${siteName} was interrupted by a jman-api restart. Check the site's plugins.`,
					type: "error",
				};
		}
		const r = job.results[0];
		if (job.status === "failed" || r?.status === "failed") {
			return {
				message: `Failed to install ${source} on ${siteName}: ${truncate(job.error || r?.error || "Unknown error")}`,
				type: "error",
			};
		}
		const name =
			r?.name && r.name !== source ? `${r.name} (${source})` : source;
		const version = r?.new_version ? ` ${r.new_version}` : "";
		const activated = job.activate ? " and activated" : "";
		return {
			message: `Installed${activated} ${name}${version} on ${siteName}.`,
			type: "success",
		};
	}

	function describePlugins(
		job: UpdateJob,
		siteName: string,
	): { message: string; type: NotificationType } {
		const requested = job.plugins.map((p) => p.name).join(", ");
		if (job.status === "queued") {
			return {
				message: `Queued update of ${requested} on ${siteName}.`,
				type: "running",
			};
		}
		if (job.status === "running") {
			return {
				message: `Updating ${requested} on ${siteName}…`,
				type: "running",
			};
		}

		const names = (status: string) =>
			job.results
				.filter((r) => r.status === status)
				.map((r) => r.name)
				.join(", ");
		const updated = names("Updated");
		const failed = names("failed");

		if (job.status === "interrupted") {
			return {
				message: `Plugin update on ${siteName} was interrupted by a jman-api restart. Check the site's plugin versions.`,
				type: "error",
			};
		}
		if (failed) {
			const alsoUpdated = updated ? ` Updated: ${updated}.` : "";
			return {
				message: `Failed to update ${failed} on ${siteName}.${alsoUpdated}`,
				type: "error",
			};
		}
		if (job.error) {
			// Every plugin succeeded but something needs attention (e.g. the
			// site was left in maintenance mode).
			return { message: `${siteName}: ${job.error}`, type: "error" };
		}
		if (updated) {
			return {
				message: `Updated ${updated} on ${siteName}.`,
				type: "success",
			};
		}
		return {
			message: `Plugins on ${siteName} were already up to date.`,
			type: "success",
		};
	}

	function syncNotification(job: UpdateJob, notify: boolean) {
		notifications.upsertTask(
			{
				jobId: job.id,
				siteId: job.site_id,
				createdBy: job.created_by,
				status: job.status,
				active: isActive(job),
				createdAt: Date.parse(job.created_at) || Date.now(),
				...describeJob(job),
			},
			notify,
		);
	}

	function merge(incoming: UpdateJob[]) {
		const byId = new Map(jobs.value.map((j) => [j.id, j]));
		for (const job of incoming) {
			byId.set(job.id, job);
			if (isActive(job)) {
				watched.add(job.id);
				syncNotification(job, true);
			} else if (watched.delete(job.id)) {
				handleFinished(job);
				syncNotification(job, true);
			} else {
				syncNotification(job, false);
			}
		}
		jobs.value = [...byId.values()].sort((a, b) => a.id - b.id);
	}

	async function fetchJobs() {
		const res = await fetch(`${BASE_URL}/update-jobs`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		const listed: UpdateJob[] = await res.json();

		// A watched job missing from the list finished longer ago than the
		// API's recent window (e.g. the tab was asleep); fetch it directly.
		const listedIds = new Set(listed.map((j) => j.id));
		const missing = [...watched].filter((id) => !listedIds.has(id));
		const fetched = await Promise.all(
			missing.map(async (id) => {
				const r = await fetch(`${BASE_URL}/update-jobs/${id}`, {
					headers: authStore.authHeader,
				});
				if (r.status === 404) {
					watched.delete(id);
					return null;
				}
				return r.ok ? ((await r.json()) as UpdateJob) : null;
			}),
		);
		merge([
			...listed,
			...fetched.filter((j): j is UpdateJob => j !== null),
		]);
	}

	function schedulePoll() {
		if (pollTimer || activeJobs.value.length === 0) return;
		pollTimer = setTimeout(async () => {
			pollTimer = null;
			try {
				await fetchJobs();
			} catch (e) {
				console.error("Failed to poll update jobs", e);
			}
			schedulePoll();
		}, POLL_INTERVAL_MS);
	}

	/** Loads the current jobs, e.g. on app start, and polls if any are active. */
	async function initialize() {
		try {
			await fetchJobs();
		} catch (e) {
			console.error("Failed to load update jobs", e);
		}
		schedulePoll();
	}

	async function postJobs<T>(path: string, body: unknown): Promise<T> {
		const res = await fetch(`${BASE_URL}${path}`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(body),
		});
		if (!res.ok) await handleErrorResponse(res);
		return (await res.json()) as T;
	}

	/**
	 * Queues updates: one job per site, each updating all its plugins in one
	 * WP-CLI call.
	 */
	async function enqueue(
		requests: { site_id: string; plugins: string[] }[],
	): Promise<UpdateJob[]> {
		const created = await postJobs<UpdateJob[]>("/plugin-update-jobs", {
			jobs: requests,
		});
		merge(created);
		schedulePoll();
		return created;
	}

	/** Queues activating, deactivating, deleting or uninstalling plugins. */
	async function enqueueAction(
		siteId: string,
		action: PluginActionKind,
		plugins: string[],
	): Promise<UpdateJob> {
		const job = await postJobs<UpdateJob>(
			`/sites/${siteId}/plugin-actions`,
			{
				action,
				plugins,
			},
		);
		merge([job]);
		schedulePoll();
		return job;
	}

	/** Queues installing a plugin from a slug, a ZIP URL or a ZIP upload. */
	async function enqueueInstall(
		siteId: string,
		install: InstallSource,
	): Promise<UpdateJob> {
		let job: UpdateJob;
		if ("file" in install) {
			const form = new FormData();
			form.append("file", install.file);
			form.append("activate", String(install.activate));
			const res = await fetch(
				`${BASE_URL}/sites/${siteId}/plugin-install`,
				{
					method: "POST",
					// No Content-Type: the browser sets the multipart boundary.
					headers: authStore.authHeader,
					body: form,
				},
			);
			if (!res.ok) await handleErrorResponse(res);
			job = (await res.json()) as UpdateJob;
		} else {
			job = await postJobs<UpdateJob>(
				`/sites/${siteId}/plugin-install`,
				install,
			);
		}
		merge([job]);
		schedulePoll();
		return job;
	}

	/** Queues a WordPress core update on a site. */
	async function enqueueCore(
		siteId: string,
		target: "minor" | "major",
	): Promise<UpdateJob> {
		const job = await postJobs<UpdateJob>(`/sites/${siteId}/core-update`, {
			target,
		});
		merge([job]);
		schedulePoll();
		return job;
	}

	function reset() {
		if (pollTimer) clearTimeout(pollTimer);
		pollTimer = null;
		watched.clear();
		jobs.value = [];
		lastFinishedBySite.value = {};
	}

	return {
		jobs,
		activeJobs,
		lastFinishedBySite,
		pendingAction,
		activeInstalls,
		activeCoreJob,
		initialize,
		enqueue,
		enqueueAction,
		enqueueInstall,
		enqueueCore,
		fetchJobs,
		reset,
	};
});
