<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useOrganizationStore } from "../stores/organization";
import { useDataStore } from "../stores/data";
import { useAuthStore } from "../stores/auth";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";

const router = useRouter();
const organizationStore = useOrganizationStore();
const dataStore = useDataStore();
const authStore = useAuthStore();

const searchQuery = ref("");
let searchTimeout: number | null = null;
let abortController: AbortController | null = null;

onMounted(async () => {
	await organizationStore.fetchOrganizations();
	dataStore.initData();
	organizationStore.organizations.forEach((org) => {
		organizationStore.fetchOrganizationSites(org.id);
	});
});

const getLinkedSites = (orgId: number) => {
	return dataStore.enrichedSites.filter((s) => s.organization_id === orgId);
};

const handleSearch = () => {
	if (searchTimeout) {
		clearTimeout(searchTimeout);
	}

	searchTimeout = window.setTimeout(async () => {
		if (abortController) {
			abortController.abort();
		}
		abortController = new AbortController();

		try {
			await organizationStore.fetchOrganizations(
				searchQuery.value,
				abortController.signal,
			);
			organizationStore.organizations.forEach((org) => {
				organizationStore.fetchOrganizationSites(org.id);
			});
		} catch (e: any) {
			if (e.name !== "AbortError") {
				console.error("Search failed", e);
			}
		}
	}, 300);
};

const goToOrganization = (id: number) => {
	router.push({ name: "organization-detail", params: { id: id.toString() } });
};

const showCreateModal = ref(false);
const newOrganization = ref({
	name: "",
	vat_number: "",
	info: "",
});

const handleCreateOrganization = async () => {
	if (!newOrganization.value.name) return;
	try {
		await organizationStore.createOrganization(newOrganization.value);
		showCreateModal.value = false;
		newOrganization.value = { name: "", vat_number: "", info: "" };
		organizationStore.fetchOrganizations(searchQuery.value);
	} catch (e) {
		console.error("Failed to create organization:", e);
	}
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Organization Management" />

		<div v-if="organizationStore.error" class="error-banner">
			<p><strong>Error:</strong> {{ organizationStore.error }}</p>
		</div>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search organizations by name or VAT..."
				class="search-input"
				@input="handleSearch"
			/>
			<button
				v-if="authStore.canEdit"
				class="btn btn-primary"
				@click="showCreateModal = true"
			>
				Add Organization
			</button>
		</div>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Name</th>
						<th class="hide-mobile">Linked Sites</th>
						<th class="text-right"></th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							organizationStore.isLoading &&
							organizationStore.organizations.length === 0
						"
					>
						<td colspan="3" class="hide-mobile">
							<LoadingSpinner
								message="Loading organizations..."
							/>
						</td>
						<td colspan="2" class="show-mobile">
							<LoadingSpinner
								message="Loading organizations..."
							/>
						</td>
					</tr>
					<tr
						v-else-if="organizationStore.organizations.length === 0"
					>
						<td colspan="3" class="empty-state hide-mobile">
							<span v-if="searchQuery"
								>No organizations found matching "{{
									searchQuery
								}}".</span
							>
							<span v-else>No organizations available.</span>
						</td>
						<td colspan="2" class="empty-state show-mobile">
							<span v-if="searchQuery"
								>No organizations found matching "{{
									searchQuery
								}}".</span
							>
							<span v-else>No organizations available.</span>
						</td>
					</tr>
					<tr
						v-for="organization in organizationStore.organizations"
						:key="organization.id"
						class="clickable-row"
						@click="goToOrganization(organization.id)"
					>
						<td>
							<strong :title="organization.info ?? undefined">{{
								organization.name
							}}</strong>
						</td>
						<td class="hide-mobile">
							<div class="flex-row gap-1 font-sm">
								<template
									v-if="
										getLinkedSites(organization.id).length >
										0
									"
								>
									<span
										v-for="site in getLinkedSites(
											organization.id,
										).slice(0, 5)"
										:key="site.id"
										class="status-badge badge-sm"
									>
										{{ site.domain }}
									</span>
									<span
										v-if="
											getLinkedSites(organization.id)
												.length > 5
										"
										class="status-badge info badge-sm"
									>
										+{{
											getLinkedSites(organization.id)
												.length - 5
										}}
										others
									</span>
								</template>
								<span v-else class="text-muted">—</span>
							</div>
						</td>
						<td class="text-right text-muted">
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
								class="opacity-50"
							>
								<polyline points="9 18 15 12 9 6"></polyline>
							</svg>
						</td>
					</tr>
				</tbody>
			</table>
		</main>

		<!-- Create Modal -->
		<div
			v-if="showCreateModal"
			class="modal-overlay"
			@click.self="showCreateModal = false"
		>
			<div class="modal-content card">
				<h2>Add New Organization</h2>
				<form
					class="content"
					@submit.prevent="handleCreateOrganization"
				>
					<div class="form-group">
						<label for="name">Organization Name*</label>
						<input
							id="name"
							v-model="newOrganization.name"
							type="text"
							required
							placeholder="Enter organization name"
						/>
					</div>
					<div class="form-group">
						<label for="vat">VAT Number</label>
						<input
							id="vat"
							v-model="newOrganization.vat_number"
							type="text"
							placeholder="e.g. FI123456789"
						/>
					</div>
					<div class="form-group">
						<label for="info">Additional Information</label>
						<textarea
							id="info"
							v-model="newOrganization.info"
							placeholder="Notes, address, etc."
						></textarea>
					</div>
					<div class="form-actions">
						<button
							type="button"
							class="btn btn-outline"
							@click="showCreateModal = false"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="
								!newOrganization.name ||
								organizationStore.isLoading
							"
						>
							Create Organization
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>
