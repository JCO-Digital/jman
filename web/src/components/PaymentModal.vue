<script setup lang="ts">
import { ref, watch } from "vue";
import type { EnrichedOrganizationAsset, BillingFrequency } from "../types";
import { useAssetStore } from "../stores/assetStore";
import { useToastStore } from "../stores/toast";

const props = defineProps<{
	modelValue: boolean;
	asset: EnrichedOrganizationAsset | null;
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: boolean): void;
	(e: "saved"): void;
}>();

const assetStore = useAssetStore();
const toast = useToastStore();

const paymentForm = ref({
	amount: 0,
	info: "",
	next_billing: "",
});

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

watch(
	() => props.modelValue,
	(newVal) => {
		if (newVal && props.asset) {
			const suggestedNextDate = calculateNextBillingDate(
				props.asset.next_billing,
				props.asset.billing_freq,
			);
			paymentForm.value = {
				amount: props.asset.price,
				info: `Renewal ${new Date().toLocaleDateString()}`,
				next_billing: suggestedNextDate,
			};
		}
	},
);

const handleRecordPayment = async () => {
	if (!props.asset) return;
	try {
		const payload = { ...paymentForm.value };
		if (payload.next_billing) {
			payload.next_billing = new Date(payload.next_billing).toISOString();
		}

		await assetStore.recordPayment(props.asset.id, payload);
		emit("saved");
		close();
	} catch (e: any) {
		toast.addToast("Failed to record payment: " + e.message, "error");
	}
};

const close = () => {
	emit("update:modelValue", false);
};

const formatCurrency = (cents: number) => {
	return new Intl.NumberFormat("de-DE", {
		style: "currency",
		currency: "EUR",
	}).format(cents / 100);
};
</script>

<template>
	<div v-if="modelValue" class="modal-overlay" @click.self="close">
		<div class="modal-content card">
			<h2>Record Payment</h2>
			<p v-if="asset" class="mb-4">
				Recording payment for
				<strong>
					{{ asset.asset?.name || asset.asset_name }}
					<span v-if="asset.identifier" class="text-muted"
						>({{ asset.identifier }})</span
					>
				</strong>
			</p>
			<div class="form-layout">
				<div class="form-group">
					<label for="p-amount">Amount (in Cents)</label>
					<input
						id="p-amount"
						v-model.number="paymentForm.amount"
						type="number"
						placeholder="e.g. 5000 for 50,00€"
					/>
					<div class="sub-text mt-1">
						Current Price: {{ formatCurrency(asset?.price || 0) }}
					</div>
				</div>
				<div class="form-group">
					<label for="p-info">Payment Info / Reference</label>
					<input
						id="p-info"
						v-model="paymentForm.info"
						type="text"
						placeholder="e.g. Invoice #123"
					/>
				</div>
				<div
					v-if="asset?.billing_freq !== 'One-time'"
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
					<button class="btn btn-outline" @click="close">
						Cancel
					</button>
					<button
						class="btn btn-primary"
						@click="handleRecordPayment"
					>
						Record Payment
					</button>
				</div>
			</div>
		</div>
	</div>
</template>
