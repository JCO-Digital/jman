import { ref, computed } from "vue";
import { defineStore } from "pinia";
import router from "../router";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

const LS_TOKEN = "jman_auth_token";
const LS_USER = "jman_auth_user";
const LS_EXPIRES_AT = "jman_auth_expires_at";

let refreshTimeoutId: ReturnType<typeof setTimeout> | null = null;

export const useAuthStore = defineStore("auth", () => {
	// State
	const token = ref<string | null>(null);
	const user = ref<{ username: string; displayName: string } | null>(null);
	const expiresAt = ref<string | null>(null);

	// Getters
	const isAuthenticated = computed(() => {
		if (!token.value || !expiresAt.value) return false;
		return new Date(expiresAt.value) > new Date();
	});

	const authHeader = computed<Record<string, string>>(() => {
		if (!token.value) return {} as Record<string, string>;
		return { Authorization: `Bearer ${token.value}` };
	});

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

		const data = await res.json();

		if (!res.ok) {
			throw new Error(data.error || "Login failed");
		}

		token.value = data.token;
		user.value = data.user;
		expiresAt.value = data.expiresAt;

		localStorage.setItem(LS_TOKEN, data.token);
		localStorage.setItem(LS_USER, JSON.stringify(data.user));
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
				user.value = JSON.parse(storedUser);
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
		// Actions
		login,
		logout,
		refreshToken,
		scheduleRefresh,
		initialize,
	};
});
