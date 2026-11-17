import { defineStore } from "pinia";
import type { SiteDiffPage, TextDiffDirection, TextDiffMute } from "../types";
import { BASE_URL, apiFetch, handleErrorResponse } from "../utils/api";

async function request<T>(
	path: string,
	init?: RequestInit,
): Promise<T | undefined> {
	const res = await apiFetch(`${BASE_URL}${path}`, init);
	if (!res.ok) await handleErrorResponse(res);
	if (res.status === 204) return undefined;
	return (await res.json()) as T;
}

const json = (method: string, body: unknown): RequestInit => ({
	method,
	headers: { "Content-Type": "application/json" },
	body: JSON.stringify(body),
});

/**
 * The update text diff's per-site settings: the extra pages compared
 * besides the frontpage, and mutes (docs/update-text-diff.md). Nothing is
 * cached; callers load what they show.
 */
export const useTextDiffStore = defineStore("textDiff", () => {
	async function fetchPages(siteId: string): Promise<SiteDiffPage[]> {
		return (
			(await request<SiteDiffPage[]>(
				`/sites/${siteId}/text-diff-pages`,
			)) ?? []
		);
	}

	async function addPage(
		siteId: string,
		path: string,
	): Promise<SiteDiffPage> {
		return (await request<SiteDiffPage>(
			`/sites/${siteId}/text-diff-pages`,
			json("POST", { path }),
		))!;
	}

	async function deletePage(siteId: string, pageId: number): Promise<void> {
		await request(`/sites/${siteId}/text-diff-pages/${pageId}`, {
			method: "DELETE",
		});
	}

	/** All of a site's mutes, expired ones included. */
	async function fetchMutes(siteId: string): Promise<TextDiffMute[]> {
		return (
			(await request<TextDiffMute[]>(
				`/sites/${siteId}/text-diff-mutes`,
			)) ?? []
		);
	}

	/**
	 * Mutes a changed line as shown in a diff; jman-api stores it
	 * normalized. days 0 uses jman-api's default.
	 */
	async function createMute(
		siteId: string,
		mute: {
			path: string;
			direction: TextDiffDirection;
			text: string;
			reason?: string;
			days?: number;
		},
	): Promise<TextDiffMute> {
		return (await request<TextDiffMute>(
			`/sites/${siteId}/text-diff-mutes`,
			json("POST", mute),
		))!;
	}

	/** Ends a mute now; jman-api keeps it for the record. */
	async function deleteMute(muteId: number): Promise<void> {
		await request(`/text-diff-mutes/${muteId}`, { method: "DELETE" });
	}

	return {
		fetchPages,
		addPage,
		deletePage,
		fetchMutes,
		createMute,
		deleteMute,
	};
});
