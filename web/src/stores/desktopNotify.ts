import { ref, watch } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import { useNotificationStore } from "./notifications";
import { apiFetch, BASE_URL } from "../utils/api";
import type { Incident, IncidentsResponse } from "../types";

/**
 * Desktop notifications, sent by the open jman tab through the browser's
 * Notification API (nothing is pushed when jman isn't open):
 *
 * - your own jobs, one notification per batch once none of them is busy,
 *   only while the tab is in the background (the toasts cover the rest);
 * - update review results, as part of that batch;
 * - new incidents (a site down, a security incident), tab hidden or not.
 *
 * Turned on per browser in Account settings. With several tabs open, the
 * first to claim a job or incident (in localStorage) sends it.
 */

export interface DesktopNotifySettings {
	/** Your jobs finishing. */
	jobs: boolean;
	/** Jobs whose update text diff found something to review. */
	reviews: boolean;
	/** New incidents. */
	incidents: boolean;
}

/** A job of yours that finished while this tab watched it. */
export interface FinishedJob {
	jobId: number;
	failed: boolean;
	review: boolean;
	title: string;
	message: string;
	site: string;
}

const SETTINGS_PREFIX = "jman_desktop_notify:";
const CLAIMS_KEY = "jman_desktop_notify_claims";
/** How long a claim keeps other tabs from sending the same thing. */
const CLAIM_TTL_MS = 24 * 60 * 60 * 1000;
/** A long batch still reports at least this often. */
const MAX_BATCH_WAIT_MS = 10 * 60 * 1000;
/** Incidents shown one by one; more are summed up in one notification. */
const MAX_INCIDENT_NOTIFICATIONS = 3;

const ICON = "/jman.png";

/** Opens a page of the app; the router is loaded late to avoid an import cycle. */
async function go(
	to: Parameters<(typeof import("../router"))["default"]["push"]>[0],
) {
	const { default: router } = await import("../router");
	await router.push(to);
}

const supported = typeof window !== "undefined" && "Notification" in window;

/**
 * Claims key for this tab, so other tabs don't send the same notification.
 * Without localStorage, every tab sends (and a tag merges them).
 */
function claim(key: string): boolean {
	try {
		const now = Date.now();
		const raw = localStorage.getItem(CLAIMS_KEY);
		const claims: Record<string, number> = raw ? JSON.parse(raw) : {};
		for (const [k, at] of Object.entries(claims)) {
			if (now - at > CLAIM_TTL_MS) delete claims[k];
		}
		if (claims[key]) return false;
		claims[key] = now;
		localStorage.setItem(CLAIMS_KEY, JSON.stringify(claims));
		return true;
	} catch {
		return true;
	}
}

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

/** "a.com, b.com and 3 more" */
function listSites(sites: string[], max = 3): string {
	const unique = [...new Set(sites)];
	if (unique.length <= max) return unique.join(", ");
	return `${unique.slice(0, max).join(", ")} and ${unique.length - max} more`;
}

