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
const submitLabel = computed(() => (isEditMode.value ? "Save" : "Create"));

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
	if (score < 33) return "#ef4444";
	if (score < 55) return "#f59e0b";
	return "#10b981";
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
				level: level.value as "basic" | "edit" | "execute",
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
				level: level.value as "basic" | "edit" | "execute",
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
			<div class="modal-card">
				<header class="modal-header">
					<h3>{{ modalTitle }}</h3>
				</header>

				<div class="modal-body">
					<div v-if="errorMessage" class="error-banner">
						<p>{{ errorMessage }}</p>
					</div>

					<form class="modal-form" @submit.prevent="handleSubmit">
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
							<label for="user-displayname">Display Name</label>
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
											width: passwordStrength.score + '%',
											backgroundColor: strengthBarColor,
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
											width: passwordStrength.score + '%',
											backgroundColor: strengthBarColor,
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
							<label for="user-level">Level</label>
							<select id="user-level" v-model="level">
								<option value="basic">Basic</option>
								<option value="edit">Edit</option>
								<option value="execute">Execute</option>
							</select>
						</div>
					</form>
				</div>

				<footer class="modal-footer">
					<button
						type="button"
						class="btn btn-cancel"
						@click="emit('close')"
					>
						Cancel
					</button>
					<button
						type="button"
						class="btn btn-primary"
						:disabled="!canSubmit"
						@click="handleSubmit"
					>
						{{ isSubmitting ? "Saving..." : submitLabel }}
					</button>
				</footer>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.modal-overlay {
	position: fixed;
	top: 0;
	left: 0;
	right: 0;
	bottom: 0;
	background: rgba(0, 0, 0, 0.5);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 1000;
	padding: 16px;
}

.modal-card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 8px;
	width: 100%;
	max-width: 480px;
	max-height: 90vh;
	overflow-y: auto;
	display: flex;
	flex-direction: column;
}

.modal-header {
	padding: 20px 24px 16px;
	border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
	margin: 0;
	font-size: 18px;
	color: var(--text-heading);
}

.modal-body {
	padding: 20px 24px;
	flex: 1;
}

.modal-form {
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
	font-size: 14px;
	font-weight: 600;
	color: var(--text-heading);
}

.help-text {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}

.form-group input,
.form-group select {
	padding: 8px 12px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 14px;
	background: var(--bg-card);
	color: var(--text-main);
}

.form-group input:focus,
.form-group select:focus {
	outline: none;
	border-color: var(--primary);
	box-shadow: 0 0 0 2px rgba(79, 70, 229, 0.15);
}

.readonly-value {
	padding: 8px 12px;
	border: 1px solid var(--border-color);
	border-radius: 4px;
	font-size: 14px;
	color: var(--text-muted);
	background: var(--bg-body);
}

/* Password strength indicator */
.strength-indicator {
	margin-top: 4px;
}

.strength-bar-track {
	width: 100%;
	height: 4px;
	background: var(--border-color);
	border-radius: 2px;
	overflow: hidden;
}

.strength-bar-fill {
	height: 100%;
	border-radius: 2px;
	transition:
		width 0.3s ease,
		background-color 0.3s ease;
}

.strength-classes {
	display: flex;
	gap: 8px;
	margin-top: 6px;
	font-size: 12px;
}

.strength-classes span {
	color: var(--text-muted);
	padding: 1px 6px;
	border-radius: 3px;
	background: var(--bg-body);
	border: 1px solid var(--border-color);
	transition: all 0.2s;
}

.strength-classes span.active {
	color: var(--badge-active-text);
	background: var(--badge-active-bg);
	border-color: var(--badge-active-text);
}

/* Error banner */
.error-banner {
	background-color: var(--error-bg);
	border-left: 4px solid var(--error-border);
	color: var(--error-text);
	padding: 10px 14px;
	margin-bottom: 16px;
	border-radius: 4px;
}

.error-banner p {
	margin: 0;
	font-size: 14px;
}

/* Footer */
.modal-footer {
	padding: 16px 24px 20px;
	border-top: 1px solid var(--border-color);
	display: flex;
	justify-content: flex-end;
	gap: 12px;
}

.btn-cancel {
	padding: 8px 16px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	background: var(--bg-card);
	color: var(--text-main);
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	transition: background-color 0.2s;
}

.btn-cancel:hover {
	background-color: var(--bg-hover);
}
</style>
