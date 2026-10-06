import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	Asset,
	OrganizationAsset,
	AssetPayment,
	EnrichedOrganizationAsset,
} from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const useAssetStore = defineStore("asset", () => {
	const assets = ref<Asset[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	// Template Management
	async function fetchAssets(signal?: AbortSignal) {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(`${BASE_URL}/assets`, {
				signal,
			});

			if (!res.ok) await handleErrorResponse(res);

			const data = await res.json();
			assets.value = data;
		} catch (e: any) {
			if (e.name === "AbortError") return;
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	// Search the template catalog without clobbering the cached `assets` list,
	// since callers (e.g. the asset-link modal) use this for transient lookups.
	async function searchAssetTemplates(
		search: string,
		signal?: AbortSignal,
	): Promise<Asset[]> {
		const url = new URL(`${BASE_URL}/assets`, window.location.origin);
		url.searchParams.append("search", search);

		const res = await apiFetch(url.toString(), {
			signal,
		});

		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function getAsset(id: number): Promise<Asset | null> {
		try {
			const res = await apiFetch(`${BASE_URL}/assets/${id}`);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function createAsset(asset: Partial<Asset>) {
		const res = await apiFetch(`${BASE_URL}/assets`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(asset),
		});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function updateAsset(id: number, asset: Partial<Asset>) {
		const res = await apiFetch(`${BASE_URL}/assets/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(asset),
		});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function deleteAsset(id: number) {
		const res = await apiFetch(`${BASE_URL}/assets/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
	}

	// Organization Asset Management
	async function fetchOrganizationAssets(
		organizationId: number,
	): Promise<EnrichedOrganizationAsset[]> {
		const res = await apiFetch(
			`${BASE_URL}/organizations/${organizationId}/assets`,
		);
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function fetchAllOrganizationAssets(params?: {
		search?: string;
		status?: string;
		before?: string;
	}): Promise<OrganizationAsset[]> {
		const url = new URL(
			`${BASE_URL}/organization-assets`,
			window.location.origin,
		);
		if (params?.search) url.searchParams.append("search", params.search);
		if (params?.status) url.searchParams.append("status", params.status);
		if (params?.before) url.searchParams.append("before", params.before);

		const res = await apiFetch(url.toString(), {});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function linkAsset(
		organizationId: number,
		data: Partial<OrganizationAsset>,
	) {
		const res = await apiFetch(
			`${BASE_URL}/organizations/${organizationId}/assets`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(data),
			},
		);
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function getOrganizationAsset(
		id: number,
	): Promise<EnrichedOrganizationAsset | null> {
		try {
			const res = await apiFetch(`${BASE_URL}/organization-assets/${id}`);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function updateOrganizationAsset(
		id: number,
		data: Partial<OrganizationAsset>,
	) {
		const res = await apiFetch(`${BASE_URL}/organization-assets/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(data),
		});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function unlinkAsset(id: number) {
		const res = await apiFetch(`${BASE_URL}/organization-assets/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
	}

	// Payment Tracking
	async function fetchAssetPayments(
		organizationAssetId: number,
	): Promise<AssetPayment[]> {
		const res = await apiFetch(
			`${BASE_URL}/organization-assets/${organizationAssetId}/payments`,
		);
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function recordPayment(
		organizationAssetId: number,
		data: {
			amount: number;
			payment_date?: string;
			info?: string;
			next_billing?: string;
		},
	) {
		const res = await apiFetch(
			`${BASE_URL}/organization-assets/${organizationAssetId}/payments`,
			{
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(data),
			},
		);
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function deletePayment(id: number) {
		const res = await apiFetch(`${BASE_URL}/asset-payments/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
	}

	return {
		assets,
		isLoading,
		error,
		fetchAssets,
		searchAssetTemplates,
		getAsset,
		createAsset,
		updateAsset,
		deleteAsset,
		fetchOrganizationAssets,
		fetchAllOrganizationAssets,
		linkAsset,
		getOrganizationAsset,
		updateOrganizationAsset,
		unlinkAsset,
		fetchAssetPayments,
		recordPayment,
		deletePayment,
	};
});
