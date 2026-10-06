import { ref, computed } from "vue";
import { defineStore } from "pinia";
import router from "../router";
import { BASE_URL } from "../utils/api";

// Keys of the pre-cookie session, which kept a 24h access token in
// localStorage. Cleared on startup so old tokens don't linger.
const LEGACY_LS_KEYS = [
	"jman_auth_token",
	"jman_auth_user",
	"jman_auth_expires_at",
];

// Refresh this long before the access token expires.
const REFRESH_MARGIN_MS = 60 * 1000;
// Retry delay after a refresh that failed for a reason other than an
// expired session (e.g. the API was briefly unreachable).
const RETRY_DELAY_MS = 30 * 1000;

type UserLevel = "basic" | "edit" | "execute" | "admin";

interface AuthUser {
	username: string;
	displayName: string;
	level?: UserLevel;
}

interface TokenResponse {
	token: string;
	expiresAt: string;
	user: AuthUser;
}

let refreshTimeoutId: ReturnType<typeof setTimeout> | null = null;
let refreshInFlight: Promise<boolean> | null = null;

// Tells other tabs about logins and logouts so they follow along.
const channel =
	typeof BroadcastChannel !== "undefined"
		? new BroadcastChannel("jman-auth")
		: null;

/**
 * Auth state. The access token (a short-lived JWT) lives only in memory.
 * The long-lived refresh token is an httpOnly cookie scoped to /api/auth,
 * so page scripts never see it; POST /auth/refresh trades it for a new
 * access token and rotates it.
 */
