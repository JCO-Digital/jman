import type { Router } from "vue-router";

// Reloads the page when jman-api reports a different version than when
// the page loaded, i.e. after a deploy. An old bundle keeps requesting
// lazy-loaded chunks that no longer exist, so it breaks until reloaded.

const VERSION_HEADER = "X-Jman-Version";
// Guards against reload loops when a reload doesn't fix things.
const RELOAD_GUARD_KEY = "jman_update_reload_at";
const RELOAD_GUARD_MS = 30 * 1000;

let loadedVersion: string | null = null;
let updatePending = false;

function reload(url?: string): boolean {
	try {
		const last = Number(sessionStorage.getItem(RELOAD_GUARD_KEY));
		if (last && Date.now() - last < RELOAD_GUARD_MS) return false;
		sessionStorage.setItem(RELOAD_GUARD_KEY, String(Date.now()));
	} catch {
		// Without storage there is no loop guard; reload anyway.
	}
	if (url) {
		window.location.assign(url);
	} else {
		window.location.reload();
	}
	return true;
}

/** Records the API version from a response and notices deploys. */
export function noteServerVersion(res: Response) {
	const version = res.headers.get(VERSION_HEADER);
	if (!version) return;
	if (loadedVersion === null) {
		loadedVersion = version;
		return;
	}
	if (version === loadedVersion || updatePending) return;

	updatePending = true;
	// Reloading a background tab is invisible to the user. A visible tab
	// reloads on its next navigation, so a half-filled form isn't lost.
	if (document.visibilityState === "hidden") reload();
}

function isChunkLoadError(err: unknown): boolean {
	const message = err instanceof Error ? err.message : String(err);
	return /dynamically imported module|Importing a module script failed|error loading dynamically imported module/i.test(
		message,
	);
}

export function installAppUpdateHandlers(router: Router) {
	router.beforeEach((to) => {
		if (updatePending && reload(router.resolve(to).href)) return false;
		return true;
	});

	document.addEventListener("visibilitychange", () => {
		if (updatePending && document.visibilityState === "hidden") reload();
	});

	// A chunk of the old build failed to load: the web UI was deployed
	// without an API version change. Load the target page fresh.
	window.addEventListener("vite:preloadError", (event) => {
		if (reload()) event.preventDefault();
	});
	router.onError((err, to) => {
		if (isChunkLoadError(err)) reload(router.resolve(to).href);
	});
}
