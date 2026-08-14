<script setup lang="ts">
import { ref, watch } from "vue";
import type { Contact, ContactType } from "../types";
import { useOrganizationStore } from "../stores/organization";
import { useToastStore } from "../stores/toast";

const props = defineProps<{
	modelValue: boolean;
	contact: Contact | null;
	organizationId: number;
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: boolean): void;
	(e: "saved"): void;
}>();

const organizationStore = useOrganizationStore();
const toast = useToastStore();

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

watch(
	() => props.modelValue,
	(newVal) => {
		if (newVal) {
			if (props.contact) {
				contactForm.value = {
					name: props.contact.name,
					email: props.contact.email || "",
					phone: props.contact.phone || "",
					type: props.contact.type,
				};
			} else {
				contactForm.value = {
					name: "",
					email: "",
					phone: "",
					type: "Main",
				};
			}
		}
	},
);

const handleContactSubmit = async () => {
	try {
		if (props.contact) {
			await organizationStore.updateContact(
				props.contact.id,
				contactForm.value,
			);
		} else {
			await organizationStore.createContact({
				...contactForm.value,
				organization_id: props.organizationId,
			});
		}
		emit("saved");
		emit("update:modelValue", false);
	} catch (e: any) {
		toast.addToast("Failed to save contact: " + e.message, "error");
	}
};

const close = () => {
	emit("update:modelValue", false);
};
</script>

<template>
	<div v-if="modelValue" class="modal-overlay" @click.self="close">
		<div class="modal-content card">
			<h2>{{ contact ? "Edit Contact" : "Add Contact" }}</h2>
			<form class="form-layout" @submit.prevent="handleContactSubmit">
				<div class="form-group">
					<label for="c-name">Name</label>
					<input
						id="c-name"
						v-model="contactForm.name"
						type="text"
						required
						placeholder="Full Name"
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
					<label for="c-email">Email</label>
					<input
						id="c-email"
						v-model="contactForm.email"
						type="email"
						placeholder="email@example.com"
					/>
				</div>
				<div class="form-group">
					<label for="c-phone">Phone</label>
					<input
						id="c-phone"
						v-model="contactForm.phone"
						type="tel"
						placeholder="+49 123 456789"
					/>
				</div>
				<div class="form-actions">
					<button
						type="button"
						class="btn btn-outline"
						@click="close"
					>
						Cancel
					</button>
					<button type="submit" class="btn btn-primary">
						{{ contact ? "Update Contact" : "Add Contact" }}
					</button>
				</div>
			</form>
		</div>
	</div>
</template>
