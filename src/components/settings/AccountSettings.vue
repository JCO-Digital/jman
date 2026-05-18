<script setup lang="ts">
import { ref, computed, nextTick, onMounted } from "vue";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import { validatePasswordStrength } from "../../utils/passwordStrength";
import QRCode from "qrcode";

const authStore = useAuthStore();
const userStore = useUserStore();

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

// Fetch user profile on mount to get 2FA status
onMounted(() => {
	userStore.fetchProfile();
	fetchSlackId();
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

// ─── Section: Slack Integration ──────────────────────────────────────────────

const slackId = ref("");
const slackSaving = ref(false);
const slackSuccess = ref("");
const slackError = ref("");

async function fetchSlackId() {
	if (!authStore.isAuthenticated) return;
	try {
		const res = await fetch(`${BASE_URL}/settings/slack_id`, {
			headers: authStore.authHeader,
		});
		if (res.ok) {
			const data = await res.json();
			// The API returns either a raw string or { value: "..." }
			slackId.value =
				typeof data === "object" && data !== null ? data.value : data;
		}
	} catch (e) {
		console.error("Failed to fetch Slack ID", e);
	}
}

function validateSlackId(id: string) {
	return /^[UW][A-Z0-9]{8}$/.test(id);
}

async function saveSlackId() {
	if (slackId.value && !validateSlackId(slackId.value)) {
		slackError.value =
			"Invalid Slack Member ID format. It should start with U or W followed by 8 alphanumeric characters.";
		return;
	}

	slackSaving.value = true;
	slackSuccess.value = "";
	slackError.value = "";
	try {
		const res = await fetch(`${BASE_URL}/settings/slack_id`, {
			method: "POST",
			headers: {
				...authStore.authHeader,
				"Content-Type": "application/json",
			},
			body: JSON.stringify(slackId.value),
		});
		if (!res.ok) throw new Error("Failed to save Slack ID");
		slackSuccess.value = "Slack ID updated successfully.";
	} catch (e: any) {
		slackError.value = e.message || "Failed to update Slack ID.";
	} finally {
		slackSaving.value = false;
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
	if (score < 33) return "var(--error-border)"; // red — below minimum
	if (score < 55) return "var(--warning-border)"; // orange — meets minimum
	return "var(--badge-active-text)"; // green — strong
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
	<div class="content mt-4">
		<!-- Section 1: Profile -->
		<section class="card">
			<h2>Profile</h2>

			<div class="form-group">
				<label>Username</label>
				<span class="font-medium text-main">{{
					authStore.user?.username
				}}</span>
			</div>

			<div class="form-group max-w-320">
				<label for="display-name">Display Name</label>
				<input
					id="display-name"
					v-model="displayName"
					type="text"
					placeholder="Enter display name"
				/>
			</div>

			<div v-if="profileSuccess" class="feedback success">
				{{ profileSuccess }}
			</div>
			<div v-if="profileError" class="feedback error">
				{{ profileError }}
			</div>

			<button
				class="btn btn-primary"
				:disabled="profileSaving"
				@click="saveProfile"
			>
				{{ profileSaving ? "Saving..." : "Save Profile" }}
			</button>
		</section>

		<!-- Section: Slack Integration -->
		<section class="card">
			<h2>Slack Integration</h2>
			<p class="sub-text mb-4">
				Connect your Slack account by providing your Member ID. This
				allows you to receive notifications and assignments directly in
				Slack.
			</p>

			<div class="form-group max-w-320">
				<label for="slack-id">Slack Member ID</label>
				<input
					id="slack-id"
					v-model="slackId"
					type="text"
					placeholder="e.g. U0G9QF9C6"
					maxlength="9"
					@input="slackId = slackId.toUpperCase()"
				/>
				<p class="help-text">
					Format: U or W followed by 8 characters.
				</p>
			</div>

			<div v-if="slackSuccess" class="feedback success">
				{{ slackSuccess }}
			</div>
			<div v-if="slackError" class="feedback error">
				{{ slackError }}
			</div>

			<button
				class="btn btn-primary"
				:disabled="slackSaving"
				@click="saveSlackId"
			>
				{{ slackSaving ? "Saving..." : "Save Slack ID" }}
			</button>

			<div class="section-divider mt-6"></div>

			<div class="mt-4">
				<p class="font-medium">How to Find a Member ID</p>
				<ol class="font-sm text-muted ml-4 mt-2">
					<li>Click on the user's name in Slack</li>
					<li>Select <strong>View full profile</strong></li>
					<li>Click the overflow menu (⋮) under their avatar</li>
					<li>Choose <strong>Copy member ID</strong></li>
				</ol>
			</div>
		</section>

		<!-- Section 2: Change Password -->
		<section class="card">
			<h2>Change Password</h2>

			<div class="form-group max-w-320">
				<label for="current-password">Current Password</label>
				<input
					id="current-password"
					v-model="currentPassword"
					type="password"
					placeholder="Enter current password"
					autocomplete="current-password"
				/>
			</div>

			<div class="form-group max-w-320">
				<label for="new-password">New Password</label>
				<input
					id="new-password"
					v-model="newPassword"
					type="password"
					placeholder="Enter new password"
					autocomplete="new-password"
				/>

				<!-- Strength indicator -->
				<div v-if="newPassword" class="strength-indicator">
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

			<div class="form-group max-w-320">
				<label for="confirm-password">Confirm New Password</label>
				<input
					id="confirm-password"
					v-model="confirmPassword"
					type="password"
					placeholder="Confirm new password"
					autocomplete="new-password"
				/>
				<p v-if="!passwordsMatch" class="text-error font-xs mt-1">
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
			<div v-if="userStore.profileLoading" class="loading-state">
				<p>Loading 2FA status...</p>
			</div>

			<!-- Error state -->
			<div
				v-else-if="!userStore.profile && userStore.profileError"
				class="feedback error"
			>
				<div class="flex-row gap-3">
					<span>{{ userStore.profileError }}</span>
					<button
						class="btn btn-text font-sm"
						@click="userStore.fetchProfile()"
					>
						Retry
					</button>
				</div>
			</div>

			<!-- Profile loaded -->
			<template v-else-if="userStore.profile">
				<div v-if="tfaSuccess" class="feedback success">
					{{ tfaSuccess }}
				</div>
				<div v-if="tfaError" class="feedback error">{{ tfaError }}</div>

				<!-- 2FA Enabled -->
				<template v-if="userStore.profile.has2FA">
					<p class="text-success font-medium mb-4">
						Two-factor authentication is enabled ✓
					</p>

					<div v-if="!showDisableInput">
						<button
							class="btn btn-outline danger"
							:disabled="tfaLoading"
							@click="showDisableInput = true"
						>
							Disable 2FA
						</button>
					</div>

					<div v-else class="content">
						<div class="form-group max-w-320">
							<label for="tfa-disable-code">
								Enter your 6-digit code to confirm
							</label>
							<input
								id="tfa-disable-code"
								v-model="tfaDisableCode"
								type="text"
								placeholder="000000"
								maxlength="6"
								style="
									text-align: center;
									letter-spacing: 4px;
									font-family: monospace;
								"
								autocomplete="one-time-code"
							/>
						</div>
						<div class="flex-row gap-3">
							<button
								class="btn btn-danger"
								:disabled="!tfaDisableCode || tfaLoading"
								@click="deactivateTFA"
							>
								{{
									tfaLoading
										? "Disabling..."
										: "Confirm Disable"
								}}
							</button>
							<button
								class="btn btn-outline"
								@click="cancelDisable"
							>
								Cancel
							</button>
						</div>
					</div>
				</template>

				<!-- 2FA Disabled -->
				<template v-else>
					<p class="text-muted mb-4">
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
					<div v-else class="content">
						<p class="sub-text">
							Scan the QR code with your authenticator app, or
							enter the secret key manually.
						</p>

						<div class="qr-container">
							<canvas ref="qrCanvas"></canvas>
						</div>

						<div class="form-group">
							<label>Secret Key (manual entry)</label>
							<code class="secret-key">{{
								tfaSetupData.secret
							}}</code>
						</div>

						<div class="form-group max-w-320">
							<label for="tfa-setup-code">
								Enter the 6-digit code from your app
							</label>
							<input
								id="tfa-setup-code"
								v-model="tfaSetupCode"
								type="text"
								placeholder="000000"
								maxlength="6"
								style="
									text-align: center;
									letter-spacing: 4px;
									font-family: monospace;
								"
								autocomplete="one-time-code"
							/>
						</div>

						<div class="flex-row gap-3">
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
							<button
								class="btn btn-outline"
								@click="cancelSetup"
							>
								Cancel
							</button>
						</div>
					</div>
				</template>
			</template>
		</section>
	</div>
</template>

<style scoped>
/* Scoped styles removed in favor of global utility classes and component styles */
</style>
