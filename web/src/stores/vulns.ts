import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	VulnExposureGroup,
	VulnExposureKind,
	VulnExposureSummary,
	VulnMute,
} from "../types";
import { BASE_URL, apiFetch, handleErrorResponse } from "../utils/api";

/** Vulnerability exposures (vulnerable plugins and core per site) and mutes. */
export const useVulnStore = defineStore("vulns", () => {
	const summary = ref<VulnExposureSummary | null>(null);
	const openGroups = ref<VulnExposureGroup[]>([]);
	const resolvedGroups = ref<VulnExposureGroup[]>([]);
	const openMutes = ref<VulnMute[]>([]);
	const liftedMutes = ref<VulnMute[]>([]);

	async function getJSON<T>(path: string): Promise<T> {
		const res = await apiFetch(`${BASE_URL}${path}`);
		if (!res.ok) await handleErrorResponse(res);
		return res.json();
	}

	async function send<T>(
		method: string,
		path: string,
		body?: unknown,
	): Promise<T | null> {
		const res = await apiFetch(`${BASE_URL}${path}`, {
			method,
			headers: { "Content-Type": "application/json" },
			body: body === undefined ? undefined : JSON.stringify(body),
		});
		if (!res.ok) await handleErrorResponse(res);
		return res.status === 204 ? null : res.json();
	}

	/** Counts for the badges; failures leave the last counts in place. */
	async function fetchSummary() {
		try {
			summary.value = await getJSON<VulnExposureSummary>(
				"/vulns/exposures/summary",
			);
		} catch (error) {
			console.error("Failed to fetch vulnerability summary:", error);
		}
	}

	async function fetchOpenGroups() {
		openGroups.value =
			(await getJSON<VulnExposureGroup[]>("/vulns/exposures")) ?? [];
	}

	async function fetchResolvedGroups(days = 30) {
		resolvedGroups.value =
			(await getJSON<VulnExposureGroup[]>(
				`/vulns/exposures?status=resolved&days=${days}`,
			)) ?? [];
	}

	async function fetchMutes() {
		const [open, lifted] = await Promise.all([
			getJSON<VulnMute[]>("/vulns/mutes"),
			getJSON<VulnMute[]>("/vulns/mutes?status=lifted&days=30"),
		]);
		openMutes.value = open ?? [];
		liftedMutes.value = lifted ?? [];
	}

	async function mute(
		kind: VulnExposureKind,
		slug: string,
		siteId: string,
		reason: string,
	) {
		await send("POST", "/vulns/mutes", {
			kind,
			slug,
			site_id: siteId,
			reason,
		});
		await fetchSummary();
	}

	async function confirmMute(id: number, reason: string) {
		await send("POST", `/vulns/mutes/${id}/confirm`, { reason });
		await fetchSummary();
	}

	async function unmute(id: number) {
		await send("DELETE", `/vulns/mutes/${id}`);
		await fetchSummary();
	}

	return {
		summary,
		openGroups,
		resolvedGroups,
		openMutes,
		liftedMutes,
		fetchSummary,
		fetchOpenGroups,
		fetchResolvedGroups,
		fetchMutes,
		mute,
		confirmMute,
		unmute,
	};
});
