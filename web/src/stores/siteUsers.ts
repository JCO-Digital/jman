import { ref } from "vue";
import { defineStore } from "pinia";
import type { SiteUser, SiteUserList, SiteUserSummary } from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

/** WordPress users of the managed sites (jman's own users are in the user store). */
export const useSiteUsersStore = defineStore("siteUsers", () => {
	const users = ref<SiteUser[]>([]);
	const summaries = ref<SiteUserSummary[]>([]);
	const isLoading = ref(false);
	const isLoaded = ref(false);
	const error = ref<string | null>(null);

	async function fetchAll(force = false) {
		if (isLoading.value || (isLoaded.value && !force)) return;
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(`${BASE_URL}/site-users`);
			if (!res.ok) await handleErrorResponse(res);
			const data: { users: SiteUser[]; summaries: SiteUserSummary[] } =
				await res.json();
			users.value = data.users ?? [];
			summaries.value = data.summaries ?? [];
			isLoaded.value = true;
		} catch (e: any) {
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	async function fetchSiteUsers(siteId: string): Promise<SiteUserList> {
		const res = await apiFetch(
			`${BASE_URL}/sites/${encodeURIComponent(siteId)}/users`,
		);
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	return {
		users,
		summaries,
		isLoading,
		isLoaded,
		error,
		fetchAll,
		fetchSiteUsers,
	};
});
