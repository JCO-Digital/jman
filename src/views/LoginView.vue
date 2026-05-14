<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import LoadingSpinner from "../components/LoadingSpinner.vue";

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
					<label for="totp">
						TOTP Code
						<span class="optional-label">(optional)</span>
					</label>
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
					<LoadingSpinner
						v-if="isLoading"
						small
						message="Signing in..."
					/>
					<span v-else>Sign In</span>
				</button>
			</form>
		</div>
	</div>
</template>
