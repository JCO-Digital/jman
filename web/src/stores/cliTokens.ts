import { ref } from "vue";
import { defineStore } from "pinia";
import type { CliToken } from "../types";
import { BASE_URL, handleErrorResponse, apiFetch } from "../utils/api";

export const useCliTokensStore = defineStore("cliTokens", () => {
	const tokens = ref<CliToken[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// fetchTokens loads the user's CLI tokens, or every user's when all is
	// set (admins only).
	async function fetchTokens(all = false) {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(
				`${BASE_URL}/cli/tokens${all ? "?all=true" : ""}`,
			);
			if (!res.ok) await handleErrorResponse(res);
			tokens.value = await res.json();
		} catch (e: any) {
			error.value = e.message || "Failed to load CLI tokens";
		} finally {
			isLoading.value = false;
		}
	}

	async function revokeToken(id: number): Promise<void> {
		const res = await apiFetch(`${BASE_URL}/cli/tokens/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
		tokens.value = tokens.value.filter((t) => t.id !== id);
	}

	// authorize approves a CLI login and returns the one-time code to hand
	// to the CLI.
	async function authorize(
		challenge: string,
		deviceName: string,
	): Promise<string> {
		const res = await apiFetch(`${BASE_URL}/cli/authorize`, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ challenge, device_name: deviceName }),
		});
		if (!res.ok) await handleErrorResponse(res);
		const data: { code: string } = await res.json();
		return data.code;
	}

	return { tokens, isLoading, error, fetchTokens, revokeToken, authorize };
});
