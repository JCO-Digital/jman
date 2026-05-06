<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from "vue";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import { validatePasswordStrength } from "../../utils/passwordStrength";
import QRCode from "qrcode";

const authStore = useAuthStore();
const userStore = useUserStore();

// Fetch user profile on mount to get 2FA status
onMounted(() => {
	userStore.fetchProfile();
});

// ─── Section 1: Profile ──────────────────────────────────────────────────────

const displayName = ref(authStore.user?.displayName ?? "");
const profileSaving = ref(false);
const profileSuccess = ref("");
const profileError = ref("");

async function saveProfile() {
	profileSaving.value = true;
	profileSuccess.value = "";
	profileError.value = "";
	try {
		await userStore.updateProfile(displayName.value);
		profileSuccess.value = "Profile updated successfully.";
	} catch (e: any) {
		profileError.value = e.message || "Failed to update profile.";
	} finally {
		profileSaving.value = false;
	}
}

// ─── Section 2: Change Password ─────────────────────────────────────────────

const currentPassword = ref("");
const newPassword = ref("");
const confirmPassword = ref("");
const passwordSaving = ref(false);
const passwordSuccess = ref("");
const passwordError = ref("");

const passwordStrength = computed(() =>
	validatePasswordStrength(newPassword.value),
);

const strengthBarColor = computed(() => {
	const score = passwordStrength.value.score;
	if (score < 33) return "#ef4444"; // red — below minimum
	if (score < 55) return "#f59e0b"; // orange — meets minimum
	return "#10b981"; // green — strong
});

const strengthLabel = computed(() => {
	const score = passwordStrength.value.score;
	if (score === 0) return "";
	if (score < 33) return "Weak";
	if (score < 55) return "Good";
	if (score < 75) return "Strong";
	return "Very strong";
});

const passwordsMatch = computed(() => {
	if (!confirmPassword.value) return true;
	return newPassword.value === confirmPassword.value;
});

const canChangePassword = computed(() => {
	return (
		currentPassword.value &&
		newPassword.value &&
		confirmPassword.value &&
		passwordStrength.value.valid &&
		passwordsMatch.value
	);
});

async function changePassword() {
	if (!canChangePassword.value) return;
	passwordSaving.value = true;
	passwordSuccess.value = "";
	passwordError.value = "";
	try {
		await userStore.changePassword(
			currentPassword.value,
			newPassword.value,
		);
		passwordSuccess.value = "Password changed successfully.";
		currentPassword.value = "";
		newPassword.value = "";
		confirmPassword.value = "";
	} catch (e: any) {
		passwordError.value = e.message || "Failed to change password.";
	} finally {
		passwordSaving.value = false;
	}
}

// ─── Section 3: Two-Factor Authentication ───────────────────────────────────

const tfaSetupData = ref<{ secret: string; uri: string } | null>(null);
const tfaSetupCode = ref("");
const tfaDisableCode = ref("");
const tfaLoading = ref(false);
const tfaError = ref("");
const tfaSuccess = ref("");
const showDisableInput = ref(false);
const qrCanvas = ref<HTMLCanvasElement | null>(null);

async function startSetup2FA() {
	tfaLoading.value = true;
	tfaError.value = "";
	tfaSuccess.value = "";
	try {
		const data = await userStore.setup2FA();
		tfaSetupData.value = data;
		await nextTick();
		if (qrCanvas.value) {
			await QRCode.toCanvas(qrCanvas.value, data.uri, { width: 200 });
		}
	} catch (e: any) {
		tfaError.value = e.message || "Failed to set up 2FA.";
	} finally {
		tfaLoading.value = false;
	}
}

async function activateTFA() {
	if (!tfaSetupData.value || !tfaSetupCode.value) return;
	tfaLoading.value = true;
	tfaError.value = "";
	try {
		await userStore.activate2FA(
			tfaSetupData.value.secret,
			tfaSetupCode.value,
		);
		tfaSuccess.value = "Two-factor authentication has been enabled.";
		tfaSetupData.value = null;
		tfaSetupCode.value = "";
	} catch (e: any) {
		tfaError.value = e.message || "Failed to activate 2FA.";
	} finally {
		tfaLoading.value = false;
	}
}

