import { ref } from "vue";
import { defineStore } from "pinia";
import type { ReportMeta, ReportResult } from "../types";
import { useAuthStore } from "./auth";
import { handleErrorResponse, BASE_URL } from "../utils/api";

export const useReportsStore = defineStore("reports", () => {
	const authStore = useAuthStore();

	const reports = ref<ReportMeta[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchReports() {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await fetch(`${BASE_URL}/reports`, {
				headers: authStore.authHeader,
			});
			if (!res.ok) await handleErrorResponse(res);
			reports.value = await res.json();
		} catch (e: any) {
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	function getReport(id: string): ReportMeta | undefined {
		return reports.value.find((r) => r.id === id);
	}

	async function runReport(
		id: string,
		params: Record<string, string>,
	): Promise<ReportResult> {
		const url = new URL(
			`${BASE_URL}/reports/${id}/run`,
			window.location.origin,
		);
		for (const [key, value] of Object.entries(params)) {
			if (value) url.searchParams.append(key, value);
		}

		const res = await fetch(url.toString(), {
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	return { reports, isLoading, error, fetchReports, getReport, runReport };
});
