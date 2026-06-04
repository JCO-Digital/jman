<script setup lang="ts">
import { ref, watch, computed } from "vue";
import type {
	Asset,
	BillingFrequency,
	OrganizationAssetStatus,
	EnrichedOrganizationAsset,
	Site,
	Organization,
} from "../types";
import { useAssetStore } from "../stores/assetStore";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{
	modelValue: boolean;
	asset: EnrichedOrganizationAsset | null;
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
const assetSearchQuery = ref("");
const availableAssetTemplates = ref<Asset[]>([]);

const assetForm = ref({
	asset_id: null as number | null,
	site_id: null as number | null,
	identifier: "",
	price_euro: 0,
	billing_freq: "Yearly" as BillingFrequency,
	next_billing: "",
	status: "active" as OrganizationAssetStatus,
	description: "",
});

const isEditing = ref(false);

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
					price_euro: props.asset.price / 100,
					billing_freq: props.asset.billing_freq,
					next_billing: props.asset.next_billing
						? props.asset.next_billing.split("T")[0] || ""
						: "",
					status: props.asset.status,
					description: props.asset.description || "",
				};
				assetSearchQuery.value =
					props.asset.asset?.name || props.asset.asset_name || "";
			} else {
				isEditing.value = false;
				assetForm.value = {
					asset_id: null,
					site_id: null,
					identifier: "",
					price_euro: 0,
					billing_freq: "Yearly",
					next_billing: "",
					status: "active",
					description: "",
				};
				assetSearchQuery.value = "";
			}
		}
	},
);

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
	assetForm.value.price_euro = template.default_price / 100;
	assetForm.value.billing_freq = template.default_freq || "Yearly";
	assetForm.value.next_billing = new Date().toISOString().split("T")[0] || "";
	assetSearchQuery.value = template.name;
	availableAssetTemplates.value = [];
};

const handleSave = async () => {
	try {
		const payload = {
			...assetForm.value,
			price: Math.round(assetForm.value.price_euro * 100),
		};
		// @ts-ignore
		delete payload.price_euro;

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

				<div class="form-group" v-if="!isEditing">
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
								{{ t.type }} • {{ t.default_price / 100 }}€ /
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
							:value="assetForm.price_euro.toFixed(2)"
							type="number"
							step="0.01"
							placeholder="e.g. 50.00"
							:disabled="!isOrgSelected"
							@input="
								(e) =>
									(assetForm.price_euro =
										parseFloat(
											(e.target as HTMLInputElement)
												.value,
										) || 0)
							"
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
							{{ site.domain || site.name }}
						</option>
					</select>
				</div>

				<div class="form-group">
					<label for="a-identifier">
						Identifier
						<span class="label-info"
							>(Domain, License Key, etc.)</span
						>
					</label>
					<input
						id="a-identifier"
						v-model="assetForm.identifier"
						type="text"
						:disabled="!isOrgSelected"
					/>
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
