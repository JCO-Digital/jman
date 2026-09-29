<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useManagedSitesStore } from "../../stores/managedSites";
import AppIcon from "../AppIcon.vue";
import type { ManagedServer } from "../../types";

const props = defineProps<{
	visible: boolean;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "created", server: ManagedServer): void;
}>();

const managedSitesStore = useManagedSitesStore();

const name = ref("");
const provider = ref("manual");
const isLogical = ref(false);
const ipAddress = ref("");
const sshPort = ref<number | "">(22);
const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);

const canSubmit = computed(
	() => !isSubmitting.value && name.value.trim().length > 0,
);

watch(
	() => props.visible,
	(newVal) => {
		if (!newVal) return;
		name.value = "";
		provider.value = "manual";
		isLogical.value = false;
		ipAddress.value = "";
		sshPort.value = 22;
		errorMessage.value = null;
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
		const created = await managedSitesStore.createServer({
			name: name.value.trim(),
			provider: provider.value.trim() || "manual",
			is_logical: isLogical.value,
			ip_address: isLogical.value ? "" : ipAddress.value.trim(),
			ssh_port: sshPort.value === "" ? 22 : Number(sshPort.value),
		});
		emit("created", created);
		emit("close");
	} catch (e: any) {
		errorMessage.value =
			e.message || "An error occurred while creating the server.";
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
					<h2>Add Server</h2>
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
							<div class="form-row">
								<div class="form-group">
									<label for="server-name">Name</label>
									<input
										id="server-name"
										v-model="name"
										type="text"
										placeholder="e.g. WPEngine or web1.example.com"
										required
										autocomplete="off"
									/>
								</div>
								<div class="form-group">
									<label for="server-provider"
										>Provider</label
									>
									<input
										id="server-provider"
										v-model="provider"
										type="text"
										placeholder="manual, wpengine, hetzner…"
									/>
								</div>
							</div>

							<label class="checkbox-label">
								<input v-model="isLogical" type="checkbox" />
								Logical grouping (managed host without root
								access)
							</label>
							<p class="help-text">
								Logical servers group sites on platforms like WP
								Engine or Kinsta. Connection details are set per
								site.
							</p>

							<div v-if="!isLogical" class="form-row">
								<div class="form-group">
									<label for="server-ip">IP Address</label>
									<input
										id="server-ip"
										v-model="ipAddress"
										type="text"
										placeholder="203.0.113.10"
										autocomplete="off"
									/>
								</div>
								<div class="form-group">
									<label for="server-ssh-port"
										>SSH Port</label
									>
									<input
										id="server-ssh-port"
										v-model.number="sshPort"
										type="number"
										min="1"
										max="65535"
									/>
								</div>
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
									:disabled="!canSubmit"
								>
									{{
										isSubmitting
											? "Creating..."
											: "Add Server"
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

<style scoped>
.checkbox-label {
	display: flex;
	align-items: center;
	gap: 8px;
	cursor: pointer;
}
</style>
