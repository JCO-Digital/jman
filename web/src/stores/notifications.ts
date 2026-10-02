import { computed, ref, watch } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";

export type ToastType = "error" | "success" | "info";
export type NotificationType = ToastType | "running";

/**
 * One entry in the task center: a plain notification (what used to be a
 * toast) or a background task. A toast is an entry that is currently
 * "popped up" in the toast stack; every entry stays in the history.
 */
export interface Notification {
	id: string;
	type: NotificationType;
	title?: string;
	message: string;
	createdAt: number;
	updatedAt: number;
	/** Shown in the toast stack. */
	popup: boolean;
	/** Arrived while the task center was closed and hasn't been seen. */
	unread: boolean;
	/** Task entries only. */
	task?: {
		jobId: number;
		siteId: string;
		createdBy: string;
		status: string;
		/** The user hid this task's sticky toast while it runs. */
		hidden: boolean;
	};
}

/** What a task entry should currently say, from the job store. */
export interface TaskUpdate {
	jobId: number;
	siteId: string;
	createdBy: string;
	status: string;
	active: boolean;
	type: NotificationType;
	title: string;
	message: string;
	createdAt: number;
}

/** Finished and plain entries kept in the history; running ones are always kept. */
export const MAX_HISTORY = 12;

const DEFAULT_DURATION: Record<ToastType, number> = {
	error: 10000,
	success: 6000,
	info: 5000,
};

const STORAGE_PREFIX = "jman_notifications:";
/** Bounds the remembered cleared/hidden job IDs. */
const MAX_REMEMBERED_IDS = 200;

interface Persisted {
	history: Notification[];
	hiddenJobs: number[];
	clearedJobs: number[];
}

const taskKey = (jobId: number) => `job:${jobId}`;

interface Timer {
	handle: ReturnType<typeof setTimeout> | null;
	remaining: number;
	startedAt: number;
}

/**
 * Notifications and background tasks, unified. `addToast` keeps its old
 * signature, so existing callers of useToastStore() are unchanged.
 */
