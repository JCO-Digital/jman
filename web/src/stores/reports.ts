import { ref } from "vue";
import { defineStore } from "pinia";
import type { ReportMeta, ReportResult } from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const useReportsStore = defineStore("reports", () => {
	const reports = ref<ReportMeta[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchReports() {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(`${BASE_URL}/reports`);
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

		const res = await apiFetch(url.toString(), {});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	return { reports, isLoading, error, fetchReports, getReport, runReport };
});
