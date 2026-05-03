<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useOrganizationStore } from "../stores/organization";
import { useDataStore } from "../stores/data";
import type { Organization, Contact, ContactType, Site } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import EditableInfoCard from "../components/EditableInfoCard.vue";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const organizationStore = useOrganizationStore();
const dataStore = useDataStore();

const organizationId = parseInt(props.id, 10);
const organization = ref<Organization | null>(null);
const contacts = ref<Contact[]>([]);
const linkedSites = ref<Site[]>([]);
const isLoading = ref(true);
const error = ref<string | null>(null);

const loadData = async () => {
	isLoading.value = true;
	error.value = null;
	try {
		const [organizationData, contactsData, sitesData] = await Promise.all([
			organizationStore.getOrganization(organizationId),
			organizationStore.fetchOrganizationContacts(organizationId),
			organizationStore.fetchOrganizationSites(organizationId),
		]);

		if (organizationData) {
			organization.value = organizationData;
			contacts.value = contactsData;
			linkedSites.value = sitesData;
		} else {
			error.value = "Organization not found";
		}
	} catch (e: any) {
		error.value = e.message || "Failed to load organization details";
	} finally {
		isLoading.value = false;
	}
};

onMounted(() => {
	loadData();
});

const organizationInfoItems = computed(() => {
	if (!organization.value) return [];
	return [
		{
			label: "Organization Name",
			key: "name",
			value: organization.value.name,
			required: true,
		},
		{
			label: "VAT Number",
			key: "vat_number",
			value: organization.value.vat_number,
		},
		{
			label: "Information",
			key: "info",
			value: organization.value.info,
			type: "textarea" as const,
		},
	];
});

const handleSaveOrganization = async (values: Record<string, any>) => {
	try {
		const updated = await organizationStore.updateOrganization(
			organizationId,
			values,
		);
		organization.value = updated;
	} catch (e: any) {
		alert("Failed to update organization: " + e.message);
	}
};

// Contact Management
const showContactModal = ref(false);
const editingContact = ref<Contact | null>(null);
const contactForm = ref({
	name: "",
	email: "",
	phone: "",
	type: "Main" as ContactType,
});

const contactTypeOptions = [
	{ label: "Main", value: "Main" },
	{ label: "Technical", value: "Technical" },
	{ label: "Billing", value: "Billing" },
];

const openAddContact = () => {
	editingContact.value = null;
	contactForm.value = {
		name: "",
		email: "",
		phone: "",
		type: "Main",
	};
	showContactModal.value = true;
};

const openEditContact = (contact: Contact) => {
	editingContact.value = contact;
	contactForm.value = {
		name: contact.name,
		email: contact.email || "",
		phone: contact.phone || "",
		type: contact.type,
	};
	showContactModal.value = true;
};

const handleContactSubmit = async () => {
	try {
		if (editingContact.value) {
			await organizationStore.updateContact(
				editingContact.value.id,
				contactForm.value,
			);
		} else {
			await organizationStore.createContact({
				...contactForm.value,
				organization_id: organizationId,
			});
		}
		showContactModal.value = false;
		contacts.value =
			await organizationStore.fetchOrganizationContacts(organizationId);
	} catch (e: any) {
		alert("Failed to save contact: " + e.message);
	}
};

const handleDeleteContact = async (id: number) => {
	if (!confirm("Are you sure you want to delete this contact?")) return;
	try {
		await organizationStore.deleteContact(id);
		contacts.value =
			await organizationStore.fetchOrganizationContacts(organizationId);
	} catch (e: any) {
		alert("Failed to delete contact: " + e.message);
	}
};

const goBack = () => {
	router.push({ name: "organizations" });
};

const handleDeleteOrganization = async () => {
	if (
		!confirm(
			`Are you sure you want to delete ${organization.value?.name}? This will also delete all associated contacts.`,
		)
	)
		return;
	try {
		await organizationStore.deleteOrganization(organizationId);
		router.push({ name: "organizations" });
	} catch (e: any) {
		alert("Failed to delete organization: " + e.message);
	}
};