async function deactivateTFA() {
	if (!tfaDisableCode.value) return;
	tfaLoading.value = true;
	tfaError.value = "";
	try {
		await userStore.deactivate2FA(tfaDisableCode.value);
		tfaSuccess.value = "Two-factor authentication has been disabled.";
		tfaDisableCode.value = "";
		showDisableInput.value = false;
	} catch (e: any) {
		tfaError.value = e.message || "Failed to disable 2FA.";
	} finally {
		tfaLoading.value = false;
	}
}

function cancelSetup() {
	tfaSetupData.value = null;
	tfaSetupCode.value = "";
	tfaError.value = "";
}

function cancelDisable() {
	showDisableInput.value = false;
	tfaDisableCode.value = "";
	tfaError.value = "";
}
</script>

<template>
	<!-- Section 1: Profile -->
	<section class="card">
		<h2>Profile</h2>

		<div class="form-group">
			<label class="form-label">Username</label>
			<span class="username-display">{{ authStore.user?.username }}</span>
		</div>

		<div class="form-group">
			<label for="display-name" class="form-label">Display Name</label>
			<input
				id="display-name"
				v-model="displayName"
				type="text"
				class="form-input"
				placeholder="Enter display name"
			/>
		</div>

		<div v-if="profileSuccess" class="feedback success">
			{{ profileSuccess }}
		</div>
		<div v-if="profileError" class="feedback error">{{ profileError }}</div>

		<button
			class="btn btn-primary"
			:disabled="profileSaving"
			@click="saveProfile"
		>
			{{ profileSaving ? "Saving..." : "Save" }}
		</button>
	</section>

	<!-- Section 2: Change Password -->
	<section class="card">
		<h2>Change Password</h2>

		<div class="form-group">
			<label for="current-password" class="form-label"
				>Current Password</label
			>
			<input
				id="current-password"
				v-model="currentPassword"
				type="password"
				class="form-input"
				placeholder="Enter current password"
				autocomplete="current-password"
			/>
		</div>

		<div class="form-group">
			<label for="new-password" class="form-label">New Password</label>
			<input
				id="new-password"
				v-model="newPassword"
				type="password"
				class="form-input"
				placeholder="Enter new password"
				autocomplete="new-password"
			/>

			<!-- Strength indicator -->
			<div v-if="newPassword" class="strength-section">
				<div class="strength-bar-track">
					<div
						class="strength-bar-fill"
						:style="{
							width: passwordStrength.score + '%',
							backgroundColor: strengthBarColor,
						}"
					></div>
				</div>
				<span
					class="strength-label"
					:style="{ color: strengthBarColor }"
				>
					{{ strengthLabel }}
				</span>

				<ul class="strength-hints">
					<li :class="{ met: passwordStrength.hasLowercase }">
						Lowercase letter
					</li>
					<li :class="{ met: passwordStrength.hasUppercase }">
						Uppercase letter
					</li>
					<li :class="{ met: passwordStrength.hasNumbers }">
						Number
					</li>
					<li :class="{ met: passwordStrength.hasSpecial }">
						Special character
					</li>
					<li :class="{ met: passwordStrength.valid }">
						Meets minimum strength requirement
					</li>
				</ul>
			</div>
		</div>

		<div class="form-group">
			<label for="confirm-password" class="form-label"
				>Confirm New Password</label
			>
			<input
				id="confirm-password"
				v-model="confirmPassword"
				type="password"
				class="form-input"
				placeholder="Confirm new password"
				autocomplete="new-password"
			/>
			<p v-if="!passwordsMatch" class="validation-error">
				Passwords do not match.
			</p>
		</div>

		<div v-if="passwordSuccess" class="feedback success">
			{{ passwordSuccess }}
		</div>
		<div v-if="passwordError" class="feedback error">
			{{ passwordError }}
		</div>

		<button
			class="btn btn-primary"
			:disabled="!canChangePassword || passwordSaving"
			@click="changePassword"
		>
			{{ passwordSaving ? "Changing..." : "Change Password" }}
		</button>
	</section>

	<!-- Section 3: Two-Factor Authentication -->
	<section class="card">
		<h2>Two-Factor Authentication</h2>

		<!-- Loading state -->
		<div v-if="userStore.profileLoading" class="tfa-status-loading">
			Loading 2FA status...
		</div>

		<!-- Error state -->
		<div
			v-else-if="!userStore.profile && userStore.profileError"
			class="feedback error"
		>
			{{ userStore.profileError }}
			<button
				class="btn-text retry-btn"
				@click="userStore.fetchProfile()"
			>
				Retry
			</button>
		</div>

		<!-- Profile loaded -->
		<template v-else-if="userStore.profile">
			<div v-if="tfaSuccess" class="feedback success">
				{{ tfaSuccess }}
			</div>
			<div v-if="tfaError" class="feedback error">{{ tfaError }}</div>

			<!-- 2FA Enabled -->
			<template v-if="userStore.profile.has2FA">
				<p class="tfa-status enabled">
					Two-factor authentication is enabled ✓
				</p>

				<div v-if="!showDisableInput">
					<button
						class="btn btn-danger"
						:disabled="tfaLoading"
						@click="showDisableInput = true"
					>
						Disable 2FA
					</button>
				</div>

				<div v-else class="tfa-action-group">
					<div class="form-group">
						<label for="tfa-disable-code" class="form-label">
							Enter your 6-digit code to confirm
						</label>
						<input
							id="tfa-disable-code"
							v-model="tfaDisableCode"
							type="text"
							class="form-input code-input"
							placeholder="000000"
							maxlength="6"
							autocomplete="one-time-code"
						/>
					</div>
					<div class="btn-group">
						<button
							class="btn btn-danger"
							:disabled="!tfaDisableCode || tfaLoading"
							@click="deactivateTFA"
						>
							{{
								tfaLoading ? "Disabling..." : "Confirm Disable"
							}}
						</button>
						<button
							class="btn btn-secondary"
							@click="cancelDisable"
						>
							Cancel
						</button>
					</div>
				</div>
			</template>

			<!-- 2FA Disabled -->
			<template v-else>
				<p class="tfa-status disabled">
					Two-factor authentication is not enabled.
				</p>

				<!-- Setup not started -->
				<div v-if="!tfaSetupData">
					<button
						class="btn btn-primary"
						:disabled="tfaLoading"
						@click="startSetup2FA"
					>
						{{
							tfaLoading
								? "Setting up..."
								: "Enable Two-Factor Authentication"
						}}
					</button>
				</div>

				<!-- Setup in progress -->
				<div v-else class="tfa-setup">
					<p class="setup-instructions">
						Scan the QR code with your authenticator app, or enter
						the secret key manually.
					</p>

					<div class="qr-container">
						<canvas ref="qrCanvas"></canvas>
					</div>

					<div class="secret-key-group">
						<label class="form-label"
							>Secret Key (manual entry)</label
						>
						<code class="secret-key">{{
							tfaSetupData.secret
						}}</code>
					</div>

					<div class="form-group">
						<label for="tfa-setup-code" class="form-label">
							Enter the 6-digit code from your app
						</label>
						<input
							id="tfa-setup-code"
							v-model="tfaSetupCode"
							type="text"
							class="form-input code-input"
							placeholder="000000"
							maxlength="6"
							autocomplete="one-time-code"
						/>
					</div>

					<div class="btn-group">
						<button
							class="btn btn-primary"
							:disabled="!tfaSetupCode || tfaLoading"
							@click="activateTFA"
						>
							{{
								tfaLoading
									? "Verifying..."
									: "Verify & Activate"
							}}
						</button>
						<button class="btn btn-secondary" @click="cancelSetup">
							Cancel
						</button>
					</div>
				</div>
			</template>
		</template>
	</section>
