<script setup lang="ts">
import { ref, watch } from "vue";
import { useAgentTokensStore } from "../../stores/agentTokens";
import AppIcon from "../AppIcon.vue";
import type { CreatedAgentToken } from "../../types";

const props = defineProps<{
	visible: boolean;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "created", token: CreatedAgentToken): void;
}>();

const agentTokensStore = useAgentTokensStore();

const serverId = ref("");
const serverName = ref("");
const description = ref("");
const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);

watch(
	() => props.visible,
	(newVal) => {
		if (newVal) {
			serverId.value = "";
			serverName.value = "";
			description.value = "";
			errorMessage.value = null;
		}
	},
);

function handleOverlayClick(event: MouseEvent) {
	if (event.target === event.currentTarget) {
		emit("close");
	}
}

async function handleSubmit() {
	const parsedServerId = parseInt(serverId.value, 10);
	if (isNaN(parsedServerId) || !serverName.value.trim()) return;

	isSubmitting.value = true;
	errorMessage.value = null;
	try {
		const created = await agentTokensStore.createToken(
			parsedServerId,
			serverName.value.trim(),
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
								<label for="agent-token-server-id"
									>Server ID</label
								>
								<input
									id="agent-token-server-id"
									v-model="serverId"
									type="number"
									placeholder="Enter server ID"
									required
								/>
							</div>

							<div class="form-group">
								<label for="agent-token-server-name"
									>Server Name</label
								>
								<input
									id="agent-token-server-name"
									v-model="serverName"
									type="text"
									placeholder="Enter server name"
									required
								/>
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
										isSubmitting ||
										!serverId ||
										!serverName.trim()
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