export const useDesktopNotifyStore = defineStore("desktopNotify", () => {
	const authStore = useAuthStore();

	const permission = ref<NotificationPermission>(
		supported ? Notification.permission : "denied",
	);
	const settings = ref<DesktopNotifySettings>({
		jobs: false,
		reviews: false,
		incidents: false,
	});
	let loadedFor: string | null = null;

	const username = () => authStore.user?.username ?? null;

	// --- Settings, per browser and user ---

	function load() {
		const u = username();
		if (!u || loadedFor === u) return;
		loadedFor = u;
		try {
			const raw = localStorage.getItem(SETTINGS_PREFIX + u);
			settings.value = {
				jobs: false,
				reviews: false,
				incidents: false,
				...(raw ? JSON.parse(raw) : {}),
			};
		} catch {
			// Keep the defaults.
		}
		if (supported) permission.value = Notification.permission;
	}

	watch(
		settings,
		(s) => {
			const u = username();
			if (!u || loadedFor !== u) return;
			try {
				localStorage.setItem(SETTINGS_PREFIX + u, JSON.stringify(s));
			} catch {
				// Not remembered then.
			}
		},
		{ deep: true },
	);

	/** Asks the browser for permission; call from a click. */
	async function requestPermission(): Promise<boolean> {
		if (!supported) return false;
		if (Notification.permission === "default") {
			permission.value = await Notification.requestPermission();
		} else {
			permission.value = Notification.permission;
		}
		return permission.value === "granted";
	}

	/** Turns one kind on or off, asking for permission to turn it on. */
	async function setEnabled(kind: keyof DesktopNotifySettings, on: boolean) {
		if (on && !(await requestPermission())) return;
		settings.value[kind] = on;
		if (on && kind === "incidents") void pollIncidents();
	}

	function show(
		title: string,
		body: string,
		tag: string,
		onClick: () => void,
	) {
		if (!supported || Notification.permission !== "granted") return;
		try {
			const n = new Notification(title, { body, tag, icon: ICON });
			n.onclick = () => {
				window.focus();
				onClick();
				n.close();
			};
		} catch (e) {
			// Some browsers (Android Chrome) only notify from a service worker.
			console.error("Desktop notification failed", e);
		}
	}

	/** Shows a sample notification, from the settings. */
	function test() {
		show(
			"jman notifications are on",
			"This is how finished jobs and incidents will show up.",
			"jman-test",
			() => {},
		);
	}

	// --- Jobs ---

	let batch: FinishedJob[] = [];
	let batchTimer: ReturnType<typeof setTimeout> | null = null;

	const jobsWanted = () =>
		(settings.value.jobs || settings.value.reviews) &&
		permission.value === "granted";

	/** Adds a job of yours that just finished to the current batch. */
	function jobFinished(job: FinishedJob) {
		if (!jobsWanted()) return;
		batch.push(job);
		batchTimer ??= setTimeout(flushJobs, MAX_BATCH_WAIT_MS);
	}

	/**
	 * Tells how many of your jobs are still busy; at none, the batch is
	 * sent.
	 */
	function ownBusy(count: number) {
		if (count === 0 && batch.length) flushJobs();
	}

	function flushJobs() {
		if (batchTimer) clearTimeout(batchTimer);
		batchTimer = null;
		const jobs = batch.filter((j) => claim(`job:${j.jobId}`));
		batch = [];
		// In front, the toasts already said it.
		if (!jobs.length || !document.hidden) return;

		const failed = jobs.filter((j) => j.failed);
		const review = jobs.filter((j) => j.review);
		if (!settings.value.jobs && !review.length) return;
		const toReview = () => void go({ name: "update-review" });
		const openCenter = () => useNotificationStore().setPanelOpen(true);
		const onClick = review.length ? toReview : openCenter;
		const tag = `jobs:${jobs[0]!.jobId}`;

		if (!settings.value.jobs) {
			show(
				`${plural(review.length, "update")} to review`,
				`The text check found changes on ${listSites(review.map((j) => j.site))}.`,
				tag,
				toReview,
			);
			return;
		}

		if (jobs.length === 1) {
			const j = jobs[0]!;
			show(j.title, j.message, tag, onClick);
			return;
		}

		const title = failed.length
			? `${failed.length} of ${jobs.length} jobs failed`
			: `${jobs.length} jobs finished`;
		const lines: string[] = [];
		if (failed.length) {
			lines.push(`Failed: ${listSites(failed.map((j) => j.site))}`);
		}
		if (review.length) {
			lines.push(`To review: ${listSites(review.map((j) => j.site))}`);
		}
		if (!lines.length) lines.push(listSites(jobs.map((j) => j.site)));
		show(title, lines.join("\n"), tag, onClick);
	}

	// --- Incidents ---

	/** Active incident IDs seen so far; null until the first poll. */
	let seenIncidents: Set<number> | null = null;

	/** Checks for new incidents; called on the monitoring refresh. */
	async function pollIncidents() {
		if (!settings.value.incidents || permission.value !== "granted") {
			seenIncidents = null;
			return;
		}
		let incidents: Incident[];
		try {
			const res = await apiFetch(
				`${BASE_URL}/incidents?filter=active&limit=100`,
			);
			if (!res.ok) return;
			incidents =
				((await res.json()) as IncidentsResponse).incidents ?? [];
		} catch {
			return;
		}
		const first = seenIncidents === null;
		const seen = seenIncidents ?? new Set<number>();
		const fresh = incidents.filter((i) => !seen.has(i.id));
		seenIncidents = new Set(incidents.map((i) => i.id));
		// The first poll only learns what's already open.
		if (first) return;

		const mine = fresh.filter(
			(i) => i.status === "open" && claim(`incident:${i.id}`),
		);
		const shown = mine.slice(0, MAX_INCIDENT_NOTIFICATIONS);
		for (const i of shown) {
			const area = i.type === "security" ? "security" : "uptime";
			const open = () =>
				void go({ name: "monitoring", params: { area } });
			if (i.type === "security") {
				show(
					`Security incident: ${i.domain}`,
					i.escalation ||
						(i.severity
							? `Severity: ${i.severity}`
							: i.error_message),
					`incident:${i.id}`,
					open,
				);
			} else {
				show(
					`${i.domain} is down`,
					i.error_message ||
						(i.error_code
							? `HTTP ${i.error_code}`
							: "Not responding"),
					`incident:${i.id}`,
					open,
				);
			}
		}
		const rest = mine.length - shown.length;
		if (rest > 0) {
			show(
				`${plural(rest, "more incident")}`,
				listSites(mine.slice(shown.length).map((i) => i.domain)),
				`incidents:${mine[shown.length]!.id}`,
				() => void go({ name: "monitoring" }),
			);
		}
	}

	/** Loads the user's settings; call once signed in. */
	function initialize() {
		load();
		void pollIncidents();
	}

	function reset() {
		loadedFor = null;
		seenIncidents = null;
		batch = [];
		if (batchTimer) clearTimeout(batchTimer);
		batchTimer = null;
		settings.value = { jobs: false, reviews: false, incidents: false };
	}

	return {
		supported,
		permission,
		settings,
		requestPermission,
		setEnabled,
		test,
		jobFinished,
		ownBusy,
		pollIncidents,
		initialize,
		reset,
	};
});
