import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	InventoryItem,
	InventoryItemPayload,
	InventoryItemResponse,
	InventoryLinkPayload,
	InventorySyncResult,
	InventoryTypeInfo,
} from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const useInventoryStore = defineStore("inventory", () => {
	const items = ref<InventoryItem[]>([]);
	const types = ref<InventoryTypeInfo[]>([]);
	const providers = ref<string[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// The list request in flight. A new fetchItems aborts it, so a slow
	// response to an older search can't replace a newer one.
	let itemsRequest: AbortController | null = null;

	async function fetchItems(params?: {
		type?: string;
		status?: string;
		search?: string;
	}) {
		itemsRequest?.abort();
		const request = new AbortController();
		itemsRequest = request;
		isLoading.value = true;
		error.value = null;
		try {
			const url = new URL(
				`${BASE_URL}/inventory/items`,
				window.location.origin,
			);
			if (params?.type) url.searchParams.append("type", params.type);
			if (params?.status)
				url.searchParams.append("status", params.status);
			if (params?.search)
				url.searchParams.append("search", params.search);
			const res = await apiFetch(url.toString(), {
				signal: request.signal,
			});
			if (!res.ok) await handleErrorResponse(res);
			const data = await res.json();
			if (itemsRequest !== request) return;
			items.value = data;
		} catch (e: any) {
			if (e.name === "AbortError" || itemsRequest !== request) return;
			error.value = e.message;
			console.error(e);
		} finally {
			if (itemsRequest === request) {
				itemsRequest = null;
				isLoading.value = false;
			}
		}
	}

	async function fetchTypes() {
		try {
			const res = await apiFetch(`${BASE_URL}/inventory/types`, {});
			if (!res.ok) await handleErrorResponse(res);
			types.value = await res.json();
		} catch (e) {
			console.error(e);
		}
	}

	async function fetchProviders() {
		try {
			const res = await apiFetch(`${BASE_URL}/inventory/providers`, {});
			if (!res.ok) await handleErrorResponse(res);
			providers.value = (await res.json()).providers ?? [];
		} catch (e) {
			console.error(e);
		}
	}

	async function getItem(id: string): Promise<InventoryItemResponse | null> {
		try {
			const res = await apiFetch(`${BASE_URL}/inventory/items/${id}`);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	/** Returns the existing item's ID on a 409 type+name conflict, else null. */
	async function createItem(payload: InventoryItemPayload): Promise<{
		item?: InventoryItem;
		conflictItemId?: string;
		error?: string;
	}> {
		try {
			const res = await apiFetch(`${BASE_URL}/inventory/items`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(payload),
			});
			if (res.status === 409) {
				const data = await res.json();
				return { conflictItemId: data.existing_item_id };
			}
			if (!res.ok) await handleErrorResponse(res);
			return { item: await res.json() };
		} catch (e: any) {
			return { error: e.message };
		}
	}

	async function updateItem(
		id: string,
		payload: InventoryItemPayload,
	): Promise<InventoryItem | null> {
		try {
			const res = await apiFetch(`${BASE_URL}/inventory/items/${id}`, {
				method: "PATCH",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(payload),
			});
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function deleteItem(id: string): Promise<string | null> {
		try {
			const res = await apiFetch(`${BASE_URL}/inventory/items/${id}`, {
				method: "DELETE",
			});
			if (!res.ok) await handleErrorResponse(res);
			return null;
		} catch (e: any) {
			return e.message;
		}
	}

	async function linkItem(
		id: string,
		payload: InventoryLinkPayload,
	): Promise<InventoryItem | null> {
		try {
			const res = await apiFetch(
				`${BASE_URL}/inventory/items/${id}/links`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify(payload),
				},
			);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function unlinkItem(
		id: string,
		link: InventoryLinkPayload,
	): Promise<InventoryItem | null> {
		try {
			const res = await apiFetch(
				`${BASE_URL}/inventory/items/${id}/links/${encodeURIComponent(link.provider)}/${encodeURIComponent(link.provider_type)}/${encodeURIComponent(link.provider_id)}`,
				{ method: "DELETE" },
			);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function dismissMatch(id: string, link: InventoryLinkPayload) {
		try {
			const res = await apiFetch(
				`${BASE_URL}/inventory/items/${id}/dismiss`,
				{
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify(link),
				},
			);
			if (!res.ok) await handleErrorResponse(res);
		} catch (e) {
			console.error(e);
		}
	}

	async function syncProvider(
		provider: string,
	): Promise<InventorySyncResult | null> {
		try {
			const res = await apiFetch(
				`${BASE_URL}/inventory/sync/${provider}`,
				{
					method: "POST",
				},
			);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	return {
		items,
		types,
		providers,
		isLoading,
		error,
		fetchItems,
		fetchTypes,
		fetchProviders,
		getItem,
		createItem,
		updateItem,
		deleteItem,
		linkItem,
		unlinkItem,
		dismissMatch,
		syncProvider,
	};
});
