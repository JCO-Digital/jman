<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { RouterLink, useRouter } from "vue-router";
import { useOrganizationStore } from "../stores/organization";
import { useDataStore } from "../stores/data";
import { useAuthStore } from "../stores/auth";
import ViewHeader from "../components/ViewHeader.vue";
import ListToolbar from "../components/ListToolbar.vue";
import SortableTh from "../components/SortableTh.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import AppIcon from "../components/AppIcon.vue";
import { useListState } from "../composables/useListState";
import { useClientList } from "../composables/useClientList";

const router = useRouter();
const organizationStore = useOrganizationStore();
const dataStore = useDataStore();
const authStore = useAuthStore();

// /organizations?q=&sort=&dir=&page=&per=
const list = useListState({
	sortKeys: ["name", "sites"],
	defaultSort: "name",
	descFirst: ["sites"],
});

let abortController: AbortController | null = null;

// The search runs on the server; a newer search cancels an older one.
const load = async () => {
	abortController?.abort();
	abortController = new AbortController();
	try {
		await organizationStore.fetchOrganizations(
			list.query.value,
			abortController.signal,
		);
		organizationStore.organizations.forEach((org) => {
			organizationStore.fetchOrganizationSites(org.id);
		});
	} catch (e: unknown) {
		if ((e as Error).name !== "AbortError") {
			console.error("Search failed", e);
		}
	}
};

onMounted(() => {
	load();
	dataStore.initData();
});
watch(list.query, load);

const getLinkedSites = (orgId: number) => {
	return dataStore.enrichedSites.filter((s) => s.organization_id === orgId);
};

const organizations = useClientList(
	() => organizationStore.organizations,
	list,
	{
		sortValue: (org, key) =>
			key === "name" ? org.name : getLinkedSites(org.id).length,
	},
);

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
		load();
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

		<ListToolbar
			:list="list"
			placeholder="Search organizations by name or VAT"
		>
			<template v-if="authStore.canEdit" #actions>
				<button class="btn btn-primary" @click="showCreateModal = true">
					Add Organization
				</button>
			</template>
		</ListToolbar>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<SortableTh :list="list" sort-key="name"
							>Name</SortableTh
						>
						<SortableTh :list="list" sort-key="sites">
							Linked Sites
						</SortableTh>
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
						<td colspan="3">
							<LoadingSpinner
								message="Loading organizations..."
							/>
						</td>
					</tr>
					<tr
						v-else-if="organizationStore.organizations.length === 0"
					>
						<td colspan="3" class="empty-state">
							<span v-if="list.query.value"
								>No organizations found matching "{{
									list.query.value
								}}".</span
							>
							<span v-else>No organizations available.</span>
						</td>
					</tr>
					<tr
						v-for="organization in organizations.pageItems.value"
						:key="organization.id"
						class="clickable-row"
						@click="goToOrganization(organization.id)"
					>
						<td>
							<RouterLink
								:to="{
									name: 'organization-detail',
									params: { id: organization.id },
								}"
								class="row-link font-semibold"
								:title="organization.info ?? undefined"
								@click.stop
							>
								{{ organization.name }}
							</RouterLink>
						</td>
						<td>
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
							<AppIcon
								name="chevron-right"
								size="16"
								class="opacity-50"
							/>
						</td>
					</tr>
				</tbody>
			</table>
			<Pagination
				:list="list"
				:page="organizations.page.value"
				:total="organizations.total.value"
			/>
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
