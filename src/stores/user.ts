import { ref, computed } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export interface User {
	username: string;
	displayName: string;
}

export const useUserStore = defineStore("user", () => {
	const authStore = useAuthStore();

	const users = ref<User[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	/**
	 * Map for quick lookup of display names by username
	 */
	const userMap = computed(() => {
		const map = new Map<string, string>();
		users.value.forEach((user) => {
			map.set(user.username, user.displayName);
		});
		return map;
	});

	/**
	 * Fetches the list of users from the backend to resolve display names
	 */
	async function fetchUsers() {
		// Only fetch if authenticated
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

	/**
	 * Resolves a username to its displayName, falling back to the username itself
	 * if the user is not found in the local cache.
	 */
	function resolveDisplayName(username: string | undefined | null): string {
		if (!username) return "—";
		return userMap.value.get(username) || username;
	}

	function clearCache() {
		users.value = [];
		error.value = null;
	}

	return {
		// State
		users,
		isLoading,
		error,
		// Getters
		userMap,
		// Actions
		fetchUsers,
		resolveDisplayName,
		clearCache,
	};
});
