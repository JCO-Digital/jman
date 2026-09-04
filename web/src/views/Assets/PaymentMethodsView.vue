<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { usePaymentMethodsStore } from "../../stores/paymentMethods";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import type { PaymentMethod, PaymentMethodType } from "../../types";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import { useConfirm } from "../../composables/useConfirm";

const paymentMethodsStore = usePaymentMethodsStore();
const authStore = useAuthStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const searchQuery = ref("");
const showModal = ref(false);
const editingMethod = ref<PaymentMethod | null>(null);

const pmForm = ref({
	name: "",
	type: "Buy" as PaymentMethodType,
	expiry_date: "",
});

const typeOptions: PaymentMethodType[] = ["Buy", "Sell"];

const filteredMethods = computed(() => {
	if (!searchQuery.value) return paymentMethodsStore.paymentMethods;
	const query = searchQuery.value.toLowerCase();
	return paymentMethodsStore.paymentMethods.filter(
		(pm) =>
			pm.name.toLowerCase().includes(query) ||
			pm.type.toLowerCase().includes(query),
	);
});

const loadPaymentMethods = async () => {
	await paymentMethodsStore.fetchPaymentMethods();
};

onMounted(loadPaymentMethods);

const openAddModal = () => {
	editingMethod.value = null;
	pmForm.value = {
		name: "",
		type: "Buy",
		expiry_date: "",
	};
	showModal.value = true;
};

const openEditModal = (pm: PaymentMethod) => {
	editingMethod.value = pm;
	pmForm.value = {
		name: pm.name,
		type: pm.type,
		expiry_date: pm.expiry_date ? pm.expiry_date.split("T")[0] || "" : "",
	};
	showModal.value = true;
};

const handleSubmit = async () => {
	try {
		const payload = {
			name: pmForm.value.name,
			type: pmForm.value.type,
			expiry_date: pmForm.value.expiry_date
				? new Date(pmForm.value.expiry_date).toISOString()
				: null,
		};
		if (editingMethod.value) {
			await paymentMethodsStore.updatePaymentMethod(
				editingMethod.value.id,
				payload,
			);
		} else {
			await paymentMethodsStore.createPaymentMethod(payload);
		}
		showModal.value = false;
		await loadPaymentMethods();
	} catch (e: any) {
		toast.addToast("Failed to save payment method: " + e.message, "error");
	}
};

const handleDelete = async (id: number) => {
	if (
		!(await confirm(
			"Are you sure you want to delete this payment method?",
			{
				danger: true,
			},
		))
	)
		return;
	try {
		await paymentMethodsStore.deletePaymentMethod(id);
		await loadPaymentMethods();
	} catch (e: any) {
		toast.addToast(
			"Failed to delete payment method: " + e.message,
			"error",
		);
	}
};

const formatDate = (dateStr: string | null) => {
	if (!dateStr) return "—";
	return new Date(dateStr).toLocaleDateString();
};

const isExpiringSoon = (pm: PaymentMethod) => {
	if (pm.type !== "Buy" || !pm.expiry_date) return false;
	const expiry = new Date(pm.expiry_date);
	const in30Days = new Date();
	in30Days.setDate(in30Days.getDate() + 30);
	return expiry <= in30Days;
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Payment Methods">
			<template #actions>
				<div class="flex-row gap-2 items-center">
					<button
						v-if="authStore.canEdit"
						class="btn btn-primary"
						@click="openAddModal"
					>
						<AppIcon name="plus-circle" size="18" />
						Create Payment Method
					</button>
					<button
						class="btn btn-secondary"
						@click="$router.push({ name: 'asset-templates' })"
					>
						<AppIcon name="tag" size="18" />
						<span>Manage Templates</span>
					</button>
				</div>
			</template>
		</ViewHeader>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search payment methods by name or type..."
				class="search-input"
			/>
		</div>

		<main class="content">
			<div
				v-if="
					paymentMethodsStore.isLoading &&
					paymentMethodsStore.paymentMethods.length === 0
				"
				class="loading-container"
			>
				<LoadingSpinner message="Loading payment methods..." />
			</div>

			<div v-else class="asset-grid">
				<div
					v-if="filteredMethods.length === 0"
					class="card empty-state"
				>
					No payment methods found.
				</div>
				<div
					v-for="pm in filteredMethods"
					:key="pm.id"
					class="card asset-card"
				>
					<div class="asset-card-header">
						<span
							:class="[
								'status-badge',
								pm.type === 'Buy' ? 'active' : 'inactive',
							]"
						>
							{{ pm.type }}
						</span>
						<div v-if="authStore.canEdit" class="row-actions">
							<button
								class="icon-btn icon-btn-sm"
								title="Edit"
								@click="openEditModal(pm)"
							>
								<AppIcon name="edit" size="16" />
							</button>
							<button
								class="icon-btn icon-btn-sm danger"
								title="Delete"
								@click="handleDelete(pm.id)"
							>
								<AppIcon name="trash" size="16" />
							</button>
						</div>
					</div>

					<div class="asset-card-content">
						<h3>{{ pm.name }}</h3>
					</div>

					<div class="asset-card-footer">
						<div class="freq">
							Expires: {{ formatDate(pm.expiry_date) }}
						</div>
						<span
							v-if="isExpiringSoon(pm)"
							class="status-badge inactive"
							title="This card expires within 30 days"
						>
							Expiring soon
						</span>
					</div>
				</div>
			</div>
		</main>

		<!-- Payment Method Modal -->
		<div
			v-if="showModal"
			class="modal-overlay"
			@click.self="showModal = false"
		>
			<div class="modal-content card">
				<h2>
					{{
						editingMethod
							? "Edit Payment Method"
							: "New Payment Method"
					}}
				</h2>
				<form class="form-layout" @submit.prevent="handleSubmit">
					<div class="form-group">
						<label for="pm-name">Name</label>
						<input
							id="pm-name"
							v-model="pmForm.name"
							type="text"
							placeholder="e.g. Amex Business or Invoice"
							required
						/>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="pm-type">Type</label>
							<select id="pm-type" v-model="pmForm.type">
								<option
									v-for="type in typeOptions"
									:key="type"
									:value="type"
								>
									{{ type }}
								</option>
							</select>
						</div>
						<div class="form-group">
							<label for="pm-expiry"
								>Expiry Date (optional)</label
							>
							<input
								id="pm-expiry"
								v-model="pmForm.expiry_date"
								type="date"
							/>
						</div>
					</div>

					<div class="form-actions">
						<button
							type="button"
							class="back-btn"
							@click="showModal = false"
						>
							Cancel
						</button>
						<button type="submit" class="btn btn-primary">
							{{
								editingMethod
									? "Update Payment Method"
									: "Create Payment Method"
							}}
						</button>
					</div>
				</form>
			</div>
		</div>
	</div>
</template>
