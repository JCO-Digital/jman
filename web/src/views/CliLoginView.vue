<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useCliTokensStore } from "../stores/cliTokens";

// `jman login` opens this page with the port of a listener on 127.0.0.1, a
// state value, a PKCE challenge and the device name. Approving sends the
// browser back to that listener with a one-time code, which only the CLI
// that holds the PKCE verifier can exchange for a token.

const route = useRoute();
const authStore = useAuthStore();
const cliTokensStore = useCliTokensStore();

const query = (key: string) => {
	const v = route.query[key];
	return typeof v === "string" ? v : "";
};

const port = computed(() => {
	const p = Number(query("port"));
	return Number.isInteger(p) && p >= 1024 && p <= 65535 ? p : null;
});
const state = computed(() => query("state"));
const challenge = computed(() => query("challenge"));
const deviceName = computed(() => query("device").trim() || "Unknown device");
const isValid = computed(
	() => port.value !== null && state.value !== "" && challenge.value !== "",
);

const isSubmitting = ref(false);
const error = ref<string | null>(null);
const done = ref<"approved" | "denied" | null>(null);

function sendToCli(params: Record<string, string>) {
	const url = new URL(`http://127.0.0.1:${port.value}/callback`);
	for (const [k, v] of Object.entries({ ...params, state: state.value })) {
		url.searchParams.set(k, v);
	}
	window.location.href = url.toString();
}

async function approve() {
	isSubmitting.value = true;
	error.value = null;
	try {
		const code = await cliTokensStore.authorize(
			challenge.value,
			deviceName.value,
		);
		done.value = "approved";
		sendToCli({ code });
	} catch (e: any) {
		error.value = e.message || "Failed to authorize the CLI";
	} finally {
		isSubmitting.value = false;
	}
}

function deny() {
	done.value = "denied";
	sendToCli({ error: "access_denied" });
}
</script>

<template>
	<div class="view-container">
		<section class="card cli-login-card">
			<h2>Authorize jman CLI</h2>

			<div v-if="!isValid" class="error-banner">
				This login link is incomplete. Run
				<code>jman login</code> again.
			</div>

			<template v-else-if="done === 'approved'">
				<p>
					Authorized. You can close this tab and return to your
					terminal.
				</p>
			</template>

			<template v-else-if="done === 'denied'">
				<p>Login denied. You can close this tab.</p>
			</template>

			<template v-else>
				<p>
					The jman CLI on
					<strong>{{ deviceName }}</strong> is asking to sign in as
					<strong>{{
						authStore.user?.displayName || authStore.user?.username
					}}</strong
					>.
				</p>
				<p class="sub-text text-muted">
					It gets your access level, and keeps it until you revoke it
					under Settings → CLI Tokens or it goes unused for 90 days.
					Only approve if you just ran <code>jman login</code>.
				</p>

				<div v-if="error" class="error-banner">{{ error }}</div>

				<div class="flex-row gap-2 mt-4">
					<button
						class="btn btn-primary"
						:disabled="isSubmitting"
						@click="approve"
					>
						Authorize
					</button>
					<button
						class="btn btn-secondary"
						:disabled="isSubmitting"
						@click="deny"
					>
						Deny
					</button>
				</div>
			</template>
		</section>
	</div>
</template>

<style scoped>
.cli-login-card {
	max-width: 520px;
	margin: 48px auto 0;
}
</style>
