<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useManagedSitesStore } from "../../stores/managedSites";
import AppIcon from "../AppIcon.vue";
import type {
	ManagedConnectionType,
	ManagedSite,
	ManagedSitePayload,
	ManagedSiteStatus,
	SiteEnvironment,
} from "../../types";

interface Props {
	visible: boolean;
	editSite?: ManagedSite | null;
}

const props = withDefaults(defineProps<Props>(), {
	editSite: null,
});

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved", site: ManagedSite): void;
}>();

const managedSitesStore = useManagedSitesStore();

const domain = ref("");
const serverId = ref("");
const provider = ref("manual");
const environment = ref<SiteEnvironment>("production");
const phpVersion = ref("");
const connectionType = ref<ManagedConnectionType>("ssh");
const sshHost = ref("");
const sshPort = ref<number | "">(22);
const sshUser = ref("");
const sitePath = ref("files");
const isWordpress = ref(true);
const canWPCLI = ref(true);
const hasAgent = ref(false);
const hasMonitoring = ref(true);
const status = ref<ManagedSiteStatus>("active");
const isSubmitting = ref(false);
const errorMessage = ref<string | null>(null);

const isEditMode = computed(() => !!props.editSite);
const modalTitle = computed(() =>
	isEditMode.value ? `Edit ${props.editSite?.domain}` : "Add Site",
);
const submitLabel = computed(() =>
	isEditMode.value ? "Save Changes" : "Add Site",
);

// SpinupWP servers are listed too: an external site can live on a server
// that SpinupWP manages.
const serverOptions = computed(() => managedSitesStore.servers);

const canSubmit = computed(
	() => !isSubmitting.value && domain.value.trim().length > 0,
);

