import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type {
	AdminUser,
	CreateUserPayload,
	UpdateUserPayload,
	TwoFactorSetupResponse,
	UserProfile,
} from "../types";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

// Deprecated: use AdminUser from ../types instead. Kept for backward compatibility.
export interface User {
	username: string;
	displayName: string;
}

export const useUserStore = defineStore("user", () => {
	const authStore = useAuthStore();

	const users = ref<AdminUser[]>([]);
	const profile = ref<UserProfile | null>(null);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	const userMap = computed(() => {
		const map = new Map<string, string>();
		users.value.forEach((user) => {
			map.set(user.username, user.displayName);
		});
		return map;
	});

	// ---------------------------------------------------------------------------
	// Helpers
	// ---------------------------------------------------------------------------

	async function handleErrorResponse(res: Response): Promise<never> {
		if (res.status === 401) {
			authStore.logout();
			throw new Error("Unauthorized");
		}
		let message: string;
		try {
			const data = await res.json();
			message = data.error || `Request failed (${res.status})`;
		} catch {
			message = `Request failed (${res.status})`;
		}
		throw new Error(message);
	}

	// ---------------------------------------------------------------------------
	// Fetch current user profile
	// ---------------------------------------------------------------------------

	async function fetchProfile(): Promise<UserProfile | null> {
		if (!authStore.isAuthenticated) return null;
		try {
			const res = await fetch(`${BASE_URL}/user/profile`, {
				headers: authStore.authHeader,
			});
			if (res.status === 401) {
				authStore.logout();
				return null;
			}
			if (!res.ok) throw new Error("Failed to fetch profile");
			const data: UserProfile = await res.json();
			profile.value = data;
			return data;
		} catch (e: any) {
			console.error("Error fetching profile:", e);
			return null;
		}
	}

	// ---------------------------------------------------------------------------
	// Fetch users list
	// ---------------------------------------------------------------------------

	async function fetchUsers() {
		if (!authStore.isAuthenticated) return;
		isLoading.value = true;
		error.value = null;
		try {
			const res = await fetch(`${BASE_URL}/users`, {
				headers: authStore.authHeader,
			});
			if (res.status === 401) {
				authStore.logout();
				return;
			}
			if (!res.ok) throw new Error("Failed to fetch users");
			const data = await res.json();
			users.value = data;
		} catch (e: any) {
			error.value = e.message;
			console.error("Error fetching users:", e);
		} finally {
			isLoading.value = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Admin CRUD actions
	// ---------------------------------------------------------------------------

	async function createUser(payload: CreateUserPayload): Promise<void> {
		const res = await fetch(`${BASE_URL}/users`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(payload),
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		await fetchUsers();
	}

	async function updateUser(
		username: string,
		payload: UpdateUserPayload,
	): Promise<void> {
		const res = await fetch(
			`${BASE_URL}/users/${encodeURIComponent(username)}`,
			{
				method: "PATCH",
				headers: {
					"Content-Type": "application/json",
					...authStore.authHeader,
				},
				body: JSON.stringify(payload),
			},
		);
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		await fetchUsers();
	}

	async function deleteUser(username: string): Promise<void> {
		const res = await fetch(
			`${BASE_URL}/users/${encodeURIComponent(username)}`,
			{
				method: "DELETE",
				headers: authStore.authHeader,
			},
		);
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		await fetchUsers();
	}

	// ---------------------------------------------------------------------------
	// Self-service actions
	// ---------------------------------------------------------------------------

	async function updateProfile(displayName: string): Promise<void> {
		const res = await fetch(`${BASE_URL}/user/profile`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ displayName }),
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		// Update auth store user and persist to localStorage
		if (authStore.user) {
			authStore.user.displayName = displayName;
			localStorage.setItem("jman_auth_user", JSON.stringify(authStore.user));
		}
	}

	async function changePassword(
		currentPassword: string,
		newPassword: string,
	): Promise<void> {
		const res = await fetch(`${BASE_URL}/user/password`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ currentPassword, newPassword }),
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
	}

	async function setup2FA(): Promise<TwoFactorSetupResponse> {
		const res = await fetch(`${BASE_URL}/user/2fa/setup`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		return await res.json();
	}

	async function activate2FA(secret: string, code: string): Promise<void> {
		const res = await fetch(`${BASE_URL}/user/2fa/activate`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ secret, code }),
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		// Update local profile state
		if (profile.value) {
			profile.value.has2FA = true;
		}
	}

	async function deactivate2FA(code: string): Promise<void> {
		const res = await fetch(`${BASE_URL}/user/2fa/deactivate`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ code }),
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		// Update local profile state
		if (profile.value) {
			profile.value.has2FA = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Utilities
	// ---------------------------------------------------------------------------

	function resolveDisplayName(username: string | undefined | null): string {
		if (!username) return "—";
		return userMap.value.get(username) || username;
	}

	function clearCache() {
		users.value = [];
		profile.value = null;
		error.value = null;
	}

	return {
		// State
		users,
		profile,
		isLoading,
		error,
		// Getters
		userMap,
		// Profile
		fetchProfile,
		// List
		fetchUsers,
		// Admin CRUD
		createUser,
		updateUser,
		deleteUser,
		// Self-service
		updateProfile,
		changePassword,
		setup2FA,
		activate2FA,
		deactivate2FA,
		// Utilities
		resolveDisplayName,
		clearCache,
	};
});
