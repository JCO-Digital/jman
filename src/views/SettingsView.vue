<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useSettingsStore } from "../stores/settings";
import { useMonitorStore } from "../stores/monitor";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";

const settingsStore = useSettingsStore();
const monitorStore = useMonitorStore();

const newDomain = ref("");
const newReason = ref("");
const isSubmitting = ref(false);
const error = ref<string | null>(null);

onMounted(() => {
	monitorStore.fetchIgnored();
});

const handleAddIgnored = async () => {
	if (!newDomain.value) return;

	isSubmitting.value = true;
	error.value = null;
	try {
		await monitorStore.addIgnored(newDomain.value, newReason.value);
		newDomain.value = "";
		newReason.value = "";
	} catch (e: any) {
		error.value = e.message || "Failed to add domain to ignore list";
	} finally {
		isSubmitting.value = false;
	}
};

const handleRemoveIgnored = async (domain: string) => {
	if (!confirm(`Are you sure you want to stop ignoring ${domain}?`)) return;

	try {
		await monitorStore.removeIgnored(domain);
	} catch (e: any) {
		alert(e.message || "Failed to remove domain");
	}
};

const formatDate = (dateStr: string) => {
	return new Date(dateStr).toLocaleString(undefined, {
		year: "numeric",
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Settings" />

		<main class="content">
			<!-- Local App Settings -->
			<section class="card">
				<h2>Application Settings</h2>
				<div class="settings-form">
					<div class="setting-group">
						<label for="monitor-refresh-interval"
							>Monitor History Refresh Interval (seconds)</label
						>
						<input
							id="monitor-refresh-interval"
							type="number"
							v-model.number="settingsStore.monitorRefreshInterval"
							min="10"
							max="3600"
							class="refresh-input"
						/>
						<p class="help-text">
							How often the uptime history data is automatically reloaded
							(Default: 60s).
						</p>
					</div>

					<div class="setting-group refresh-interval-group">
						<label for="data-refresh-interval"
							>Site & Plugin Data Refresh Interval (seconds)</label
						>
						<input
							id="data-refresh-interval"
							type="number"
							v-model.number="settingsStore.dataRefreshInterval"
							min="30"
							max="3600"
							class="refresh-input"
						/>
						<p class="help-text">
							How often sites, servers, and plugins are automatically reloaded
							(Default: 300s).
						</p>
					</div>
				</div>
			</section>

			<!-- Ignored Domains Section -->
			<section class="card">
				<h2>Ignored Domains</h2>
				<p class="section-desc">
					Sites in this list are excluded from uptime monitoring.
				</p>

				<!-- Add form -->
				<form @submit.prevent="handleAddIgnored" class="add-ignored-form">
					<div class="input-group">
						<input
							type="text"
							v-model="newDomain"
							placeholder="domain.com"
							required
							class="text-input"
						/>
						<input
							type="text"
							v-model="newReason"
							placeholder="Reason (optional)"
							class="text-input"
						/>
						<button
							type="submit"
							class="btn btn-primary"
							:disabled="isSubmitting"
						>
							{{ isSubmitting ? "Adding..." : "Add to list" }}
						</button>
					</div>
					<p v-if="error" class="error-text small">{{ error }}</p>
				</form>

				<div v-if="monitorStore.isLoadingIgnored" class="state-container">
					<LoadingSpinner message="Loading ignored domains..." />
				</div>

				<div
					v-else-if="monitorStore.ignoredDomains.length > 0"
					class="table-container"
				>
					<table class="data-table">
						<thead>
							<tr>
								<th>Domain</th>
								<th>Reason</th>
								<th>Added At</th>
								<th class="text-right">Actions</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-for="site in monitorStore.ignoredDomains"
								:key="site.domain"
							>
								<td class="font-medium">{{ site.domain }}</td>
								<td>{{ site.reason || "-" }}</td>
								<td class="text-muted small">
									{{ formatDate(site.created_at) }}
								</td>
								<td class="text-right">
									<button
										class="btn-text danger"
										@click="handleRemoveIgnored(site.domain)"
									>
										Remove
									</button>
								</td>
							</tr>
						</tbody>
					</table>
				</div>

				<div v-else class="state-container">
					<p class="empty-text">No domains are currently ignored.</p>
				</div>
			</section>
		</main>
	</div>
</template>

<style scoped>
.refresh-interval-group {
	margin-top: 20px;
}

.settings-form {
	margin-top: 16px;
}

.setting-group {
	display: flex;
	flex-direction: column;
	gap: 8px;
	max-width: 400px;
}

.setting-group label {
	font-weight: 600;
	font-size: 14px;
	color: var(--text-heading);
}

.refresh-input,
.text-input {
	padding: 8px 12px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 14px;
}

.refresh-input {
	width: 120px;
}

.add-ignored-form {
	margin-bottom: 24px;
	background: var(--bg-body);
	padding: 16px;
	border-radius: 6px;
}

.input-group {
	display: flex;
	gap: 12px;
}

.input-group .text-input {
	flex: 1;
}

.font-medium {
	font-weight: 500;
}

.small {
	font-size: 0.85em;
}

.error-text.small {
	margin-top: 8px;
	color: var(--error-text);
}

.help-text {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}

.section-desc {
	color: var(--text-muted);
	margin-bottom: 20px;
}

.state-container {
	padding: 40px;
	text-align: center;
	color: var(--text-muted);
}

.empty-text {
	margin: 0;
	font-style: italic;
}

.info-footer {
	margin-top: 24px;
	padding-top: 16px;
	border-top: 1px solid var(--border-color);
	color: var(--text-muted);
}

.text-right {
	text-align: right;
}

.btn-text {
	background: none;
	border: none;
	color: var(--primary);
	cursor: pointer;
	font-size: 14px;
	padding: 0;
	font-weight: 500;
}

.btn-text.danger {
	color: var(--error-text);
}

.btn-text:disabled {
	color: var(--text-disabled);
	cursor: not-allowed;
}
</style>