watch(
	() => props.visible,
	(newVal) => {
		if (!newVal) return;
		errorMessage.value = null;
		const s = props.editSite;
		domain.value = s?.domain ?? "";
		serverId.value = s?.server_id ?? "";
		provider.value = s?.provider ?? "manual";
		environment.value = s?.environment ?? "production";
		phpVersion.value = s?.php_version ?? "";
		connectionType.value = s?.connection_type ?? "ssh";
		sshHost.value = s?.ssh_host ?? "";
		sshPort.value = s?.ssh_port ?? 22;
		sshUser.value = s?.ssh_user ?? "";
		sitePath.value = s?.site_path ?? "files";
		isWordpress.value = s?.is_wordpress ?? true;
		canWPCLI.value = s?.can_wp_cli ?? true;
		hasAgent.value = s?.has_agent ?? false;
		hasMonitoring.value = s?.has_monitoring ?? true;
		status.value = s?.status ?? "active";
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

	const payload: ManagedSitePayload = {
		domain: domain.value.trim(),
		server_id: serverId.value,
		provider: provider.value.trim() || "manual",
		environment: environment.value,
		is_wordpress: isWordpress.value,
		php_version: phpVersion.value.trim(),
		connection_type: connectionType.value,
		ssh_host: sshHost.value.trim(),
		ssh_port: sshPort.value === "" ? 22 : Number(sshPort.value),
		ssh_user: sshUser.value.trim(),
		site_path: sitePath.value.trim(),
		can_wp_cli: canWPCLI.value,
		has_agent: hasAgent.value,
		has_monitoring: hasMonitoring.value,
		status: status.value,
	};

	try {
		const saved =
			isEditMode.value && props.editSite
				? await managedSitesStore.updateSite(props.editSite.id, payload)
				: await managedSitesStore.createSite(payload);
		emit("saved", saved);
		emit("close");
	} catch (e: any) {
		errorMessage.value =
			e.message || "An error occurred while saving the site.";
	} finally {
		isSubmitting.value = false;
	}
}
</script>

<template>
	<Teleport to="body">
		<div v-if="visible" class="modal-overlay" @click="handleOverlayClick">
			<div class="modal-content card site-form-modal">
				<header class="modal-header">
					<h2>{{ modalTitle }}</h2>
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
								<label for="site-domain">Domain</label>
								<input
									id="site-domain"
									v-model="domain"
									type="text"
									placeholder="example.com"
									required
									autocomplete="off"
								/>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="site-server">Server</label>
									<select id="site-server" v-model="serverId">
										<option value="">None</option>
										<option
											v-for="server in serverOptions"
											:key="server.id"
											:value="server.id"
										>
											{{ server.name }}
											{{
												server.is_logical
													? "(logical)"
													: ""
											}}
										</option>
									</select>
								</div>
								<div class="form-group">
									<label for="site-provider">Provider</label>
									<input
										id="site-provider"
										v-model="provider"
										type="text"
										placeholder="manual, wpengine, kinsta…"
									/>
								</div>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="site-environment"
										>Environment</label
									>
									<select
										id="site-environment"
										v-model="environment"
									>
										<option value="production">
											Production
										</option>
										<option value="staging">Staging</option>
										<option value="development">
											Development
										</option>
									</select>
								</div>
								<div class="form-group">
									<label for="site-status">Status</label>
									<select id="site-status" v-model="status">
										<option value="active">Active</option>
										<option value="paused">Paused</option>
										<option value="archived">
											Archived
										</option>
									</select>
								</div>
							</div>

							<div class="section-divider">
								<h3 class="font-medium">Connection</h3>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="site-connection-type"
										>Connection Type</label
									>
									<select
										id="site-connection-type"
										v-model="connectionType"
									>
										<option value="ssh">SSH</option>
										<option value="agent">Agent</option>
										<option value="none">None</option>
									</select>
								</div>
								<div class="form-group">
									<label for="site-php">PHP Version</label>
									<input
										id="site-php"
										v-model="phpVersion"
										type="text"
										placeholder="e.g. 8.3"
									/>
								</div>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="site-ssh-host">SSH Host</label>
									<input
										id="site-ssh-host"
										v-model="sshHost"
										type="text"
										placeholder="mysite.ssh.wpengine.net"
										autocomplete="off"
									/>
								</div>
								<div class="form-group">
									<label for="site-ssh-port">SSH Port</label>
									<input
										id="site-ssh-port"
										v-model.number="sshPort"
										type="number"
										min="1"
										max="65535"
									/>
								</div>
							</div>

							<div class="form-row">
								<div class="form-group">
									<label for="site-ssh-user">SSH User</label>
									<input
										id="site-ssh-user"
										v-model="sshUser"
										type="text"
										autocomplete="off"
									/>
								</div>
								<div class="form-group">
									<label for="site-path">Site Path</label>
									<input
										id="site-path"
										v-model="sitePath"
										type="text"
										placeholder="files"
									/>
								</div>
							</div>
							<p class="help-text">
								SSH authentication uses the jman-api host's
								OpenSSH setup (agent, default keys,
								<code>~/.ssh/config</code>). No keys are stored
								in jman.
							</p>

							<div class="section-divider">
								<h3 class="font-medium">Capabilities</h3>
							</div>

							<div class="capability-grid">
								<label class="checkbox-label">
									<input
										v-model="isWordpress"
										type="checkbox"
									/>
									WordPress site
								</label>
								<label class="checkbox-label">
									<input v-model="canWPCLI" type="checkbox" />
									WP-CLI over SSH
								</label>
								<label class="checkbox-label">
									<input v-model="hasAgent" type="checkbox" />
									jman-agent installed
								</label>
								<label class="checkbox-label">
									<input
										v-model="hasMonitoring"
										type="checkbox"
									/>
									Uptime monitoring
								</label>
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
										isSubmitting ? "Saving..." : submitLabel
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
.site-form-modal {
	max-width: 640px;
}

.capability-grid {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: 8px 16px;
}

.checkbox-label {
	display: flex;
	align-items: center;
	gap: 8px;
	cursor: pointer;
}

@media (max-width: 640px) {
	.capability-grid {
		grid-template-columns: 1fr;
	}
}
</style>
