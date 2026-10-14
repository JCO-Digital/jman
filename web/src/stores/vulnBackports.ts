import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	VulnBackport,
	CreateVulnBackportPayload,
	CreateVulnBackportResponse,
} from "../types";
import { useDataStore } from "./data";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const useVulnBackportStore = defineStore("vulnBackports", () => {
	const rules = ref<VulnBackport[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchRules() {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(`${BASE_URL}/vuln-backports`);
			if (!res.ok) await handleErrorResponse(res);
			rules.value = (await res.json()) || [];
			return rules.value;
		} catch (e: any) {
			error.value = e.message;
			throw e;
		} finally {
			isLoading.value = false;
		}
	}

	// Rules change which sites count as affected, so the vulnerability
	// data is reloaded after every change.
	async function addRule(
		payload: CreateVulnBackportPayload,
	): Promise<CreateVulnBackportResponse> {
		const res = await apiFetch(`${BASE_URL}/vuln-backports`, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(payload),
		});
		if (!res.ok) await handleErrorResponse(res);
		const data: CreateVulnBackportResponse = await res.json();
		rules.value = [...rules.value, data.rule];
		await useDataStore().fetchVulnerabilities();
		return data;
	}

	async function deleteRule(id: number) {
		const res = await apiFetch(`${BASE_URL}/vuln-backports/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
		rules.value = rules.value.filter((r) => r.id !== id);
		await useDataStore().fetchVulnerabilities();
	}

	return { rules, isLoading, error, fetchRules, addRule, deleteRule };
});