const showLinkSiteModal = ref(false);
const siteSearchQuery = ref("");

const availableSites = computed(() => {
	const query = siteSearchQuery.value.toLowerCase();
	return dataStore.enrichedSites.filter((site) => {
		const isNotLinked = !linkedSites.value.some((s) => s.id === site.id);
		const matchesQuery = site.domain.toLowerCase().includes(query);
		return isNotLinked && matchesQuery;
	});
});

const handleLinkSite = async (siteId: number) => {
	try {
		await organizationStore.linkSiteToOrganization(siteId, organizationId);
		dataStore.setSiteOrganizationLink(siteId, organizationId);
		await dataStore.refreshData();
		linkedSites.value =
			await organizationStore.fetchOrganizationSites(organizationId);
		showLinkSiteModal.value = false;
		siteSearchQuery.value = "";
	} catch (e: any) {
		alert("Failed to link site: " + e.message);
	}
};

const handleUnlinkSite = async (siteId: number) => {
	if (!confirm("Are you sure you want to unlink this site?")) return;
	try {
		await organizationStore.unlinkSite(siteId);
		dataStore.setSiteOrganizationLink(siteId, undefined);
		await dataStore.refreshData();
		linkedSites.value =
			await organizationStore.fetchOrganizationSites(organizationId);
	} catch (e: any) {
		alert("Failed to unlink site: " + e.message);
	}
};