export const useNotificationStore = defineStore("notifications", () => {
	const authStore = useAuthStore();

	const entries = ref<Notification[]>([]);
	const panelOpen = ref(false);
	// Job IDs whose sticky toast the user hid, and finished jobs the user
	// cleared, so a poll doesn't bring them back.
	const hiddenJobs = new Set<number>();
	const clearedJobs = new Set<number>();
	const timers = new Map<string, Timer>();
	let nextId = 0;
	let loadedFor: string | null = null;

	const username = () => authStore.user?.username ?? null;

	const running = computed(() =>
		entries.value
			.filter((e) => e.type === "running")
			.sort((a, b) => a.createdAt - b.createdAt),
	);

	/** Non-running entries, newest first, capped at MAX_HISTORY. */
	const recent = computed(() =>
		entries.value
			.filter((e) => e.type !== "running")
			.sort((a, b) => b.updatedAt - a.updatedAt)
			.slice(0, MAX_HISTORY),
	);

	/** Entries in the toast stack, oldest first (newest nearest the launcher). */
	const popups = computed(() =>
		entries.value
			.filter((e) => e.popup)
			.sort((a, b) => a.updatedAt - b.updatedAt),
	);

	const unreadCount = computed(
		() => entries.value.filter((e) => e.unread).length,
	);

	function find(id: string) {
		return entries.value.find((e) => e.id === id);
	}

	// --- Toast timers (pause on hover) ---

	function clearTimer(id: string) {
		const t = timers.get(id);
		if (t?.handle) clearTimeout(t.handle);
		timers.delete(id);
	}

	function startTimer(id: string, duration: number) {
		clearTimer(id);
		const t: Timer = { handle: null, remaining: duration, startedAt: 0 };
		timers.set(id, t);
		resumeTimer(id);
	}

	function pauseTimer(id: string) {
		const t = timers.get(id);
		if (!t?.handle) return;
		clearTimeout(t.handle);
		t.handle = null;
		t.remaining -= Date.now() - t.startedAt;
	}

	function resumeTimer(id: string) {
		const t = timers.get(id);
		if (!t || t.handle) return;
		t.startedAt = Date.now();
		t.handle = setTimeout(
			() => {
				timers.delete(id);
				const e = find(id);
				if (e) e.popup = false;
			},
			Math.max(t.remaining, 0),
		);
	}

	// --- History bounds and persistence ---

	function prune() {
		const keep = new Set(recent.value.map((e) => e.id));
		const removed = entries.value.filter(
			(e) => e.type !== "running" && !keep.has(e.id),
		);
		if (removed.length === 0) return;
		for (const e of removed) {
			clearTimer(e.id);
			// A pruned job would otherwise be re-added by the next poll.
			if (e.task) remember(clearedJobs, e.task.jobId);
		}
		entries.value = entries.value.filter(
			(e) => e.type === "running" || keep.has(e.id),
		);
	}

	function remember(set: Set<number>, id: number) {
		set.add(id);
		if (set.size > MAX_REMEMBERED_IDS) {
			set.delete(set.values().next().value!);
		}
	}

	function storageKey() {
		const u = username();
		return u ? STORAGE_PREFIX + u : null;
	}

	function save() {
		const key = storageKey();
		if (!key || loadedFor !== username()) return;
		const data: Persisted = {
			// Running tasks are rebuilt from jman-api after a reload.
			history: recent.value.map((e) => ({ ...e, popup: false })),
			hiddenJobs: [...hiddenJobs],
			clearedJobs: [...clearedJobs],
		};
		try {
			localStorage.setItem(key, JSON.stringify(data));
		} catch {
			// Storage unavailable or full; history just won't persist.
		}
	}

	function load() {
		const key = storageKey();
		loadedFor = username();
		if (!key) return;
		let data: Persisted | null;
		try {
			const raw = localStorage.getItem(key);
			data = raw ? (JSON.parse(raw) as Persisted) : null;
		} catch {
			data = null;
		}
		if (!data) return;
		data.hiddenJobs?.forEach((id) => hiddenJobs.add(id));
		data.clearedJobs?.forEach((id) => clearedJobs.add(id));
		const present = new Set(entries.value.map((e) => e.id));
		const restored = (data.history ?? [])
			.filter((e) => e && e.type !== "running" && !present.has(e.id))
			.map((e) => ({ ...e, popup: false }));
		entries.value = [...entries.value, ...restored];
		prune();
	}

	watch(entries, save, { deep: true });

	// --- Plain notifications ---

	/** Shows a toast and keeps it in the task center history. */
	function notify(
		message: string,
		type: ToastType = "error",
		duration: number = DEFAULT_DURATION[type],
	) {
		const now = Date.now();
		const id = `n:${now}:${nextId++}`;
		entries.value.push({
			id,
			type,
			message,
			createdAt: now,
			updatedAt: now,
			popup: !panelOpen.value,
			unread: !panelOpen.value,
		});
		if (!panelOpen.value) startTimer(id, duration);
		prune();
	}

	/** Old toast API: addToast(message, type, duration). */
	function addToast(message: string, type?: ToastType, duration?: number) {
		notify(message, type, duration);
	}

	/** Closes an entry's toast; the entry stays in the history. */
	function dismiss(id: string) {
		clearTimer(id);
		const e = find(id);
		if (e) e.popup = false;
	}

	// --- Tasks ---

	/**
	 * Creates or updates a task entry from a job. Status changes pop the
	 * entry up again for the user's own jobs: running ones stay up (unless
	 * hidden) until they finish, finished ones get a normal toast timer.
	 * notify is false for jobs that finished before this tab saw them run;
	 * those are only added to the history.
	 */
	function upsertTask(u: TaskUpdate, notify: boolean) {
		if (clearedJobs.has(u.jobId)) return;
		const id = taskKey(u.jobId);
		const existing = find(id);
		const own = u.createdBy === username();
		const hidden = hiddenJobs.has(u.jobId);

		if (existing && existing.task?.status === u.status) {
			existing.title = u.title;
			existing.message = u.message;
			return;
		}
		if (!existing && !u.active && !notify) {
			entries.value.push({
				id,
				type: u.type,
				title: u.title,
				message: u.message,
				createdAt: u.createdAt,
				updatedAt: u.createdAt,
				popup: false,
				unread: false,
				task: {
					jobId: u.jobId,
					siteId: u.siteId,
					createdBy: u.createdBy,
					status: u.status,
					hidden,
				},
			});
			prune();
			return;
		}

		const now = Date.now();
		const show = own && !panelOpen.value && (!u.active || !hidden);
		const entry: Notification = existing ?? {
			id,
			type: u.type,
			message: u.message,
			createdAt: u.createdAt,
			updatedAt: now,
			popup: false,
			unread: false,
		};
		entry.type = u.type;
		entry.title = u.title;
		entry.message = u.message;
		entry.updatedAt = now;
		entry.popup = show;
		entry.unread = own && !u.active && !panelOpen.value;
		entry.task = {
			jobId: u.jobId,
			siteId: u.siteId,
			createdBy: u.createdBy,
			status: u.status,
			hidden,
		};
		if (!existing) entries.value.push(entry);

		clearTimer(id);
		if (show && !u.active) {
			startTimer(id, DEFAULT_DURATION[u.type as ToastType]);
		}
		if (!u.active) {
			hiddenJobs.delete(u.jobId);
			prune();
		}
	}

	/** Hides or shows a running task's sticky toast. */
	function setTaskHidden(id: string, hidden: boolean) {
		const e = find(id);
		if (!e?.task) return;
		e.task.hidden = hidden;
		if (hidden) remember(hiddenJobs, e.task.jobId);
		else hiddenJobs.delete(e.task.jobId);
		if (e.type === "running") e.popup = !hidden && !panelOpen.value;
		save();
	}

	/** Removes every finished entry from the history. */
	function clearFinished() {
		for (const e of entries.value) {
			if (e.type === "running") continue;
			clearTimer(e.id);
			if (e.task) remember(clearedJobs, e.task.jobId);
		}
		entries.value = entries.value.filter((e) => e.type === "running");
	}

	// --- Task center panel ---

	function setPanelOpen(open: boolean) {
		panelOpen.value = open;
		if (!open) return;
		// The panel shows everything the toasts would.
		for (const e of entries.value) {
			e.unread = false;
			if (e.popup && e.type !== "running") dismiss(e.id);
		}
	}

	/** Loads the logged-in user's persisted history. */
	function initialize() {
		if (loadedFor === username()) return;
		load();
	}

	/** Clears all state on logout, including the persisted history. */
	function reset() {
		// The user is usually already logged out here, so use the user the
		// history was loaded for.
		const key = loadedFor ? STORAGE_PREFIX + loadedFor : null;
		if (key) {
			try {
				localStorage.removeItem(key);
			} catch {
				// Ignore unavailable storage.
			}
		}
		for (const id of timers.keys()) clearTimer(id);
		loadedFor = null;
		hiddenJobs.clear();
		clearedJobs.clear();
		panelOpen.value = false;
		// Keep plain toasts that are still showing (e.g. a logout error).
		entries.value = entries.value.filter((e) => !e.task && e.popup);
		for (const e of entries.value) e.unread = false;
	}

	return {
		entries,
		running,
		recent,
		popups,
		unreadCount,
		panelOpen,
		notify,
		addToast,
		dismiss,
		pauseTimer,
		resumeTimer,
		upsertTask,
		setTaskHidden,
		clearFinished,
		setPanelOpen,
		initialize,
		reset,
	};
});