export const useAuthStore = defineStore("auth", () => {
	// State
	const token = ref<string | null>(null);
	const user = ref<AuthUser | null>(null);
	const expiresAt = ref<string | null>(null);

	// Resolves once the startup session restore has finished.
	let readyResolve: () => void = () => {};
	const ready = new Promise<void>((resolve) => {
		readyResolve = resolve;
	});

	// Getters
	const userLevel = computed(() => {
		return user.value?.level || "basic";
	});

	const canEdit = computed(() => {
		const l = userLevel.value;
		return l === "edit" || l === "execute" || l === "admin";
	});

	const canExecute = computed(() => {
		const l = userLevel.value;
		return l === "execute" || l === "admin";
	});

	const canAdmin = computed(() => {
		return userLevel.value === "admin";
	});

	// True while a session exists. The access token may be momentarily
	// expired; apiFetch refreshes it before the next request.
	const isAuthenticated = computed(() => !!token.value && !!user.value);

	// Helper
	function extractLevel(t: string): UserLevel {
		try {
			const parts = t.split(".");
			const payloadPart = parts[1];
			if (!payloadPart) return "basic";
			const payload = JSON.parse(
				atob(payloadPart.replace(/-/g, "+").replace(/_/g, "/")),
			);
			return payload.level || "basic";
		} catch {
			return "basic";
		}
	}

	function applyTokenResponse(data: TokenResponse) {
		token.value = data.token;
		expiresAt.value = data.expiresAt;
		user.value = { ...data.user, level: extractLevel(data.token) };
		scheduleRefresh();
	}

	function clearSession() {
		token.value = null;
		user.value = null;
		expiresAt.value = null;
		if (refreshTimeoutId !== null) {
			clearTimeout(refreshTimeoutId);
			refreshTimeoutId = null;
		}
	}

	function redirectToLogin() {
		const current = router.currentRoute.value;
		if (!current.meta.public) {
			router.push("/login");
		}
	}

	// Actions
	async function login(
		username: string,
		password: string,
		totp?: string,
	): Promise<void> {
		const body: Record<string, string> = { username, password };
		if (totp) {
			body.totp = totp;
		}

		const res = await fetch(`${BASE_URL}/auth/login`, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body),
			credentials: "include",
		});

		const contentType = res.headers.get("content-type") || "";
		let data: any = null;
		let rawBody: string | null = null;

		if (contentType.includes("application/json")) {
			try {
				data = await res.json();
			} catch {
				// If the server indicates failure but we cannot parse JSON,
				// treat it as a generic login failure instead of surfacing
				// a JSON parsing error to the UI.
				if (!res.ok) {
					throw new Error("Login failed");
				}
				// For a successful status with invalid JSON, this is an
				// unexpected server response.
				throw new Error("Unexpected server response");
			}
		} else {
			try {
				rawBody = await res.text();
			} catch {
				rawBody = null;
			}
		}

		if (!res.ok) {
			if (rawBody) {
				console.error("Login failed (server response):", rawBody);
			}
			const message = (data && data.error) || "Login failed";
			throw new Error(message);
		}

		if (!data) {
			throw new Error("Unexpected server response");
		}
		applyTokenResponse(data);
		channel?.postMessage("login");
	}

	function logout() {
		// Revoke the session server-side; the cookie is cleared either way.
		fetch(`${BASE_URL}/auth/logout`, {
			method: "POST",
			credentials: "include",
		}).catch(() => {});
		clearSession();
		channel?.postMessage("logout");
		router.push("/login");
	}

	/**
	 * Trades the refresh token cookie for a new access token. Concurrent
	 * callers share one request. Resolves to whether a session is active
	 * afterwards: a rejected refresh ends the session, while a network
	 * failure keeps it and retries later.
	 */
	function refreshToken(): Promise<boolean> {
		if (refreshInFlight) return refreshInFlight;

		refreshInFlight = (async () => {
			let res: Response;
			try {
				res = await fetch(`${BASE_URL}/auth/refresh`, {
					method: "POST",
					credentials: "include",
				});
			} catch {
				scheduleRetry();
				return isAuthenticated.value;
			}

			if (res.status === 401) {
				const hadSession = isAuthenticated.value;
				clearSession();
				if (hadSession) redirectToLogin();
				return false;
			}
			if (!res.ok) {
				scheduleRetry();
				return isAuthenticated.value;
			}

			applyTokenResponse(await res.json());
			return true;
		})().finally(() => {
			refreshInFlight = null;
		});

		return refreshInFlight;
	}

	function msUntilExpiry(): number {
		if (!expiresAt.value) return 0;
		return new Date(expiresAt.value).getTime() - Date.now();
	}

	/**
	 * Returns an access token that is valid for at least the refresh margin,
	 * refreshing first if needed. Null when there is no session.
	 */
	async function getValidToken(): Promise<string | null> {
		await ready;
		if (!token.value) return null;
		if (msUntilExpiry() <= REFRESH_MARGIN_MS) {
			await refreshToken();
		}
		return token.value;
	}

	function scheduleRefresh() {
		if (refreshTimeoutId !== null) {
			clearTimeout(refreshTimeoutId);
			refreshTimeoutId = null;
		}
		if (!expiresAt.value) return;

		const delay = Math.max(msUntilExpiry() - REFRESH_MARGIN_MS, 0);
		refreshTimeoutId = setTimeout(() => {
			refreshToken();
		}, delay);
	}

	function scheduleRetry() {
		if (refreshTimeoutId !== null) clearTimeout(refreshTimeoutId);
		refreshTimeoutId = setTimeout(() => {
			refreshToken();
		}, RETRY_DELAY_MS);
	}

	// Timers stall while a laptop sleeps or a tab is in the background, so
	// check the token whenever the page becomes active again.
	function refreshIfStale() {
		if (token.value && msUntilExpiry() <= REFRESH_MARGIN_MS) {
			refreshToken();
		}
	}

	function setDisplayName(name: string) {
		if (user.value) {
			user.value.displayName = name;
		}
	}

	/** Restores the session from the refresh token cookie, if any. */
	async function initialize(): Promise<void> {
		try {
			for (const key of LEGACY_LS_KEYS) localStorage.removeItem(key);
		} catch {
			// Storage may be unavailable; nothing to clean up then.
		}

		document.addEventListener("visibilitychange", () => {
			if (document.visibilityState === "visible") refreshIfStale();
		});
		window.addEventListener("focus", refreshIfStale);
		window.addEventListener("online", refreshIfStale);

		if (channel) {
			channel.onmessage = (e) => {
				if (e.data === "logout") {
					clearSession();
					redirectToLogin();
				} else if (e.data === "login" && !isAuthenticated.value) {
					refreshToken().then((ok) => {
						if (ok && router.currentRoute.value.name === "login") {
							router.push("/");
						}
					});
				}
			};
		}

		try {
			await refreshToken();
		} finally {
			readyResolve();
		}
	}

	return {
		// State
		token,
		user,
		expiresAt,
		ready,
		// Getters
		isAuthenticated,
		userLevel,
		canEdit,
		canExecute,
		canAdmin,
		// Actions
		login,
		logout,
		refreshToken,
		getValidToken,
		initialize,
		setDisplayName,
	};
});