const goToSite = (siteId: number) => {
	router.push({ name: "site-detail", params: { id: siteId.toString() } });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader
			:title="organization?.name || 'Organization Details'"
			:back-button="{ text: 'Back to Organizations', onClick: goBack }"
		>
			<template #actions>
				<button
					v-if="organization"
					class="btn-danger"
					@click="handleDeleteOrganization"
				>
					Delete Organization
				</button>
			</template>
		</ViewHeader>

		<div v-if="error" class="error-banner">
			<p><strong>Error:</strong> {{ error }}</p>
			<button class="text-btn" @click="loadData">Retry</button>
		</div>

		<main class="content" v-if="organization">
			<EditableInfoCard
				title="Organization Information"
				:items="organizationInfoItems"
				@save="handleSaveOrganization"
			/>

			<section class="card">
				<div class="card-header">
					<h2>Contacts ({{ contacts.length }})</h2>
					<button class="btn btn-primary btn-sm" @click="openAddContact">
						Add Contact
					</button>
				</div>

				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Name</th>
								<th>Type</th>
								<th>Email</th>
								<th>Phone</th>
								<th class="actions-cell">Actions</th>
							</tr>
						</thead>
						<tbody>
							<tr v-if="contacts.length === 0">
								<td colspan="5" class="empty-state">
									No contacts found for this organization.
								</td>
							</tr>
							<tr v-for="contact in contacts" :key="contact.id">
								<td>
									<strong>{{ contact.name }}</strong>
								</td>
								<td>
									<span :class="['status-badge', contact.type.toLowerCase()]">
										{{ contact.type }}
									</span>
								</td>
								<td>{{ contact.email || "—" }}</td>
								<td>{{ contact.phone || "—" }}</td>
								<td class="actions-cell">
									<div class="row-actions">
										<button
											class="icon-btn-sm"
											@click="openEditContact(contact)"
											title="Edit"
										>
											<svg
												xmlns="http://www.w3.org/2000/svg"
												width="16"
												height="16"
												viewBox="0 0 24 24"
												fill="none"
												stroke="currentColor"
												stroke-width="2"
												stroke-linecap="round"
												stroke-linejoin="round"
											>
												<path
													d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"
												></path>
												<path
													d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"
												></path>
											</svg>
										</button>
										<button
											class="icon-btn-sm delete"
											@click="handleDeleteContact(contact.id)"
											title="Delete"
										>
											<svg
												xmlns="http://www.w3.org/2000/svg"
												width="16"
												height="16"
												viewBox="0 0 24 24"
												fill="none"
												stroke="currentColor"
												stroke-width="2"
												stroke-linecap="round"
												stroke-linejoin="round"
											>
												<polyline points="3 6 5 6 21 6"></polyline>
												<path
													d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
												></path>
												<line x1="10" y1="11" x2="10" y2="17"></line>
												<line x1="14" y1="11" x2="14" y2="17"></line>
											</svg>
										</button>
									</div>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</section>

			<section class="card">
				<div class="card-header">
					<h2>Linked Sites ({{ linkedSites.length }})</h2>
					<button
						class="btn btn-primary btn-sm"
						@click="showLinkSiteModal = true"
					>
						Link Site
					</button>
				</div>

				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Domain</th>
								<th>PHP</th>
								<th class="actions-cell">Actions</th>
							</tr>
						</thead>
						<tbody>
							<tr v-if="linkedSites.length === 0">
								<td colspan="3" class="empty-state">
									No sites linked to this organization.
								</td>
							</tr>
							<tr v-for="site in linkedSites" :key="site.id">
								<td>
									<a
										href="#"
										@click.prevent="goToSite(site.id)"
										class="site-link"
									>
										<strong>{{ site.domain }}</strong>
									</a>
								</td>
								<td>{{ site.php_version }}</td>
								<td class="actions-cell">
									<div class="row-actions">
										<button
											class="icon-btn-sm delete"
											@click="handleUnlinkSite(site.id)"
											title="Unlink Site"
										>
											<svg
												xmlns="http://www.w3.org/2000/svg"
												width="16"
												height="16"
												viewBox="0 0 24 24"
												fill="none"
												stroke="currentColor"
												stroke-width="2"
												stroke-linecap="round"
												stroke-linejoin="round"
											>
												<line x1="18" y1="6" x2="6" y2="18"></line>
												<line x1="6" y1="6" x2="18" y2="18"></line>
											</svg>
										</button>
									</div>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</section>
		</main>

		<main class="content" v-else-if="isLoading">
			<div class="card">
				<LoadingSpinner message="Loading organization details..." />
			</div>
		</main>

		<!-- Contact Modal -->
		<div
			v-if="showContactModal"
			class="modal-overlay"
			@click.self="showContactModal = false"
		>
			<div class="modal-content card">
				<h2>{{ editingContact ? "Edit Contact" : "Add New Contact" }}</h2>
				<form @submit.prevent="handleContactSubmit" class="form-layout">
					<div class="form-group">
						<label for="c-name">Full Name*</label>
						<input
							id="c-name"
							v-model="contactForm.name"
							type="text"
							required
							placeholder="Contact person name"
						/>
					</div>
					<div class="form-group">
						<label for="c-type">Type</label>
						<select id="c-type" v-model="contactForm.type">
							<option
								v-for="opt in contactTypeOptions"
								:key="opt.value"
								:value="opt.value"
							>
								{{ opt.label }}
							</option>
						</select>
					</div>
					<div class="form-group">
						<label for="c-email">Email Address</label>
						<input
							id="c-email"
							v-model="contactForm.email"
							type="email"
							placeholder="email@example.com"
						/>
					</div>
					<div class="form-group">
						<label for="c-phone">Phone Number</label>
						<input
							id="c-phone"
							v-model="contactForm.phone"
							type="tel"
							placeholder="+49 ..."
						/>
					</div>
					<div class="form-actions">
						<button
							type="button"
							class="back-btn"
							@click="showContactModal = false"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="!contactForm.name"
						>
							{{ editingContact ? "Update Contact" : "Add Contact" }}
						</button>
					</div>
				</form>
			</div>
		</div>

		<!-- Link Site Modal -->
		<div
			v-if="showLinkSiteModal"
			class="modal-overlay"
			@click.self="showLinkSiteModal = false"
		>
			<div class="modal-content card">
				<h2>Link Site to Organization</h2>
				<div class="form-layout">
					<div class="form-group">
						<label for="s-search">Search Site Domain</label>
						<input
							id="s-search"
							v-model="siteSearchQuery"
							type="text"
							placeholder="e.g. example.com"
						/>
					</div>

					<div class="search-results-list" v-if="availableSites.length > 0">
						<div
							v-for="site in availableSites"
							:key="site.id"
							class="search-result-item"
							@click="handleLinkSite(site.id)"
						>
							<div class="res-name">{{ site.domain }}</div>
						</div>
					</div>
					<div v-else-if="siteSearchQuery.length > 0" class="empty-state">
						No available sites found.
					</div>

					<div class="form-actions">
						<button class="back-btn" @click="showLinkSiteModal = false">
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>
</template>

<style scoped>
.site-link {
	color: var(--primary);
	text-decoration: none;
}

.site-link:hover {
	text-decoration: underline;
}

.search-results-list {
	margin-top: 12px;
	border: 1px solid var(--border-color);
	border-radius: 6px;
	max-height: 250px;
	overflow-y: auto;
}

.search-result-item {
	padding: 10px 16px;
	cursor: pointer;
	transition: background-color 0.2s;
	border-bottom: 1px solid var(--border-color);
}

.search-result-item:last-child {
	border-bottom: none;
}

.search-result-item:hover {
	background-color: var(--bg-hover);
}

.res-name {
	font-weight: 600;
	color: var(--text-main);
}

.card-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 20px;
}

