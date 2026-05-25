import { ref, computed } from "vue";
import { defineStore } from "pinia";
import router from "../router";
import { BASE_URL } from "../utils/api";

const LS_TOKEN = "jman_auth_token";
const LS_USER = "jman_auth_user";
const LS_EXPIRES_AT = "jman_auth_expires_at";

let refreshTimeoutId: ReturnType<typeof setTimeout> | null = null;

export const useAuthStore = defineStore("auth", () => {
	// State
	const token = ref<string | null>(null);
	const user = ref<{
		username: string;
		displayName: string;
		level?: "basic" | "edit" | "execute" | "admin";
	} | null>(null);
	const expiresAt = ref<string | null>(null);

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

	const isAuthenticated = computed(() => {
		if (!token.value || !expiresAt.value) return false;
		return new Date(expiresAt.value) > new Date();
	});

	const authHeader = computed<Record<string, string>>(() => {
		if (!token.value) return {} as Record<string, string>;
		return { Authorization: `Bearer ${token.value}` };
	});

	// Helper
	function extractLevel(t: string): "basic" | "edit" | "execute" | "admin" {
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
		token.value = data.token;
		user.value = {
			...data.user,
			level: extractLevel(data.token),
		};
		expiresAt.value = data.expiresAt;

		localStorage.setItem(LS_TOKEN, data.token);
		localStorage.setItem(LS_USER, JSON.stringify(user.value));
		localStorage.setItem(LS_EXPIRES_AT, data.expiresAt);

		scheduleRefresh();
	}

	function logout() {
		token.value = null;
		user.value = null;
		expiresAt.value = null;

		localStorage.removeItem(LS_TOKEN);
		localStorage.removeItem(LS_USER);
		localStorage.removeItem(LS_EXPIRES_AT);

		if (refreshTimeoutId !== null) {
			clearTimeout(refreshTimeoutId);
			refreshTimeoutId = null;
		}

		router.push("/login");
	}

	async function refreshToken(): Promise<void> {
		try {
			const res = await fetch(`${BASE_URL}/auth/refresh`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					...authHeader.value,
				},
			});

			if (!res.ok) {
				logout();
				return;
			}

			const data = await res.json();

			token.value = data.token;
			expiresAt.value = data.expiresAt;

			if (user.value) {
				user.value.level = extractLevel(data.token);
				localStorage.setItem(LS_USER, JSON.stringify(user.value));
			}

			localStorage.setItem(LS_TOKEN, data.token);
			localStorage.setItem(LS_EXPIRES_AT, data.expiresAt);

			scheduleRefresh();
		} catch {
			logout();
		}
	}

	function scheduleRefresh() {
		if (refreshTimeoutId !== null) {
			clearTimeout(refreshTimeoutId);
			refreshTimeoutId = null;
		}

		if (!expiresAt.value) return;

		const expiresMs = new Date(expiresAt.value).getTime();
		const nowMs = Date.now();
		const fiveMinutes = 5 * 60 * 1000;
		const delay = expiresMs - nowMs - fiveMinutes;

		if (delay > 0) {
			refreshTimeoutId = setTimeout(() => {
				refreshToken();
			}, delay);
		} else {
			// Token expires in less than 5 minutes, refresh immediately
			refreshToken();
		}
	}

	function initialize() {
		const storedToken = localStorage.getItem(LS_TOKEN);
		const storedUser = localStorage.getItem(LS_USER);
		const storedExpiresAt = localStorage.getItem(LS_EXPIRES_AT);

		if (storedToken && storedUser && storedExpiresAt) {
			token.value = storedToken;
			try {
				const parsedUser = JSON.parse(storedUser);
				// Ensure level is present if we just upgraded
				if (!parsedUser.level) {
					parsedUser.level = extractLevel(storedToken);
				}
				user.value = parsedUser;
			} catch {
				logout();
				return;
			}
			expiresAt.value = storedExpiresAt;

			if (new Date(storedExpiresAt) > new Date()) {
				scheduleRefresh();
			} else {
				logout();
			}
		}
	}

	return {
		// State
		token,
		user,
		expiresAt,
		// Getters
		isAuthenticated,
		authHeader,
		userLevel,
		canEdit,
		canExecute,
		canAdmin,
		// Actions
		login,
		logout,
		refreshToken,
		scheduleRefresh,
		initialize,
	};
});
