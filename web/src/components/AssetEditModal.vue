<script setup lang="ts">
import { ref, watch, computed, onMounted } from "vue";
import type {
	Asset,
	BillingFrequency,
	OrganizationAssetStatus,
	EnrichedOrganizationAsset,
	Site,
	Organization,
} from "../types";
import { useAssetStore } from "../stores/assetStore";
import { usePaymentMethodsStore } from "../stores/paymentMethods";

const props = defineProps<{
	modelValue: boolean;
	asset: EnrichedOrganizationAsset | null;
	prefill?: { template: Asset; siteId: number | null } | null;
	sites: Site[];
	organizations?: Organization[];
	organizationId?: number | null;
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: boolean): void;
	(e: "update:organizationId", value: number | null): void;
	(e: "saved"): void;
}>();

const assetStore = useAssetStore();
const paymentMethodsStore = usePaymentMethodsStore();
const assetSearchQuery = ref("");
const availableAssetTemplates = ref<Asset[]>([]);

const assetForm = ref({
	asset_id: null as number | null,
	site_id: null as number | null,
	identifier: "",
	billing_freq: "Yearly" as BillingFrequency,
	next_billing: "",
	status: "active" as OrganizationAssetStatus,
	description: "",
	payment_method_id: null as number | null,
	license_key: "",
});
const priceInput = ref("0.00");

onMounted(() => {
	paymentMethodsStore.fetchPaymentMethods();
});

// Read-only cost info from the linked template, shown for context only (not editable here).
// Populated from props.asset when editing, or from the selected/prefilled template when linking.
const linkedTemplateCost = ref<{
	purchase_price?: number | null;
	quantity?: number | null;
	next_payment?: string | null;
	management_url?: string | null;
	management_account?: string | null;
	license_key?: string | null;
} | null>(null);

const formatCurrency = (cents: number) => {
	return new Intl.NumberFormat("de-DE", {
		style: "currency",
		currency: "EUR",
	}).format(cents / 100);
};

const isEditing = ref(false);

const normalizePriceInput = () => {
	priceInput.value = (parseFloat(priceInput.value) || 0).toFixed(2);
};

const isOrgSelected = computed(() => {
	return !!(props.asset?.organization_id || props.organizationId);
});

watch(
	() => props.modelValue,
	(newVal) => {
		if (newVal) {
			if (props.asset) {
				isEditing.value = true;
				assetForm.value = {
					asset_id: props.asset.asset_id,
					site_id: props.asset.site_id,
					identifier: props.asset.identifier || "",
					billing_freq: props.asset.billing_freq,
					next_billing: props.asset.next_billing
						? props.asset.next_billing.split("T")[0] || ""
						: "",
					status: props.asset.status,
					description: props.asset.description || "",
					payment_method_id: props.asset.payment_method_id,
					license_key: props.asset.license_key || "",
				};
				priceInput.value = (props.asset.price / 100).toFixed(2);
				assetSearchQuery.value =
					props.asset.asset?.name || props.asset.asset_name || "";
				linkedTemplateCost.value = {
					purchase_price: props.asset.asset_purchase_price,
					quantity: props.asset.asset_quantity,
					next_payment: props.asset.asset_next_payment,
					management_url: props.asset.asset_management_url,
					management_account: props.asset.asset_management_account,
					license_key: props.asset.asset_license_key,
				};
			} else if (props.prefill) {
				isEditing.value = false;
				const { template, siteId } = props.prefill;
				assetForm.value = {
					asset_id: template.id,
					site_id: siteId,
					identifier: template.identifier || "",
					billing_freq: template.default_freq || "Yearly",
					next_billing: new Date().toISOString().split("T")[0] || "",
					status: "active",
					description: "",
					payment_method_id: template.payment_method_id,
					license_key: "",
				};
				priceInput.value = (
					(template.default_price || 0) / 100
				).toFixed(2);
				assetSearchQuery.value = template.name;
				availableAssetTemplates.value = [];
				linkedTemplateCost.value = {
					purchase_price: template.purchase_price,
					quantity: template.quantity,
					next_payment: template.next_payment,
					management_url: template.management_url,
					management_account: template.management_account,
					license_key: template.license_key,
				};
			} else {
				isEditing.value = false;
				assetForm.value = {
					asset_id: null,
					site_id: null,
					identifier: "",
					billing_freq: "Yearly",
					next_billing: "",
					status: "active",
					description: "",
					payment_method_id: null,
					license_key: "",
				};
				priceInput.value = "0.00";
				assetSearchQuery.value = "";
				linkedTemplateCost.value = null;
			}
		}
	},
);

