import { ref } from "vue";
import { defineStore } from "pinia";
import type { Company, Contact, ContactType } from "../types";
import { useAuthStore } from "./auth";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useCompanyStore = defineStore("company", () => {
	const authStore = useAuthStore();

	const companies = ref<Company[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchCompanies(search?: string) {
		isLoading.value = true;
		error.value = null;
		try {
			const url = new URL(`${BASE_URL}/companies`, window.location.origin);
			if (search) {
				url.searchParams.append("search", search);
			}

			const res = await fetch(url.toString(), {
				headers: authStore.authHeader,
			});

			if (!res.ok) throw new Error("Failed to fetch companies");

			const data = await res.json();
			companies.value = data;
		} catch (e: any) {
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	async function getCompany(id: number): Promise<Company | null> {
		try {
			const res = await fetch(`${BASE_URL}/companies/${id}`, {
				headers: authStore.authHeader,
			});
			if (!res.ok) throw new Error("Failed to fetch company");
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function createCompany(company: Partial<Company>) {
		const res = await fetch(`${BASE_URL}/companies`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(company),
		});
		if (!res.ok) throw new Error("Failed to create company");
		return await res.json();
	}

	async function updateCompany(id: number, company: Partial<Company>) {
		const res = await fetch(`${BASE_URL}/companies/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(company),
		});
		if (!res.ok) throw new Error("Failed to update company");
		return await res.json();
	}

	async function deleteCompany(id: number) {
		const res = await fetch(`${BASE_URL}/companies/${id}`, {
			method: "DELETE",
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to delete company");
	}

	async function fetchCompanyContacts(companyId: number): Promise<Contact[]> {
		const res = await fetch(`${BASE_URL}/companies/${companyId}/contacts`, {
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to fetch contacts");
		return await res.json();
	}

	async function createContact(contact: {
		company_id: number;
		name: string;
		email?: string;
		phone?: string;
		type: ContactType;
	}) {
		const res = await fetch(`${BASE_URL}/contacts`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(contact),
		});
		if (!res.ok) throw new Error("Failed to create contact");
		return await res.json();
	}

	async function updateContact(id: number, contact: Partial<Contact>) {
		const res = await fetch(`${BASE_URL}/contacts/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(contact),
		});
		if (!res.ok) throw new Error("Failed to update contact");
		return await res.json();
	}

	async function deleteContact(id: number) {
		const res = await fetch(`${BASE_URL}/contacts/${id}`, {
			method: "DELETE",
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to delete contact");
	}

	async function getCompanyForSite(siteId: number): Promise<Company | null> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/company`, {
			headers: authStore.authHeader,
		});
		if (res.status === 404) return null;
		if (!res.ok) throw new Error("Failed to fetch company for site");
		return await res.json();
	}

	async function linkSiteToCompany(siteId: number, companyId: number) {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/link`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ company_id: companyId }),
		});
		if (!res.ok) throw new Error("Failed to link site");
	}

	async function unlinkSite(siteId: number) {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/link`, {
			method: "DELETE",
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to unlink site");
	}

	return {
		companies,
		isLoading,
		error,
		fetchCompanies,
		getCompany,
		createCompany,
		updateCompany,
		deleteCompany,
		fetchCompanyContacts,
		createContact,
		updateContact,
		deleteContact,
		getCompanyForSite,
		linkSiteToCompany,
		unlinkSite,
	};
});