</template>

<style scoped>
.form-group {
	margin-bottom: 16px;
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-label {
	font-weight: 600;
	font-size: 14px;
	color: var(--text-heading);
}

.form-input {
	padding: 8px 12px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 14px;
	max-width: 400px;

	@media (max-width: 640px) {
		max-width: none;
	}
}

.code-input {
	max-width: 160px;
	font-family: monospace;
	font-size: 18px;
	letter-spacing: 4px;
	text-align: center;
}

.username-display {
	font-size: 15px;
	color: var(--text-main);
	font-weight: 500;
}

/* Feedback messages */
.feedback {
	padding: 10px 14px;
	border-radius: 4px;
	font-size: 14px;
	margin-bottom: 16px;
}

.feedback.success {
	background-color: var(--badge-active-bg);
	color: var(--badge-active-text);
	border: 1px solid var(--badge-active-text);
}

.feedback.error {
	background-color: var(--error-bg);
	color: var(--error-text);
	border: 1px solid var(--error-border);
}

.validation-error {
	color: var(--error-text);
	font-size: 13px;
	margin: 0;
}

/* Password strength */
.strength-section {
	margin-top: 8px;
}

.strength-bar-track {
	height: 6px;
	background-color: var(--bg-body);
	border-radius: 3px;
	overflow: hidden;
	border: 1px solid var(--border-color);
}

.strength-bar-fill {
	height: 100%;
	border-radius: 3px;
	transition:
		width 0.3s ease,
		background-color 0.3s ease;
}

.strength-label {
	font-size: 12px;
	font-weight: 600;
	margin-top: 4px;
	display: inline-block;
}

.strength-hints {
	list-style: none;
	padding: 0;
	margin: 8px 0 0 0;
	font-size: 12px;
	color: var(--text-muted);
}

.strength-hints li {
	padding: 2px 0;
}

.strength-hints li::before {
	content: "✗ ";
	color: var(--error-text);
}

.strength-hints li.met::before {
	content: "✓ ";
	color: #10b981;
}

.strength-hints li.met {
	color: var(--text-main);
}

/* 2FA */
.tfa-status {
	font-size: 15px;
	margin-bottom: 16px;
}

.tfa-status.enabled {
	color: #10b981;
	font-weight: 600;
}

.tfa-status.disabled {
	color: var(--text-muted);
}

.tfa-status-loading {
	color: var(--text-muted);
	font-size: 14px;
	font-style: italic;
}

.retry-btn {
	margin-left: 12px;
}

.tfa-setup {
	margin-top: 16px;
}

.setup-instructions {
	color: var(--text-muted);
	font-size: 14px;
	margin-bottom: 16px;
}

.qr-container {
	margin-bottom: 16px;
	display: flex;
	justify-content: center;
	padding: 16px;
	background: var(--bg-body);
	border-radius: 8px;
	border: 1px solid var(--border-color);
	max-width: 240px;
}

.qr-container canvas {
	display: block;
}

.secret-key-group {
	margin-bottom: 16px;
}

.secret-key {
	display: inline-block;
	margin-top: 4px;
	padding: 8px 12px;
	background: var(--bg-body);
	border: 1px solid var(--border-color);
	border-radius: 4px;
	font-size: 14px;
	word-break: break-all;
	user-select: all;
}

.tfa-action-group {
	margin-top: 8px;
}

/* Buttons */
.btn-group {
	display: flex;
	gap: 12px;
	align-items: center;
	flex-wrap: wrap;
}

.btn-secondary {
	padding: 8px 16px;
	border-radius: 4px;
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	background-color: transparent;
	border: 1px solid var(--border-input);
	color: var(--text-main);
	transition: background-color 0.2s;
}

.btn-secondary:hover {
	background-color: var(--bg-hover);
}

.btn-danger {
	padding: 8px 16px;
	border-radius: 4px;
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	background-color: var(--error-border);
	color: #ffffff;
	border: none;
	transition: background-color 0.2s;
}

.btn-danger:hover {
	opacity: 0.9;
}

.btn-danger:disabled {
	opacity: 0.7;
	cursor: not-allowed;
}
</style>
