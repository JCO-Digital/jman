<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useAgentTokensStore } from "../../stores/agentTokens";
import { useDataStore } from "../../stores/data";
import AppIcon from "../AppIcon.vue";
import type { CreatedAgentToken } from "../../types";
import { providerLabel } from "../../utils/format";

const props = defineProps<{
	visible: boolean;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "created", token: CreatedAgentToken): void;
}>();

const agentTokensStore = useAgentTokensStore();
const dataStore = useDataStore();

// Server ids are UUIDs (the same ones used everywhere else in jman, e.g.
// site.server_id). Any real server from /api/servers can run an agent, but
// logical servers have no host to run it on, so they're excluded.
const selectedServerId = ref("");

const agentCapableServers = computed(() =>
	dataStore.servers
		.filter((s) => !s.is_logical)
		.sort((a, b) => a.name.localeCompare(b.name)),
);
const description = ref("");
const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);

watch(
	() => props.visible,
	(newVal) => {
		if (newVal) {
			selectedServerId.value = "";
			description.value = "";
			errorMessage.value = null;
			if (!dataStore.isLoaded) dataStore.initData();
		}
	},
);

function handleOverlayClick(event: MouseEvent) {
	if (event.target === event.currentTarget) {
		emit("close");
	}
}

async function handleSubmit() {
	const server = agentCapableServers.value.find(
		(s) => s.id === selectedServerId.value,
	);
	if (!server) return;

	isSubmitting.value = true;
	errorMessage.value = null;
	try {
		const created = await agentTokensStore.createToken(
			server.id,
			server.name,
			description.value.trim() || undefined,
		);
		emit("created", created);
		emit("close");
	} catch (e: any) {
		errorMessage.value =
			e.message || "An error occurred while creating the token.";
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
					<h2>Create Agent Token</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div class="content">
					<div v-if="errorMessage" class="error-banner">
						<p>{{ errorMessage }}</p>
					</div>

					<form @submit.prevent="handleSubmit">
						<div class="content">
							<div class="form-group">
								<label for="agent-token-server">Server</label>
								<select
									id="agent-token-server"
									v-model="selectedServerId"
									required
								>
									<option value="" disabled>
										Select a server
									</option>
									<option
										v-for="server in agentCapableServers"
										:key="server.id"
										:value="server.id"
									>
										{{ server.name }} ({{
											providerLabel(server.provider)
										}})
									</option>
								</select>
							</div>

							<div class="form-group">
								<label for="agent-token-description"
									>Description (optional)</label
								>
								<input
									id="agent-token-description"
									v-model="description"
									type="text"
									placeholder="e.g. Primary web server"
								/>
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
									:disabled="
										isSubmitting || !selectedServerId
									"
								>
									{{
										isSubmitting
											? "Creating..."
											: "Create Token"
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
