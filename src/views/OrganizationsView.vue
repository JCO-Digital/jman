<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useOrganizationStore } from "../stores/organization";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";

const router = useRouter();
const organizationStore = useOrganizationStore();

const searchQuery = ref("");
let searchTimeout: number | null = null;
let abortController: AbortController | null = null;

onMounted(() => {
	organizationStore.fetchOrganizations();
});

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
				type="text"
				placeholder="Search organizations by name or VAT..."
				class="search-input"
				v-model="searchQuery"
				@input="handleSearch"
			/>
			<button class="btn btn-primary" @click="showCreateModal = true">
				Add Organization
			</button>
		</div>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Name</th>
						<th>VAT Number</th>
						<th>Information</th>
						<th class="actions-cell"></th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							organizationStore.isLoading &&
							organizationStore.organizations.length === 0
						"
					>
						<td colspan="4">
							<LoadingSpinner message="Loading organizations..." />
						</td>
					</tr>
					<tr v-else-if="organizationStore.organizations.length === 0">
						<td colspan="4" class="empty-state">
							<span v-if="searchQuery"
								>No organizations found matching "{{ searchQuery }}".</span
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
							<strong>{{ organization.name }}</strong>
						</td>
						<td>{{ organization.vat_number || "—" }}</td>
						<td>
							<div
								class="text-truncate"
								:title="organization.info ?? undefined"
							>
								{{ organization.info || "—" }}
							</div>
						</td>
						<td class="actions-cell">
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
								class="chevron-icon"
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
				<form @submit.prevent="handleCreateOrganization" class="form-layout">
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
							placeholder="e.g. DE123456789"
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
							class="back-btn"
							@click="showCreateModal = false"
						>
							Cancel
						</button>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="!newOrganization.name || organizationStore.isLoading"
						>
							Create Organization
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>

<style scoped>
.controls {
	display: flex;
	gap: 16px;
	margin-bottom: 24px;
}

.search-input {
	flex: 1;
}

.text-truncate {
	max-width: 400px;
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.actions-cell {
	width: 40px;
	text-align: right;
	color: var(--text-muted);
}

.chevron-icon {
	opacity: 0.5;
}

.clickable-row:hover .chevron-icon {
	opacity: 1;
	color: var(--primary);
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
.form-group textarea {
	padding: 10px 12px;
	border: 1px solid var(--border-input);
	border-radius: 6px;
	background: var(--bg-body);
	color: var(--text-main);
	font-size: 0.95rem;
	transition: border-color 0.2s;
}

.form-group input:focus,
.form-group textarea:focus {
	outline: none;
	border-color: var(--primary);
}

.form-group textarea {
	min-height: 120px;
	resize: vertical;
}

.form-actions {
	display: flex;
	justify-content: flex-end;
	gap: 12px;
	margin-top: 8px;
}
</style>
