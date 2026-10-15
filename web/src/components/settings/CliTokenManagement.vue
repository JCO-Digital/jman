<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { useAuthStore } from "../../stores/auth";
import { useCliTokensStore } from "../../stores/cliTokens";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import LoadingSpinner from "../LoadingSpinner.vue";
import type { CliToken } from "../../types";

const authStore = useAuthStore();
const cliTokensStore = useCliTokensStore();
const toast = useToastStore();
const { confirm } = useConfirm();

// Admins can switch to every user's tokens.
const showAll = ref(false);

onMounted(() => cliTokensStore.fetchTokens(showAll.value));
watch(showAll, (all) => cliTokensStore.fetchTokens(all));

async function handleRevoke(token: CliToken) {
	const owner =
		token.username === authStore.user?.username
			? ""
			: ` (${token.username})`;
	if (
		!(await confirm(
			`Revoke the CLI token for "${token.device_name}"${owner}? The jman CLI on that device will have to log in again.`,
			{ danger: true, confirmLabel: "Revoke" },
		))
	)
		return;

	try {
		await cliTokensStore.revokeToken(token.id);
		toast.addToast(
			`CLI token for "${token.device_name}" revoked.`,
			"success",
		);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to revoke CLI token", "error");
	}
}

function formatDate(d: string) {
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "short",
		timeStyle: "short",
	});
}
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>CLI Tokens</h2>
			<label v-if="authStore.canAdmin" class="flex-row gap-2">
				<input v-model="showAll" type="checkbox" />
				All users
			</label>
		</div>

		<p class="sub-text mb-4">
			Each device where you ran <code>jman login</code> has its own token.
			A token expires after 90 days without use. Revoke a token to sign
			that device out, for example if the device is lost.
		</p>

		<div v-if="cliTokensStore.isLoading" class="loading-state">
			<LoadingSpinner message="Loading CLI tokens..." />
		</div>

		<div v-else-if="cliTokensStore.error" class="error-banner">
			{{ cliTokensStore.error }}
		</div>

		<div
			v-else-if="cliTokensStore.tokens.length > 0"
			class="table-container mt-4"
		>
			<table class="data-table">
				<thead>
					<tr>
						<th>Device</th>
						<th v-if="showAll">User</th>
						<th>Token</th>
						<th>Created</th>
						<th>Last Used</th>
						<th>Expires</th>
						<th class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="token in cliTokensStore.tokens" :key="token.id">
						<td class="font-medium text-main">
							{{ token.device_name }}
						</td>
						<td v-if="showAll">{{ token.username }}</td>
						<td>
							<code class="secret-key"
								>{{ token.token_prefix }}…</code
							>
						</td>
						<td class="text-muted">
							{{ formatDate(token.created_at) }}
						</td>
						<td class="text-muted">
							{{ formatDate(token.last_used_at) }}
						</td>
						<td class="text-muted">
							{{ formatDate(token.expires_at) }}
						</td>
						<td class="text-right">
							<button
								class="btn btn-text danger"
								@click="handleRevoke(token)"
							>
								Revoke
							</button>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">
				No CLI tokens. Run <code>jman login</code> to create one.
			</p>
		</div>
	</section>
</template>
