import { ref } from "vue";
import { defineStore } from "pinia";
import type { Organization, Contact, ContactType, Site } from "../types";
import { useAuthStore } from "./auth";
import { useDataStore } from "./data";

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

export const useOrganizationStore = defineStore("organization", () => {
	const authStore = useAuthStore();
	const dataStore = useDataStore();

	const organizations = ref<Organization[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchOrganizations(search?: string, signal?: AbortSignal) {
		isLoading.value = true;
		error.value = null;
		try {
			const url = new URL(
				`${BASE_URL}/organizations`,
				window.location.origin,
			);
			if (search) {
				url.searchParams.append("search", search);
			}

			const res = await fetch(url.toString(), {
				headers: authStore.authHeader,
				signal,
			});

			if (!res.ok) throw new Error("Failed to fetch organizations");

			const data = await res.json();
			organizations.value = data;
		} catch (e: any) {
			if (e.name === "AbortError") return;
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	async function getOrganization(id: number): Promise<Organization | null> {
		try {
			const res = await fetch(`${BASE_URL}/organizations/${id}`, {
				headers: authStore.authHeader,
			});
			if (!res.ok) throw new Error("Failed to fetch organization");
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	async function createOrganization(organization: Partial<Organization>) {
		const res = await fetch(`${BASE_URL}/organizations`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(organization),
		});
		if (!res.ok) throw new Error("Failed to create organization");
		return await res.json();
	}

	async function updateOrganization(
		id: number,
		organization: Partial<Organization>,
	) {
		const res = await fetch(`${BASE_URL}/organizations/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(organization),
		});
		if (!res.ok) throw new Error("Failed to update organization");
		return await res.json();
	}

	async function deleteOrganization(id: number) {
		const res = await fetch(`${BASE_URL}/organizations/${id}`, {
			method: "DELETE",
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to delete organization");
	}

	async function fetchOrganizationContacts(
		organizationId: number,
	): Promise<Contact[]> {
		const res = await fetch(
			`${BASE_URL}/organizations/${organizationId}/contacts`,
			{
				headers: authStore.authHeader,
			},
		);
		if (!res.ok) throw new Error("Failed to fetch contacts");
		return await res.json();
	}

	async function fetchOrganizationSites(organizationId: number) {
		const res = await fetch(
			`${BASE_URL}/organizations/${organizationId}/sites`,
			{
				headers: authStore.authHeader,
			},
		);
		if (!res.ok) throw new Error("Failed to fetch organization sites");
		const sites = await res.json();

		if (Array.isArray(sites)) {
			sites.forEach((site: Site) => {
				dataStore.setSiteOrganizationLink(site.id, organizationId);
			});
		}

		return sites;
	}

	async function createContact(contact: {
		organization_id: number;
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

	async function getOrganizationForSite(
		siteId: number,
	): Promise<Organization | null> {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/organization`, {
			headers: authStore.authHeader,
		});
		if (res.status === 404) return null;
		if (!res.ok) throw new Error("Failed to fetch organization for site");
		return await res.json();
	}

	async function linkSiteToOrganization(
		siteId: number,
		organizationId: number,
	) {
		const res = await fetch(`${BASE_URL}/sites/${siteId}/link`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify({ organization_id: organizationId }),
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
		organizations,
		isLoading,
		error,
		fetchOrganizations,
		getOrganization,
		createOrganization,
		updateOrganization,
		deleteOrganization,
		fetchOrganizationContacts,
		fetchOrganizationSites,
		createContact,
		updateContact,
		deleteContact,
		getOrganizationForSite,
		linkSiteToOrganization,
		unlinkSite,
	};
});
