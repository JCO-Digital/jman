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
	Site,
	EnrichedOrganizationAsset,
	Asset,
} from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import EditableInfoCard from "../components/EditableInfoCard.vue";
import AppIcon from "../components/AppIcon.vue";
import AssetEditModal from "../components/AssetEditModal.vue";
import ContactModal from "../components/ContactModal.vue";
import LinkSiteModal from "../components/LinkSiteModal.vue";
import PaymentModal from "../components/PaymentModal.vue";
import { useConfirm } from "../composables/useConfirm";

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
const { confirm } = useConfirm();

const organizationId = parseInt(props.id, 10);
if (isNaN(organizationId)) router.replace({ name: "organizations" });
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
				assetStore.fetchAssets(),
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

const openAddContact = () => {
	editingContact.value = null;
	showContactModal.value = true;
};

const openEditContact = (contact: Contact) => {
	editingContact.value = contact;
	showContactModal.value = true;
};

const handleContactSaved = async () => {
	contacts.value =
		await organizationStore.fetchOrganizationContacts(organizationId);
};

const handleDeleteContact = async (id: number) => {
	if (
		!(await confirm("Are you sure you want to delete this contact?", {
			danger: true,
		}))
	)
		return;
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
		!(await confirm(
			`Are you sure you want to delete ${organization.value?.name}? This will also delete all associated contacts.`,
			{ danger: true },
		))
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

const handleSiteLinked = async () => {
	linkedSites.value =
		await organizationStore.fetchOrganizationSites(organizationId);
};

const handleUnlinkSite = async (siteId: number) => {
	if (!(await confirm("Are you sure you want to unlink this site?"))) return;
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
const assetPrefill = ref<{ template: Asset; siteId: number | null } | null>(
	null,
);

const openAddAsset = () => {
	editingOrgAsset.value = null;
	assetPrefill.value = null;
	showLinkAssetModal.value = true;
};

const openEditAsset = (oa: EnrichedOrganizationAsset) => {
	editingOrgAsset.value = oa;
	assetPrefill.value = null;
	showLinkAssetModal.value = true;
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
		template: Asset;
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
					template: matchingTemplate,
				});
			}
		});
	});

	return unlinked;
});

const convertToAsset = (p: { site: Site; template: Asset }) => {
	editingOrgAsset.value = null;
	assetPrefill.value = { template: p.template, siteId: p.site.id };
	showLinkAssetModal.value = true;
};

const openPaymentModal = (asset: EnrichedOrganizationAsset) => {
	selectedAssetForPayment.value = asset;
	showPaymentModal.value = true;
};

const handlePaymentSaved = async () => {
	orgAssets.value = await assetStore.fetchOrganizationAssets(organizationId);
};

const handleUnlinkAsset = async (id: number) => {
	if (!(await confirm("Are you sure you want to unlink this asset?"))) return;
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
					class="btn btn-outline danger"
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
						<table class="data-table hide-col-3-sm hide-col-4-sm">
							<thead>
								<tr>
									<th>Name</th>
									<th>Type</th>
									<th>Email</th>
									<th>Phone</th>
									<th
										v-if="authStore.canEdit"
										class="text-right"
									></th>
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
									<td class="font-medium">
										{{ contact.name }}
									</td>
									<td>
										<span
											:class="[
												'status-badge',
												'badge-sm',
												contact.type.toLowerCase(),
											]"
										>
											{{ contact.type }}
										</span>
									</td>
									<td class="text-muted">
										{{ contact.email || "—" }}
									</td>
									<td class="text-muted">
										{{ contact.phone || "—" }}
									</td>
									<td class="text-right">
										<div class="flex-row gap-2 justify-end">
											<button
												class="icon-btn icon-btn-sm"
												title="Edit Contact"
												@click="
													openEditContact(contact)
												"
											>
												<AppIcon
													name="edit"
													size="16"
												/>
											</button>
											<button
												class="icon-btn icon-btn-sm danger"
												title="Delete Contact"
												@click="
													handleDeleteContact(
														contact.id,
													)
												"
											>
												<AppIcon
													name="trash"
													size="16"
												/>
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
						<table class="data-table hide-col-2-sm">
							<thead>
								<tr>
									<th>Domain</th>
									<th>Type</th>
									<th>PHP</th>
									<th
										v-if="authStore.canEdit"
										class="text-right"
									></th>
								</tr>
							</thead>
							<tbody>
								<tr v-if="linkedSites.length === 0">
									<td
										:colspan="authStore.canEdit ? 4 : 3"
										class="empty-state"
									>
										No sites linked to this organization.
									</td>
								</tr>
								<tr v-for="site in linkedSites" :key="site.id">
									<td>
										<a
											href="#"
											class="site-link font-medium"
											@click.prevent="goToSite(site.id)"
										>
											{{ site.domain }}
										</a>
									</td>
									<td>
										<span
											class="status-badge badge-sm"
											:class="
												site.is_wordpress
													? 'info'
													: 'default'
											"
										>
											{{
												site.is_wordpress
													? "WordPress"
													: "App"
											}}
										</span>
									</td>
									<td class="text-muted">
										{{ site.php_version }}
									</td>
									<td
										v-if="authStore.canEdit"
										class="text-right"
									>
										<div class="flex-row gap-2 justify-end">
											<button
												class="icon-btn icon-btn-sm danger"
												title="Unlink Site"
												@click="
													handleUnlinkSite(site.id)
												"
											>
												<AppIcon
													name="trash"
													size="16"
												/>
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
										class="text-right"
									></th>
								</tr>
							</thead>
							<tbody>
								<tr v-if="orgAssets.length === 0">
									<td
										:colspan="authStore.canEdit ? 7 : 6"
										class="empty-state"
									>
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
										class="text-right"
									>
										<div class="flex-row gap-2 justify-end">
											<button
												class="icon-btn icon-btn-sm"
												title="Edit Asset"
												@click="openEditAsset(oa)"
											>
												<AppIcon
													name="edit"
													size="16"
												/>
											</button>
											<button
												class="icon-btn icon-btn-sm"
												title="Record Payment"
												@click="openPaymentModal(oa)"
											>
												<AppIcon
													name="credit-card"
													size="16"
												/>
											</button>
											<button
												class="icon-btn icon-btn-sm danger"
												title="Unlink Asset"
												@click="
													handleUnlinkAsset(oa.id)
												"
											>
												<AppIcon
													name="trash"
													size="16"
												/>
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
										class="text-right"
									></th>
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
										class="text-right"
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
		<ContactModal
			v-model="showContactModal"
			:contact="editingContact"
			:organization-id="organizationId"
			@saved="handleContactSaved"
		/>

		<LinkSiteModal
			v-model="showLinkSiteModal"
			:organization-id="organizationId"
			:linked-sites="linkedSites"
			@linked="handleSiteLinked"
		/>

		<AssetEditModal
			v-model="showLinkAssetModal"
			:asset="editingOrgAsset"
			:prefill="assetPrefill"
			:sites="linkedSites"
			:organization-id="organizationId"
			@saved="loadData"
		/>

		<PaymentModal
			v-model="showPaymentModal"
			:asset="selectedAssetForPayment"
			@saved="handlePaymentSaved"
		/>
	</div>
</template>