const searchAssets = async () => {
	if (assetSearchQuery.value.length < 2) {
		availableAssetTemplates.value = [];
		return;
	}
	availableAssetTemplates.value = await assetStore.searchAssetTemplates(
		assetSearchQuery.value,
	);
};

const selectAssetTemplate = (template: Asset) => {
	assetForm.value.asset_id = template.id;
	assetForm.value.identifier = template.identifier || "";
	priceInput.value = ((template.default_price || 0) / 100).toFixed(2);
	assetForm.value.billing_freq = template.default_freq || "Yearly";
	assetForm.value.next_billing = new Date().toISOString().split("T")[0] || "";
	assetForm.value.payment_method_id = template.payment_method_id;
	assetForm.value.license_key = "";
	assetSearchQuery.value = template.name;
	availableAssetTemplates.value = [];
	linkedTemplateCost.value = {
		purchase_price: template.purchase_price,
		quantity: template.quantity,
		next_payment: template.next_payment,
		management_url: template.management_url,
		management_account: template.management_account,
		license_key: template.license_key,
	};
};

const handleSave = async () => {
	try {
		const payload = {
			...assetForm.value,
			price: Math.round((parseFloat(priceInput.value) || 0) * 100),
		};

		if (payload.next_billing) {
			payload.next_billing = new Date(payload.next_billing).toISOString();
		}

		if (isEditing.value && props.asset) {
			await assetStore.updateOrganizationAsset(props.asset.id, payload);
		} else {
			const orgId = props.asset?.organization_id || props.organizationId;
			if (orgId) {
				await assetStore.linkAsset(orgId, payload);
			} else {
				throw new Error("Organization ID missing");
			}
		}

		emit("saved");
		emit("update:modelValue", false);
	} catch (e) {
		console.error("Failed to save asset", e);
		// Error handling should be done via toast in the parent or here
		throw e;
	}
};

const close = () => {
	emit("update:modelValue", false);
};
</script>