.card-header h2 {
	margin: 0;
	font-size: 1.1rem;
}

.actions-cell {
	width: 100px;
	text-align: right;
}

.row-actions {
	display: flex;
	justify-content: flex-end;
	gap: 8px;
}

.icon-btn-sm {
	background: none;
	border: none;
	color: var(--text-muted);
	padding: 4px;
	border-radius: 4px;
	cursor: pointer;
	display: flex;
	align-items: center;
	transition: all 0.2s;
}

.icon-btn-sm:hover {
	background-color: var(--bg-hover);
	color: var(--primary);
}

.icon-btn-sm.delete:hover {
	color: #ef4444;
}

.btn-sm {
	padding: 6px 12px;
	font-size: 13px;
}

/* Status badges for contact types */
.status-badge.main {
	background-color: rgba(59, 130, 246, 0.1);
	color: #3b82f6;
}

.status-badge.technical {
	background-color: rgba(16, 185, 129, 0.1);
	color: #10b981;
}

.status-badge.billing {
	background-color: rgba(245, 158, 11, 0.1);
	color: #f59e0b;
}

/* Modal styles */
.modal-overlay {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background: rgba(0, 0, 0, 0.6);
	backdrop-filter: blur(2px);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 1000;
}

.modal-content {
	width: 100%;
	max-width: 500px;
	padding: 24px;
	box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
}

.modal-content h2 {
	margin-top: 0;
	margin-bottom: 20px;
	font-size: 1.25rem;
	border-bottom: none;
	padding-bottom: 0;
}

.form-layout {
	display: flex;
	flex-direction: column;
	gap: 16px;
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-group label {
	font-size: 0.85rem;
	font-weight: 600;
	color: var(--text-muted);
}

.form-group input,
.form-group select {
	padding: 10px 12px;
	border: 1px solid var(--border-input);
	border-radius: 6px;
	background: var(--bg-body);
	color: var(--text-main);
	font-size: 0.95rem;
}

.form-actions {
	display: flex;
	justify-content: flex-end;
	gap: 12px;
	margin-top: 8px;
}

.btn-danger {
	padding: 8px 16px;
	background-color: transparent;
	border: 1px solid #ef4444;
	color: #ef4444;
	border-radius: 4px;
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	transition: all 0.2s;
}

.btn-danger:hover {
	background-color: #ef4444;
	color: white;
}

.error-banner {
	display: flex;
	justify-content: space-between;
	align-items: center;
}

.text-btn {
	background: none;
	border: none;
	color: var(--primary);
	font-weight: 600;
	cursor: pointer;
	text-decoration: underline;
}
</style>
