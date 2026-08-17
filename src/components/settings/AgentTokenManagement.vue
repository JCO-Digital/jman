<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useAgentTokensStore } from "../../stores/agentTokens";
import { useToastStore } from "../../stores/toast";
import LoadingSpinner from "../LoadingSpinner.vue";
import AgentTokenFormModal from "./AgentTokenFormModal.vue";
import type { AgentToken, CreatedAgentToken } from "../../types";
import { useConfirm } from "../../composables/useConfirm";

const agentTokensStore = useAgentTokensStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const showModal = ref(false);
const newlyCreatedToken = ref<CreatedAgentToken | null>(null);
const copied = ref(false);

onMounted(() => {
	agentTokensStore.fetchTokens();
});

function openCreateModal() {
	showModal.value = true;
}

function closeModal() {
	showModal.value = false;
}

function handleCreated(token: CreatedAgentToken) {
	newlyCreatedToken.value = token;
	copied.value = false;
	toast.addToast("Agent token created successfully", "success");
}

function dismissNewToken() {
	newlyCreatedToken.value = null;
	copied.value = false;
}

async function copyToken() {
	if (!newlyCreatedToken.value) return;
	try {
		await navigator.clipboard.writeText(newlyCreatedToken.value.token);
		copied.value = true;
		setTimeout(() => {
			copied.value = false;
		}, 2000);
	} catch (err) {
		console.error("Failed to copy token: ", err);
	}
}

async function handleRevoke(token: AgentToken) {
	if (
		!(await confirm(
			`Are you sure you want to revoke the agent token for "${token.server_name}"? Any agent using it will immediately be unable to report data.`,
			{ danger: true, confirmLabel: "Revoke" },
		))
	)
		return;

	try {
		await agentTokensStore.revokeToken(token.id);
		toast.addToast(
			`Agent token for "${token.server_name}" revoked.`,
			"success",
		);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to revoke agent token", "error");
	}
}

function formatDate(d: string | null) {
	if (!d) return "—";
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "short",
		timeStyle: "short",
	});
}
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Agent Tokens</h2>
			<button class="btn btn-primary" @click="openCreateModal">
				Create Token
			</button>
		</div>

		<p class="sub-text mb-4">
			Agent tokens authenticate the <code>jman-agent</code> service
			running on each server so it can report disk usage and WordPress
			flags back to jman. Tokens are shown in full only once, at creation
			time.
		</p>

		<!-- One-time plaintext token notice -->
		<div v-if="newlyCreatedToken" class="feedback success">
			<div class="flex-row gap-3 flex-between">
				<div>
					<p class="font-medium mb-1">
						Token created for
						{{ newlyCreatedToken.server_name }}. Copy it now — it
						will not be shown again.
					</p>
					<code class="secret-key">{{
						newlyCreatedToken.token
					}}</code>
				</div>
				<div class="flex-row gap-2">
					<button class="btn btn-outline btn-sm" @click="copyToken">
						{{ copied ? "Copied!" : "Copy" }}
					</button>
					<button
						class="btn btn-text btn-sm"
						@click="dismissNewToken"
					>
						Dismiss
					</button>
				</div>
			</div>
		</div>

		<div v-if="agentTokensStore.isLoading" class="loading-state">
			<LoadingSpinner message="Loading agent tokens..." />
		</div>

		<div
			v-else-if="agentTokensStore.tokens.length > 0"
			class="table-container mt-4"
		>
			<table class="data-table">
				<thead>
					<tr>
						<th>Server</th>
						<th class="hide-mobile">Token</th>
						<th class="hide-mobile">Description</th>
						<th class="hide-mobile">Last Seen</th>
						<th>Status</th>
						<th class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="token in agentTokensStore.tokens"
						:key="token.id"
					>
						<td class="font-medium text-main">
							{{ token.server_name }}
						</td>
						<td class="hide-mobile">
							<code class="secret-key"
								>{{ token.token_prefix }}…</code
							>
						</td>
						<td class="hide-mobile text-muted">
							{{ token.description || "—" }}
						</td>
						<td class="hide-mobile text-muted">
							{{ formatDate(token.last_seen_at) }}
						</td>
						<td>
							<span
								:class="[
									'status-badge',
									'badge-sm',
									token.revoked ? 'inactive' : 'active',
								]"
							>
								{{ token.revoked ? "Revoked" : "Active" }}
							</span>
						</td>
						<td class="text-right">
							<button
								class="btn btn-text danger"
								:disabled="token.revoked"
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
			<p class="text-muted">No agent tokens created yet.</p>
		</div>
	</section>

	<AgentTokenFormModal
		:visible="showModal"
		@close="closeModal"
		@created="handleCreated"
	/>
</template>