<template>
	<div v-if="modelValue" class="modal-overlay" @click.self="close">
		<div class="modal-content card">
			<h2>{{ isEditing ? "Edit Asset" : "Link Asset" }}</h2>
			<div class="form-layout">
				<div
					v-if="!isEditing && organizations && !organizationId"
					class="form-group"
				>
					<label for="a-org">Select Organization</label>
					<select
						id="a-org"
						:value="organizationId"
						@change="
							(e) =>
								emit(
									'update:organizationId',
									parseInt(
										(e.target as HTMLSelectElement).value,
									),
								)
						"
					>
						<option :value="null">Select an organization...</option>
						<option
							v-for="org in organizations"
							:key="org.id"
							:value="org.id"
						>
							{{ org.name }}
						</option>
					</select>
				</div>

				<div
					v-if="!isEditing && organizations && organizationId"
					class="form-group"
				>
					<label>Organization</label>
					<div class="readonly-value">
						{{
							organizations.find((o) => o.id === organizationId)
								?.name
						}}
					</div>
				</div>

				<div v-if="isEditing" class="form-group">
					<label>Organization</label>
					<span class="readonly-value">{{
						asset?.organization_name
					}}</span>
				</div>

				<div v-if="!isEditing" class="form-group">
					<label for="a-search">Search Asset Template</label>
					<input
						id="a-search"
						v-model="assetSearchQuery"
						type="text"
						placeholder="Start typing to search templates..."
						:disabled="!isOrgSelected"
						@input="searchAssets"
					/>
					<div
						v-if="availableAssetTemplates.length > 0"
						class="search-results-dropdown"
					>
						<div
							v-for="t in availableAssetTemplates"
							:key="t.id"
							class="search-result-item"
							@click="selectAssetTemplate(t)"
						>
							<div class="res-name">{{ t.name }}</div>
							<div class="sub-text">
								{{ t.type }} •
								{{ (t.default_price || 0) / 100 }}€ /
								{{ t.default_freq }}
							</div>
						</div>
					</div>
				</div>

				<div v-if="isEditing" class="form-group">
					<label>Asset Template</label>
					<span class="readonly-value">{{ assetSearchQuery }}</span>
				</div>

				<div v-if="assetForm.asset_id" class="form-row">
					<div class="form-group">
						<label for="a-price">Custom Price (€)</label>
						<input
							id="a-price"
							v-model="priceInput"
							type="number"
							step="0.01"
							placeholder="e.g. 50.00"
							:disabled="!isOrgSelected"
							@blur="normalizePriceInput"
						/>
					</div>
					<div class="form-group">
						<label for="a-freq">Billing Frequency</label>
						<select
							id="a-freq"
							v-model="assetForm.billing_freq"
							:disabled="!isOrgSelected"
						>
							<option value="Monthly">Monthly</option>
							<option value="Quarterly">Quarterly</option>
							<option value="Yearly">Yearly</option>
							<option value="One-time">One-time</option>
						</select>
					</div>
				</div>

				<div class="form-group">
					<label for="a-site">Link to Site (Optional)</label>
					<select
						id="a-site"
						v-model="assetForm.site_id"
						:disabled="!isOrgSelected"
					>
						<option :value="null">
							No site (Organization-wide)
						</option>
						<option
							v-for="site in sites"
							:key="site.id"
							:value="site.id"
						>
							{{ site.domain }}
						</option>
					</select>
				</div>

				<div class="form-group">
					<label for="a-payment-method">Payment Method</label>
					<select
						id="a-payment-method"
						v-model="assetForm.payment_method_id"
						:disabled="!isOrgSelected"
					>
						<option :value="null">No payment method</option>
						<option
							v-for="pm in paymentMethodsStore.paymentMethods"
							:key="pm.id"
							:value="pm.id"
						>
							{{ pm.name }} ({{ pm.type }})
						</option>
					</select>
				</div>

				<div class="form-group">
					<label for="a-identifier">
						Identifier
						<span class="label-info">(Domain, Slug, etc.)</span>
					</label>
					<input
						id="a-identifier"
						v-model="assetForm.identifier"
						type="text"
						:disabled="!isOrgSelected"
					/>
				</div>

				<div class="form-group">
					<label for="a-license-key">
						License Key
						<span class="label-info"
							>(Overrides template default)</span
						>
					</label>
					<input
						id="a-license-key"
						v-model="assetForm.license_key"
						type="text"
						:placeholder="
							linkedTemplateCost?.license_key
								? `Inherited: ${linkedTemplateCost.license_key}`
								: 'e.g. key_xyz (optional)'
						"
						:disabled="!isOrgSelected"
					/>
				</div>

				<div v-if="linkedTemplateCost" class="form-group">
					<label>Template Cost Info (read-only)</label>
					<div class="readonly-value">
						Purchase:
						{{
							formatCurrency(
								linkedTemplateCost.purchase_price || 0,
							)
						}}
						<template
							v-if="(linkedTemplateCost.quantity || 1) !== 1"
						>
							/ {{ linkedTemplateCost.quantity }}
						</template>
						<template v-if="linkedTemplateCost.next_payment">
							· Next payment:
							{{
								new Date(
									linkedTemplateCost.next_payment,
								).toLocaleDateString()
							}}
						</template>
						<template v-if="linkedTemplateCost.management_account">
							· Account:
							{{ linkedTemplateCost.management_account }}
						</template>
						<template v-if="linkedTemplateCost.management_url">
							·
							<a
								:href="linkedTemplateCost.management_url"
								target="_blank"
								rel="noopener noreferrer"
								>Manage</a
							>
						</template>
					</div>
				</div>

				<div class="form-row">
					<div class="form-group">
						<label for="a-next-billing">Next Billing Date</label>
						<input
							id="a-next-billing"
							v-model="assetForm.next_billing"
							type="date"
							:disabled="!isOrgSelected"
						/>
					</div>
					<div class="form-group">
						<label for="a-status">Status</label>
						<select
							id="a-status"
							v-model="assetForm.status"
							:disabled="!isOrgSelected"
						>
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
						:disabled="!isOrgSelected"
					></textarea>
				</div>

				<div class="form-actions">
					<button class="btn btn-outline" @click="close">
						Cancel
					</button>
					<button
						class="btn btn-primary"
						:disabled="
							!assetForm.asset_id ||
							(!isEditing && organizations && !organizationId)
						"
						@click="handleSave"
					>
						{{ isEditing ? "Update Asset" : "Link Asset" }}
					</button>
				</div>
			</div>
		</div>
	</div>
</template>

<style scoped></style>
