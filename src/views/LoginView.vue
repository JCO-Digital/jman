<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";

const router = useRouter();
const authStore = useAuthStore();

const username = ref("");
const password = ref("");
const totp = ref("");
const error = ref<string | null>(null);
const isLoading = ref(false);

const handleLogin = async () => {
	isLoading.value = true;
	error.value = null;

	try {
		await authStore.login(
			username.value,
			password.value,
			totp.value || undefined,
		);
		router.push("/");
	} catch (err: unknown) {
		let message: string;
		if (err instanceof Error && err.message) {
			message = err.message;
		} else if (typeof err === "string") {
			message = err;
		} else {
			message = String(err);
		}
		error.value = message;
	} finally {
		isLoading.value = false;
	}
};
</script>

<template>
	<div class="login-page">
		<div class="login-card">
			<h1 class="login-title">jman</h1>

			<div v-if="error" class="error-banner">
				<p>{{ error }}</p>
			</div>

			<form @submit.prevent="handleLogin">
				<div class="form-group">
					<label for="username">Username</label>
					<input
						id="username"
						v-model="username"
						type="text"
						placeholder="Enter your username"
						autocomplete="username"
						required
					/>
				</div>

				<div class="form-group">
					<label for="password">Password</label>
					<input
						id="password"
						v-model="password"
						type="password"
						placeholder="Enter your password"
						autocomplete="current-password"
						required
					/>
				</div>

				<div class="form-group">
					<label for="totp"
						>TOTP Code <span class="optional-label">(optional)</span></label
					>
					<input
						id="totp"
						v-model="totp"
						type="text"
						placeholder="Enter 6-digit code"
						maxlength="6"
						inputmode="numeric"
						autocomplete="one-time-code"
					/>
				</div>

				<button type="submit" class="login-btn" :disabled="isLoading">
					<span
						v-if="isLoading"
						class="spinner spinner-small"
						style="margin-right: 8px; vertical-align: middle"
					></span>
					<span style="vertical-align: middle">{{
						isLoading ? "Signing in..." : "Sign In"
					}}</span>
				</button>
			</form>
		</div>
	</div>
</template>

<style scoped>
.login-page {
	min-height: 100vh;
	display: flex;
	align-items: center;
	justify-content: center;
	background: var(--bg-body);
	padding: 20px;
}

.login-card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 8px;
	box-shadow: 0 4px 24px rgba(0, 0, 0, 0.08);
	padding: 32px;
	max-width: 400px;
	width: 100%;
}

.login-title {
	text-align: center;
	color: var(--text-heading);
	font-size: 28px;
	margin: 0 0 24px 0;
}

.error-banner {
	background-color: var(--error-bg);
	border-left: 4px solid var(--error-border);
	color: var(--error-text);
	padding: 12px 16px;
	margin-bottom: 20px;
	border-radius: 4px;
}

.error-banner p {
	margin: 0;
}

.form-group {
	margin-bottom: 20px;
}

.form-group label {
	display: block;
	font-size: 14px;
	font-weight: 500;
	color: var(--text-main);
	margin-bottom: 6px;
}

.form-group input {
	width: 100%;
	padding: 10px 12px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 14px;
	background-color: var(--bg-card);
	color: var(--text-main);
	transition:
		border-color 0.2s,
		box-shadow 0.2s;
}

.form-group input::placeholder {
	color: var(--text-placeholder);
}

.form-group input:focus {
	outline: none;
	border-color: var(--primary);
	box-shadow: 0 0 0 3px rgba(79, 70, 229, 0.15);
}

.optional-label {
	font-weight: 400;
	color: var(--text-muted);
	font-size: 12px;
}

.login-btn {
	width: 100%;
	padding: 12px;
	background-color: var(--primary);
	color: var(--primary-text);
	border: none;
	border-radius: 4px;
	font-size: 16px;
	font-weight: 600;
	cursor: pointer;
	transition: background-color 0.2s;
}

.login-btn:hover:not(:disabled) {
	background-color: var(--primary-hover);
}

.login-btn:disabled {
	opacity: 0.7;
	cursor: not-allowed;
}
</style>
