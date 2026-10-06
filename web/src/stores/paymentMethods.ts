import { ref } from "vue";
import { defineStore } from "pinia";
import type { PaymentMethod } from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const usePaymentMethodsStore = defineStore("paymentMethods", () => {
	const paymentMethods = ref<PaymentMethod[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchPaymentMethods(params?: {
		search?: string;
		type?: string;
	}) {
		isLoading.value = true;
		error.value = null;
		try {
			const url = new URL(
				`${BASE_URL}/payment-methods`,
				window.location.origin,
			);
			if (params?.search)
				url.searchParams.append("search", params.search);
			if (params?.type) url.searchParams.append("type", params.type);

			const res = await apiFetch(url.toString(), {});
			if (!res.ok) await handleErrorResponse(res);
			paymentMethods.value = await res.json();
		} catch (e: any) {
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	async function getPaymentMethod(id: number): Promise<PaymentMethod | null> {
		try {
			const res = await apiFetch(`${BASE_URL}/payment-methods/${id}`);
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function createPaymentMethod(payload: Partial<PaymentMethod>) {
		const res = await apiFetch(`${BASE_URL}/payment-methods`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(payload),
		});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function updatePaymentMethod(
		id: number,
		payload: Partial<PaymentMethod>,
	) {
		const res = await apiFetch(`${BASE_URL}/payment-methods/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(payload),
		});
		if (!res.ok) await handleErrorResponse(res);
		return await res.json();
	}

	async function deletePaymentMethod(id: number) {
		const res = await apiFetch(`${BASE_URL}/payment-methods/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
	}

	return {
		paymentMethods,
		isLoading,
		error,
		fetchPaymentMethods,
		getPaymentMethod,
		createPaymentMethod,
		updatePaymentMethod,
		deletePaymentMethod,
	};
});
