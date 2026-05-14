<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useUserStore } from "../../stores/user";
import { validatePasswordStrength } from "../../utils/passwordStrength";
import type { CreateUserPayload, UpdateUserPayload } from "../../types";

interface Props {
	visible: boolean;
	editUser?: { username: string; displayName: string; level: string } | null;
}

const props = withDefaults(defineProps<Props>(), {
	editUser: null,
});

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved"): void;
}>();

const userStore = useUserStore();

const username = ref("");
const displayName = ref("");
const password = ref("");
const level = ref("basic");
const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);

const isEditMode = computed(() => !!props.editUser);
const modalTitle = computed(() =>
	isEditMode.value ? "Edit User" : "Create User",
);
const submitLabel = computed(() =>
	isEditMode.value ? "Save Changes" : "Create User",
);

const passwordStrength = computed(() => {
	if (!password.value) {
		return {
			valid: false,
			score: 0,
			poolSize: 0,
			hasLowercase: false,
			hasUppercase: false,
			hasNumbers: false,
			hasSpecial: false,
		};
	}
	return validatePasswordStrength(password.value);
});

const strengthBarColor = computed(() => {
	const score = passwordStrength.value.score;
	if (score < 33) return "var(--error-border)";
	if (score < 55) return "var(--warning-border)";
	return "var(--badge-active-text)";
});

const canSubmit = computed(() => {
	if (isSubmitting.value) return false;
	if (isEditMode.value) {
		// In edit mode: display name required, and if password is entered it must be valid
		if (displayName.value.trim().length === 0) return false;
		if (password.value && !passwordStrength.value.valid) return false;
		return true;
	}
	return (
		username.value.trim().length > 0 &&
		displayName.value.trim().length > 0 &&
		passwordStrength.value.valid
	);
});

// Reset form when modal opens / editUser changes
watch(
	() => props.visible,
	(newVal) => {
		if (newVal) {
			errorMessage.value = null;
			if (props.editUser) {
				username.value = props.editUser.username;
				displayName.value = props.editUser.displayName;
				level.value = props.editUser.level;
				password.value = "";
			} else {
				username.value = "";
				displayName.value = "";
				password.value = "";
				level.value = "basic";
			}
		}
	},
);

function handleOverlayClick(event: MouseEvent) {
	if (event.target === event.currentTarget) {
		emit("close");
	}
}

async function handleSubmit() {
	if (!canSubmit.value) return;

	isSubmitting.value = true;
	errorMessage.value = null;

	try {
		if (isEditMode.value && props.editUser) {
			const payload: UpdateUserPayload = {
				displayName: displayName.value.trim(),
				level: level.value as "basic" | "edit" | "execute" | "admin",
			};
			if (password.value) {
				payload.password = password.value;
			}
			await userStore.updateUser(props.editUser.username, payload);
		} else {
			const payload: CreateUserPayload = {
				username: username.value.trim(),
				password: password.value,
				displayName: displayName.value.trim(),
				level: level.value as "basic" | "edit" | "execute" | "admin",
			};
			await userStore.createUser(payload);
		}
		emit("saved");
		emit("close");
	} catch (e: any) {
		errorMessage.value =
			e.message || "An error occurred while saving the user.";
	} finally {
		isSubmitting.value = false;
	}
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click="handleOverlayClick">
			<div class="modal-content card">
				<header class="modal-header">
					<h2>{{ modalTitle }}</h2>
					<button class="modal-close" @click="emit('close')">
						&times;
					</button>
				</header>

				<div class="content">
					<div v-if="errorMessage" class="error-banner">
						<p>{{ errorMessage }}</p>
					</div>

					<form @submit.prevent="handleSubmit">
						<div class="content">
							<!-- Username -->
							<div class="form-group">
								<label for="user-username">Username</label>
								<input
									v-if="!isEditMode"
									id="user-username"
									v-model="username"
									type="text"
									placeholder="Enter username"
									required
									autocomplete="off"
								/>
								<span v-else class="readonly-value">{{
									username
								}}</span>
							</div>

							<!-- Display Name -->
							<div class="form-group">
								<label for="user-displayname"
									>Display Name</label
								>
								<input
									id="user-displayname"
									v-model="displayName"
									type="text"
									placeholder="Enter display name"
									required
								/>
							</div>

							<!-- Password (create mode) -->
							<div v-if="!isEditMode" class="form-group">
								<label for="user-password">Password</label>
								<input
									id="user-password"
									v-model="password"
									type="password"
									placeholder="Enter password"
									required
									autocomplete="new-password"
								/>

								<!-- Password strength indicator -->
								<div
									v-if="password.length > 0"
									class="strength-indicator"
								>
									<div class="strength-bar-track">
										<div
											class="strength-bar-fill"
											:style="{
												width:
													passwordStrength.score +
													'%',
												backgroundColor:
													strengthBarColor,
											}"
										></div>
									</div>
									<div class="strength-classes">
										<span
											:class="{
												active: passwordStrength.hasLowercase,
											}"
											>a-z</span
										>
										<span
											:class="{
												active: passwordStrength.hasUppercase,
											}"
											>A-Z</span
										>
										<span
											:class="{
												active: passwordStrength.hasNumbers,
											}"
											>0-9</span
										>
										<span
											:class="{
												active: passwordStrength.hasSpecial,
											}"
											>!@#</span
										>
									</div>
								</div>
							</div>

							<!-- Password (edit mode - optional reset) -->
							<div v-if="isEditMode" class="form-group">
								<label for="user-password-edit"
									>Reset Password</label
								>
								<input
									id="user-password-edit"
									v-model="password"
									type="password"
									placeholder="Leave blank to keep current"
									autocomplete="new-password"
								/>
								<p class="help-text">
									Only fill this in if you want to change the
									user's password.
								</p>

								<!-- Password strength indicator -->
								<div
									v-if="password.length > 0"
									class="strength-indicator"
								>
									<div class="strength-bar-track">
										<div
											class="strength-bar-fill"
											:style="{
												width:
													passwordStrength.score +
													'%',
												backgroundColor:
													strengthBarColor,
											}"
										></div>
									</div>
									<div class="strength-classes">
										<span
											:class="{
												active: passwordStrength.hasLowercase,
											}"
											>a-z</span
										>
										<span
											:class="{
												active: passwordStrength.hasUppercase,
											}"
											>A-Z</span
										>
										<span
											:class="{
												active: passwordStrength.hasNumbers,
											}"
											>0-9</span
										>
										<span
											:class="{
												active: passwordStrength.hasSpecial,
											}"
											>!@#</span
										>
									</div>
								</div>
							</div>

							<!-- Level -->
							<div class="form-group">
								<label for="user-level">Permission Level</label>
								<select id="user-level" v-model="level">
									<option value="basic">Basic</option>
									<option value="edit">Edit</option>
									<option value="execute">Execute</option>
									<option value="admin">Admin</option>
								</select>
							</div>

							<div class="form-actions mt-4">
								<button
									type="button"
									class="btn btn-outline"
									@click="emit('close')"
								>
									Cancel
								</button>
								<button
									type="submit"
									class="btn btn-primary"
									:disabled="!canSubmit"
								>
									{{
										isSubmitting ? "Saving..." : submitLabel
									}}
								</button>
							</div>
						</div>
					</form>
				</div>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
/* Scoped styles removed in favor of global modal, form, and utility classes */
</style>
