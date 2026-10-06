import { ref } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type {
	AgentToken,
	CreateAgentTokenPayload,
	CreatedAgentToken,
} from "../types";
import { BASE_URL, handleErrorResponse, apiFetch } from "../utils/api";

export const useAgentTokensStore = defineStore("agentTokens", () => {
	const authStore = useAuthStore();

	const tokens = ref<AgentToken[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// ---------------------------------------------------------------------------
	// Fetch tokens list
	// ---------------------------------------------------------------------------

	async function fetchTokens() {
		if (!authStore.isAuthenticated) return;
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(`${BASE_URL}/agent-tokens`);
			if (res.status === 401) {
				authStore.logout();
				return;
			}
			if (!res.ok) throw new Error("Failed to fetch agent tokens");
			const data: AgentToken[] = await res.json();
			tokens.value = data;
		} catch (e: any) {
			error.value = e.message || "Failed to load agent tokens";
			console.error("Error fetching agent tokens:", e);
		} finally {
			isLoading.value = false;
		}
	}

	// ---------------------------------------------------------------------------
	// Admin CRUD actions
	// ---------------------------------------------------------------------------

	async function createToken(
		serverId: string,
		serverName: string,
		description?: string,
	): Promise<CreatedAgentToken> {
		const payload: CreateAgentTokenPayload = {
			server_id: serverId,
			server_name: serverName,
			description: description || undefined,
		};
		const res = await apiFetch(`${BASE_URL}/agent-tokens`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(payload),
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		const created: CreatedAgentToken = await res.json();
		await fetchTokens();
		return created;
	}

	async function revokeToken(id: number): Promise<void> {
		const res = await apiFetch(`${BASE_URL}/agent-tokens/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) {
			await handleErrorResponse(res);
		}
		await fetchTokens();
	}

	function clearCache() {
		tokens.value = [];
		error.value = null;
	}

	return {
		// State
		tokens,
		isLoading,
		error,
		// Actions
		fetchTokens,
		createToken,
		revokeToken,
		clearCache,
	};
});
