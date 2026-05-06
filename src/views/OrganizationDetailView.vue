<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useOrganizationStore } from "../stores/organization";
import { useDataStore } from "../stores/data";
import { useUserStore } from "../stores/user";
import { useAssetStore } from "../stores/assetStore";
import { useAuthStore } from "../stores/auth";
import { useToastStore } from "../stores/toast";
import type {
	Organization,
	Contact,
	ContactType,
	Site,
	EnrichedOrganizationAsset,
	Asset,
	BillingFrequency,
	OrganizationAssetStatus,
} from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import EditableInfoCard from "../components/EditableInfoCard.vue";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const organizationStore = useOrganizationStore();
const dataStore = useDataStore();
const userStore = useUserStore();
const assetStore = useAssetStore();
const authStore = useAuthStore();
const toast = useToastStore();

const organizationId = parseInt(props.id, 10);
const organization = ref<Organization | null>(null);
const contacts = ref<Contact[]>([]);
const linkedSites = ref<Site[]>([]);
const orgAssets = ref<EnrichedOrganizationAsset[]>([]);
const isLoading = ref(true);
const error = ref<string | null>(null);

const loadData = async () => {
	isLoading.value = true;
	error.value = null;
	try {
		const [organizationData, contactsData, sitesData, assetsData] =
			await Promise.all([
				organizationStore.getOrganization(organizationId),
				organizationStore.fetchOrganizationContacts(organizationId),
				organizationStore.fetchOrganizationSites(organizationId),
				assetStore.fetchOrganizationAssets(organizationId),
			]);

		if (organizationData) {
			organization.value = organizationData;
			contacts.value = contactsData;
			linkedSites.value = sitesData;
			orgAssets.value = assetsData;
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
	userStore.fetchUsers();
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
		toast.addToast("Failed to update organization: " + e.message, "error");
		throw e;
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
		toast.addToast("Failed to save contact: " + e.message, "error");
	}
};

const handleDeleteContact = async (id: number) => {
	if (!confirm("Are you sure you want to delete this contact?")) return;
	try {
		await organizationStore.deleteContact(id);
		contacts.value =
			await organizationStore.fetchOrganizationContacts(organizationId);
	} catch (e: any) {
		toast.addToast("Failed to delete contact: " + e.message, "error");
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
		toast.addToast("Failed to delete organization: " + e.message, "error");
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
		toast.addToast("Failed to link site: " + e.message, "error");
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
		toast.addToast("Failed to unlink site: " + e.message, "error");
	}
};

const goToSite = (id: number) => {
	router.push({ name: "site-detail", params: { id: id.toString() } });
};

// Asset Management
const showLinkAssetModal = ref(false);
const showPaymentModal = ref(false);
const editingOrgAsset = ref<EnrichedOrganizationAsset | null>(null);
const selectedAssetForPayment = ref<EnrichedOrganizationAsset | null>(null);
const assetSearchQuery = ref("");
const availableAssetTemplates = ref<Asset[]>([]);

const assetForm = ref({
	asset_id: null as number | null,
	site_id: null as number | null,
	identifier: "",
	price: 0,
	billing_freq: "Yearly" as BillingFrequency,
	next_billing: "",
	status: "active" as OrganizationAssetStatus,
	description: "",
});

const paymentForm = ref({
	amount: 0,
	info: "",
	next_billing: "",
});

const searchAssets = async () => {
	if (assetSearchQuery.value.length < 2) {
		availableAssetTemplates.value = [];
		return;
	}
	await assetStore.fetchAssets(assetSearchQuery.value);
	availableAssetTemplates.value = assetStore.assets;
};

const selectAssetTemplate = (template: Asset) => {
	assetForm.value.asset_id = template.id;
	assetForm.value.identifier = template.identifier || "";
	assetForm.value.price = template.default_price || 0;
	assetForm.value.billing_freq = template.default_freq || "Yearly";
	assetForm.value.next_billing = new Date().toISOString().split("T")[0] || "";
	assetSearchQuery.value = template.name;
	availableAssetTemplates.value = [];
};

const openAddAsset = () => {
	editingOrgAsset.value = null;
	assetForm.value = {
		asset_id: null,
		site_id: null,
		identifier: "",
		price: 0,
		billing_freq: "Yearly",
		next_billing: "",
		status: "active",
		description: "",
	};
	assetSearchQuery.value = "";
	showLinkAssetModal.value = true;
};

const openEditAsset = (oa: EnrichedOrganizationAsset) => {
	editingOrgAsset.value = oa;
	assetForm.value = {
		asset_id: oa.asset_id,
		site_id: oa.site_id,
		identifier: oa.identifier || "",
		price: oa.price,
		billing_freq: oa.billing_freq,
		next_billing: oa.next_billing
			? oa.next_billing.split("T")[0] || ""
			: "",
		status: oa.status,
		description: oa.description || "",
	};
	assetSearchQuery.value = oa.asset?.name || oa.asset_name || "";
	showLinkAssetModal.value = true;
};

const handleLinkAsset = async () => {
	try {
		const payload = { ...assetForm.value };
		if (payload.next_billing) {
			payload.next_billing = new Date(payload.next_billing).toISOString();
		}

		if (editingOrgAsset.value) {
			await assetStore.updateOrganizationAsset(
				editingOrgAsset.value.id,
				payload,
			);
		} else {
			await assetStore.linkAsset(organizationId, payload);
		}

		showLinkAssetModal.value = false;
		orgAssets.value =
			await assetStore.fetchOrganizationAssets(organizationId);
	} catch (e: any) {
		toast.addToast("Failed to save asset: " + e.message, "error");
	}
};

const unlinkedPlugins = computed(() => {
	const siteIds = linkedSites.value.map((s) => s.id);
	const orgSites = dataStore.enrichedSites.filter((s) =>
		siteIds.includes(s.id),
	);

	const unlinked: Array<{
		site: Site;
		pluginName: string;
		slug: string;
	}> = [];

	// Get all asset templates that are of type 'Plugin'
	// Get all asset templates that are of type 'Plugin'
	const pluginTemplates = assetStore.assets.filter(
		(a) => a.type === "Plugin" && a.identifier,
	);

	orgSites.forEach((site) => {
		site.plugins.forEach((plugin) => {
			// Only suggest plugins that match an existing asset template identifier (slug)
			const matchingTemplate = pluginTemplates.find(
				(a) => a.identifier === plugin.name,
			);

			if (!matchingTemplate) {
				return;
			}

			// Check if already linked specifically to this site OR globally to the organization
			const isLinked = orgAssets.value.some(
				(oa) =>
					(oa.site_id === site.id || oa.site_id === null) &&
					(oa.asset_id === matchingTemplate.id ||
						oa.identifier === plugin.name),
			);

			if (!isLinked) {
				const enriched = dataStore.enrichedPlugins.find(
					(p) => p.slug === plugin.name,
				);
				unlinked.push({
					site,
					pluginName: enriched ? enriched.shortName : plugin.name,
					slug: plugin.name,
				});
			}
		});
	});

	return unlinked;
});

const convertToAsset = async (plugin: {
	site: Site;
	pluginName: string;
	slug: string;
}) => {
	editingOrgAsset.value = null;
	assetForm.value.site_id = plugin.site.id;
	assetForm.value.identifier = plugin.slug;
	assetForm.value.next_billing = new Date().toISOString().split("T")[0] || "";

	await assetStore.fetchAssets(plugin.slug);
	const template = assetStore.assets.find(
		(a) => a.type === "Plugin" && a.identifier === plugin.slug,
	);

	if (template) {
		selectAssetTemplate(template);
	} else {
		assetForm.value.asset_id = null;
		assetSearchQuery.value = plugin.pluginName;
	}

	showLinkAssetModal.value = true;
};

const calculateNextBillingDate = (
	dateStr: string | null,
	freq: BillingFrequency,
): string => {
	const date = dateStr ? new Date(dateStr) : new Date();
	if (isNaN(date.getTime()))
		return new Date().toISOString().split("T")[0] || "";

	if (freq === "Monthly") date.setMonth(date.getMonth() + 1);
	else if (freq === "Quarterly") date.setMonth(date.getMonth() + 3);
	else if (freq === "Yearly") date.setFullYear(date.getFullYear() + 1);
	else return "";

	return date.toISOString().split("T")[0] || "";
};

const openPaymentModal = (asset: EnrichedOrganizationAsset) => {
	selectedAssetForPayment.value = asset;
	const suggestedNextDate = calculateNextBillingDate(
		asset.next_billing,
		asset.billing_freq,
	);
	paymentForm.value = {
		amount: asset.price,
		info: `Renewal ${new Date().toLocaleDateString()}`,
		next_billing: suggestedNextDate,
	};
	showPaymentModal.value = true;
};

const handleRecordPayment = async () => {
	if (!selectedAssetForPayment.value) return;
	try {
		const payload = { ...paymentForm.value };
		if (payload.next_billing) {
			payload.next_billing = new Date(payload.next_billing).toISOString();
		}

		await assetStore.recordPayment(
			selectedAssetForPayment.value.id,
			payload,
		);
		showPaymentModal.value = false;
		orgAssets.value =
			await assetStore.fetchOrganizationAssets(organizationId);
	} catch (e: any) {
		toast.addToast("Failed to record payment: " + e.message, "error");
	}
};

const handleUnlinkAsset = async (id: number) => {
	if (!confirm("Are you sure you want to unlink this asset?")) return;
	try {
		await assetStore.unlinkAsset(id);
		orgAssets.value =
			await assetStore.fetchOrganizationAssets(organizationId);
	} catch (e: any) {
		toast.addToast("Failed to unlink asset: " + e.message, "error");
	}
};

const formatCurrency = (cents: number) => {
	return new Intl.NumberFormat("de-DE", {
		style: "currency",
		currency: "EUR",
	}).format(cents / 100);
};

const formatDate = (dateString: string | null) => {
	if (!dateString) return "-";
	return new Date(dateString).toLocaleDateString("de-DE");
};

const formatAuditDate = (dateStr: string) => {
	if (!dateStr) return "";
	return new Date(dateStr).toLocaleString(undefined, {
		year: "numeric",
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
};

const isModified = (item: any) => {
	if (!item || !item.updated_at || !item.created_at) return false;
	return (
		item.updated_at !== item.created_at ||
		item.updated_by !== item.created_by
	);
};

const contactsAudit = computed(() => {
	if (!contacts.value?.length) return null;
	return contacts.value.reduce((latest, current) => {
		const latestTime = new Date(latest.updated_at || 0).getTime();
		const currentTime = new Date(current.updated_at || 0).getTime();
		return currentTime > latestTime ? current : latest;
	});
});

const sitesAudit = computed(() => {
	if (!linkedSites.value?.length) return null;
	return linkedSites.value.reduce((latest, current) => {
		const latestTime = new Date(latest.updated_at || 0).getTime();
		const currentTime = new Date(current.updated_at || 0).getTime();
		return currentTime > latestTime ? current : latest;
	});
});
</script>

<template>
	<div class="view-container">
		<ViewHeader
			:title="organization?.name || 'Organization Details'"
			:back-button="{ text: 'Back to Organizations', onClick: goBack }"
		>
			<template #actions>
				<button
					v-if="organization && authStore.canEdit"
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

		<main v-if="organization" class="content">
			<div class="card-group">
				<EditableInfoCard
					title="Organization Information"
					:items="organizationInfoItems"
					:editable="authStore.canEdit"
					:on-save="handleSaveOrganization"
				/>
				<div v-if="organization?.created_by" class="card-footer-audit">
					Created by
					{{ userStore.resolveDisplayName(organization.created_by) }}
					on {{ formatAuditDate(organization.created_at) }}.<template
						v-if="isModified(organization)"
					>
						Last edited by
						{{
							userStore.resolveDisplayName(
								organization.updated_by,
							)
						}}
						on {{ formatAuditDate(organization.updated_at) }}.
					</template>
				</div>
			</div>

			<div class="card-group">
				<section class="card">
					<div class="card-header">
						<h2>Contacts ({{ contacts.length }})</h2>
						<button
							v-if="authStore.canEdit"
							class="btn btn-primary btn-sm"
							@click="openAddContact"
						>
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
									<th
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										Actions
									</th>
								</tr>
							</thead>
							<tbody>
								<tr v-if="contacts.length === 0">
									<td colspan="5" class="empty-state">
										No contacts found for this organization.
									</td>
								</tr>
								<tr
									v-for="contact in contacts"
									:key="contact.id"
								>
									<td>
										<strong>{{ contact.name }}</strong>
									</td>
									<td>
										<span
											:class="[
												'status-badge',
												contact.type.toLowerCase(),
											]"
										>
											{{ contact.type }}
										</span>
									</td>
									<td>{{ contact.email || "—" }}</td>
									<td>{{ contact.phone || "—" }}</td>
									<td
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										<div class="row-actions">
											<button
												class="icon-btn-sm"
												title="Edit"
												@click="
													openEditContact(contact)
												"
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
												title="Delete"
												@click="
													handleDeleteContact(
														contact.id,
													)
												"
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
													<polyline
														points="3 6 5 6 21 6"
													></polyline>
													<path
														d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
													></path>
													<line
														x1="10"
														y1="11"
														x2="10"
														y2="17"
													></line>
													<line
														x1="14"
														y1="11"
														x2="14"
														y2="17"
													></line>
												</svg>
											</button>
										</div>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</section>
				<div v-if="contactsAudit?.updated_by" class="card-footer-audit">
					<template v-if="isModified(contactsAudit)">
						Last edited by
						{{
							userStore.resolveDisplayName(
								contactsAudit.updated_by,
							)
						}}
						on {{ formatAuditDate(contactsAudit.updated_at) }}.
					</template>
					<template v-else>
						Created by
						{{
							userStore.resolveDisplayName(
								contactsAudit.created_by,
							)
						}}
						on {{ formatAuditDate(contactsAudit.created_at) }}.
					</template>
				</div>
			</div>

			<div class="card-group">
				<section class="card">
					<div class="card-header">
						<h2>Linked Sites ({{ linkedSites.length }})</h2>
						<button
							v-if="authStore.canEdit"
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
									<th
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										Actions
									</th>
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
											class="site-link"
											@click.prevent="goToSite(site.id)"
										>
											<strong>{{ site.domain }}</strong>
										</a>
									</td>
									<td>{{ site.php_version }}</td>
									<td
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										<div class="row-actions">
											<button
												class="icon-btn-sm delete"
												title="Unlink Site"
												@click="
													handleUnlinkSite(site.id)
												"
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
													<line
														x1="18"
														y1="6"
														x2="6"
														y2="18"
													></line>
													<line
														x1="6"
														y1="6"
														x2="18"
														y2="18"
													></line>
												</svg>
											</button>
										</div>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</section>
				<div v-if="sitesAudit?.updated_by" class="card-footer-audit">
					<template v-if="isModified(sitesAudit)">
						Last edited by
						{{
							userStore.resolveDisplayName(sitesAudit.updated_by)
						}}
						on {{ formatAuditDate(sitesAudit.updated_at) }}.
					</template>
					<template v-else>
						Created by
						{{
							userStore.resolveDisplayName(sitesAudit.created_by)
						}}
						on {{ formatAuditDate(sitesAudit.created_at) }}.
					</template>
				</div>
			</div>

			<div class="card-group">
				<section class="card">
					<div class="card-header">
						<h2>Assets & Services ({{ orgAssets.length }})</h2>
						<button
							v-if="authStore.canEdit"
							class="btn btn-primary btn-sm"
							@click="openAddAsset"
						>
							Link Asset
						</button>
					</div>

					<div class="table-container">
						<table class="data-table">
							<thead>
								<tr>
									<th>Asset</th>
									<th>Identifier</th>
									<th>Price</th>
									<th>Frequency</th>
									<th>Next Billing</th>
									<th>Status</th>
									<th
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										Actions
									</th>
								</tr>
							</thead>
							<tbody>
								<tr v-if="orgAssets.length === 0">
									<td colspan="6" class="empty-state">
										No assets linked to this organization.
									</td>
								</tr>
								<tr v-for="oa in orgAssets" :key="oa.id">
									<td>
										<strong>{{
											oa.asset?.name ||
											oa.asset_name ||
											oa.identifier ||
											"Custom Asset"
										}}</strong>
										<div v-if="oa.site_id" class="sub-text">
											Linked to:
											{{
												linkedSites.find(
													(s) => s.id === oa.site_id,
												)?.domain || "Unknown Site"
											}}
										</div>
									</td>
									<td>{{ oa.identifier }}</td>
									<td>{{ formatCurrency(oa.price) }}</td>
									<td>{{ oa.billing_freq }}</td>
									<td>{{ formatDate(oa.next_billing) }}</td>
									<td>
										<span
											:class="['status-badge', oa.status]"
										>
											{{ oa.status }}
										</span>
									</td>
									<td
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										<div class="row-actions">
											<button
												class="icon-btn-sm"
												title="Edit Asset"
												@click="openEditAsset(oa)"
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
												class="icon-btn-sm"
												title="Record Payment"
												@click="openPaymentModal(oa)"
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
													<rect
														x="2"
														y="5"
														width="20"
														height="14"
														rx="2"
													></rect>
													<line
														x1="2"
														y1="10"
														x2="22"
														y2="10"
													></line>
												</svg>
											</button>
											<button
												class="icon-btn-sm delete"
												title="Unlink Asset"
												@click="
													handleUnlinkAsset(oa.id)
												"
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
													<line
														x1="18"
														y1="6"
														x2="6"
														y2="18"
													></line>
													<line
														x1="6"
														y1="6"
														x2="18"
														y2="18"
													></line>
												</svg>
											</button>
										</div>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</section>
			</div>

			<div v-if="unlinkedPlugins.length > 0" class="card-group">
				<section class="card">
					<div class="card-header">
						<h2>Plugin Audit</h2>
						<span class="unlinked-count"
							>{{ unlinkedPlugins.length }} Potential Assets</span
						>
					</div>
					<div class="table-container">
						<table class="data-table">
							<thead>
								<tr>
									<th>Site</th>
									<th>Plugin</th>
									<th
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										Actions
									</th>
								</tr>
							</thead>
							<tbody>
								<tr
									v-for="(p, idx) in unlinkedPlugins"
									:key="idx"
								>
									<td>{{ p.site.domain }}</td>
									<td>
										<strong>{{ p.pluginName }}</strong>
									</td>
									<td
										v-if="authStore.canEdit"
										class="actions-cell"
									>
										<button
											class="btn btn-primary btn-sm"
											@click="convertToAsset(p)"
										>
											Convert to Asset
										</button>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</section>
			</div>
		</main>

		<main v-else-if="isLoading" class="content">
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
				<h2>
					{{ editingContact ? "Edit Contact" : "Add New Contact" }}
				</h2>
				<form class="form-layout" @submit.prevent="handleContactSubmit">
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
							placeholder="+358 ..."
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
							{{
								editingContact
									? "Update Contact"
									: "Add Contact"
							}}
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

					<div
						v-if="availableSites.length > 0"
						class="search-results-list"
					>
						<div
							v-for="site in availableSites"
							:key="site.id"
							class="search-result-item"
							@click="handleLinkSite(site.id)"
						>
							<div class="res-name">{{ site.domain }}</div>
						</div>
					</div>
					<div
						v-else-if="siteSearchQuery.length > 0"
						class="empty-state"
					>
						No available sites found.
					</div>

					<div class="form-actions">
						<button
							class="back-btn"
							@click="showLinkSiteModal = false"
						>
							Cancel
						</button>
					</div>
				</div>
			</div>
		</div>
	</div>

	<!-- Link Asset Modal -->
	<div
		v-if="showLinkAssetModal"
		class="modal-overlay"
		@click.self="showLinkAssetModal = false"
	>
		<div class="modal-content card">
			<h2>{{ editingOrgAsset ? "Edit Asset" : "Link New Asset" }}</h2>
			<div class="form-layout">
				<div class="form-group">
					<label for="a-search">Search Template</label>
					<input
						id="a-search"
						v-model="assetSearchQuery"
						type="text"
						placeholder="Start typing asset name..."
						autocomplete="off"
						@input="searchAssets"
					/>
					<div
						v-if="availableAssetTemplates.length > 0"
						class="search-results-list"
					>
						<div
							v-for="template in availableAssetTemplates"
							:key="template.id"
							class="search-result-item"
							@click="selectAssetTemplate(template)"
						>
							<div class="res-name">{{ template.name }}</div>
							<div class="sub-text">
								{{ template.type }} -
								{{
									formatCurrency(template.default_price || 0)
								}}
							</div>
						</div>
					</div>
				</div>

				<div v-if="assetForm.asset_id" class="form-row">
					<div class="form-group">
						<label for="a-price">Price (€)</label>
						<input
							id="a-price"
							type="number"
							step="0.01"
							:value="(assetForm.price / 100).toFixed(2)"
							@input="
								(e) =>
									(assetForm.price = Math.round(
										parseFloat(
											(e.target as HTMLInputElement)
												.value || '0',
										) * 100,
									))
							"
						/>
					</div>
					<div class="form-group">
						<label for="a-freq">Frequency</label>
						<select id="a-freq" v-model="assetForm.billing_freq">
							<option value="Monthly">Monthly</option>
							<option value="Quarterly">Quarterly</option>
							<option value="Yearly">Yearly</option>
							<option value="One-time">One-time</option>
						</select>
					</div>
				</div>

				<div class="form-group">
					<label for="a-site">Link to Site (Optional)</label>
					<select id="a-site" v-model="assetForm.site_id">
						<option :value="null">None</option>
						<option
							v-for="site in linkedSites"
							:key="site.id"
							:value="site.id"
						>
							{{ site.domain }}
						</option>
					</select>
				</div>

				<div class="form-group">
					<label for="a-identifier"
						>Identifier / License / Domain</label
					>
					<input
						id="a-identifier"
						v-model="assetForm.identifier"
						type="text"
					/>
				</div>

				<div class="form-row">
					<div class="form-group">
						<label for="a-next-billing">Next Billing Date</label>
						<input
							id="a-next-billing"
							v-model="assetForm.next_billing"
							type="date"
						/>
					</div>
					<div class="form-group">
						<label for="a-status">Status</label>
						<select id="a-status" v-model="assetForm.status">
							<option value="active">Active</option>
							<option value="paused">Paused</option>
							<option value="cancelled">Cancelled</option>
						</select>
					</div>
				</div>

				<div class="form-group">
					<label for="a-description">Description / Notes</label>
					<textarea
						id="a-description"
						v-model="assetForm.description"
						rows="2"
					></textarea>
				</div>

				<div class="form-actions">
					<button
						class="back-btn"
						@click="showLinkAssetModal = false"
					>
						Cancel
					</button>
					<button
						class="btn btn-primary"
						:disabled="!assetForm.asset_id"
						@click="handleLinkAsset"
					>
						{{ editingOrgAsset ? "Update Asset" : "Link Asset" }}
					</button>
				</div>
			</div>
		</div>
	</div>

	<!-- Record Payment Modal -->
	<div
		v-if="showPaymentModal"
		class="modal-overlay"
		@click.self="showPaymentModal = false"
	>
		<div class="modal-content card">
			<h2>Record Payment</h2>
			<p v-if="selectedAssetForPayment">
				Recording payment for:
				<strong>{{
					selectedAssetForPayment.asset?.name ||
					selectedAssetForPayment.identifier
				}}</strong>
			</p>
			<div class="form-layout">
				<div class="form-group">
					<label for="p-amount">Amount (€)</label>
					<input
						id="p-amount"
						type="number"
						step="0.01"
						:value="(paymentForm.amount / 100).toFixed(2)"
						@input="
							(e) =>
								(paymentForm.amount = Math.round(
									parseFloat(
										(e.target as HTMLInputElement).value ||
											'0',
									) * 100,
								))
						"
					/>
				</div>
				<div class="form-group">
					<label for="p-info">Reference / Info</label>
					<input
						id="p-info"
						v-model="paymentForm.info"
						type="text"
						placeholder="Invoice # or Note"
					/>
				</div>
				<div
					v-if="selectedAssetForPayment?.billing_freq !== 'One-time'"
					class="form-group"
				>
					<label for="p-next-billing">Next Billing Date</label>
					<input
						id="p-next-billing"
						v-model="paymentForm.next_billing"
						type="date"
					/>
				</div>
				<div class="form-actions">
					<button class="back-btn" @click="showPaymentModal = false">
						Cancel
					</button>
					<button
						class="btn btn-primary"
						@click="handleRecordPayment"
					>
						Confirm & Advance Billing
					</button>
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

.card-group {
	display: flex;
	flex-direction: column;
	gap: 4px;
}

.card-footer-audit {
	font-size: 0.7rem;
	color: var(--text-muted);
	padding: 0 8px;
	opacity: 0.8;
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
	margin-bottom: 16px;
	flex-wrap: wrap;
	gap: 12px;
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
	max-width: 550px;
	padding: 24px;
	box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);

	@media (max-width: 640px) {
		width: 95%;
		padding: 20px 16px;
		max-height: 90vh;
		overflow-y: auto;
	}
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
	gap: 1.5rem;
}

.form-row {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 1rem;
}

.sub-text {
	font-size: 0.8rem;
	color: var(--text-muted);
	margin-top: 0.2rem;
}

.unlinked-count {
	font-size: 0.8rem;
	background-color: var(--bg-muted);
	color: var(--text-muted);
	padding: 2px 8px;
	border-radius: 10px;
	font-weight: 500;
}

.status-badge.active {
	background-color: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.status-badge.paused {
	background-color: var(--badge-drop-in-bg);
	color: var(--badge-drop-in-text);
}

.status-badge.cancelled {
	background-color: var(--badge-inactive-bg);
	color: var(--badge-inactive-text);
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 0.5rem;
}

.form-group label {
	font-weight: 600;
	font-size: 0.875rem;
	color: var(--text-muted);
}

.form-group input,
.form-group select,
.form-group textarea {
	padding: 0.625rem;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 0.875rem;
	width: 100%;
	background-color: var(--bg-card);
	color: var(--text-main);
}

.form-group textarea {
	resize: vertical;
	min-height: 80px;
	font-family: inherit;
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

@media (max-width: 640px) {
	/* Hide Email and Phone in contacts table on mobile */
	section:nth-of-type(1) .data-table th:nth-child(3),
	section:nth-of-type(1) .data-table td:nth-child(3),
	section:nth-of-type(1) .data-table th:nth-child(4),
	section:nth-of-type(1) .data-table td:nth-child(4) {
		display: none;
	}

	/* Hide Type in sites table on mobile */
	section:nth-of-type(2) .data-table th:nth-child(2),
	section:nth-of-type(2) .data-table td:nth-child(2) {
		display: none;
	}
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
