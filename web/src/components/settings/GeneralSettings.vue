<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useSettingsStore } from "../../stores/settings";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import { BASE_URL } from "../../utils/api";
import type { VulnSettings } from "../../types";

const settingsStore = useSettingsStore();
const authStore = useAuthStore();
const userStore = useUserStore();

// ─── Default Vulnerability Task Assignee (admin only) ───────────────────────

const vulnDefaultAssignee = ref("");
const vulnAssigneeSaving = ref(false);
const vulnAssigneeSuccess = ref("");
const vulnAssigneeError = ref("");

onMounted(async () => {
	if (!authStore.canAdmin) return;
	userStore.ensureUsers();
	try {
		const res = await fetch(`${BASE_URL}/vuln-settings`, {
			headers: authStore.authHeader,
		});
		if (res.ok) {
			const data: VulnSettings = await res.json();
			vulnDefaultAssignee.value = data.defaultAssignee;
		}
	} catch (e) {
		console.error("Failed to fetch vulnerability task settings", e);
	}
});

async function saveVulnAssignee() {
	vulnAssigneeSaving.value = true;
	vulnAssigneeSuccess.value = "";
	vulnAssigneeError.value = "";
	try {
		const res = await fetch(`${BASE_URL}/vuln-settings`, {
			method: "POST",
			headers: {
				...authStore.authHeader,
				"Content-Type": "application/json",
			},
			body: JSON.stringify({
				defaultAssignee: vulnDefaultAssignee.value,
			}),
		});
		if (!res.ok) throw new Error("Failed to save");
		vulnAssigneeSuccess.value = "Saved.";
	} catch (e: any) {
		vulnAssigneeError.value =
			e.message || "Failed to save default assignee.";
	} finally {
		vulnAssigneeSaving.value = false;
	}
}
</script>

<template>
	<section class="card">
		<h2>Application Settings</h2>
		<div class="content mt-4">
			<div class="form-group max-w-320">
				<label for="monitor-refresh-interval"
					>Monitor History Refresh Interval (seconds)</label
				>
				<input
					id="monitor-refresh-interval"
					v-model.number="settingsStore.monitorRefreshInterval"
					type="number"
					min="10"
					max="3600"
				/>
				<p class="help-text">
					How often the uptime history data is automatically reloaded
					(Default: 60s).
				</p>
			</div>

			<div class="form-group max-w-320">
				<label for="data-refresh-interval"
					>Site & Plugin Data Refresh Interval (seconds)</label
				>
				<input
					id="data-refresh-interval"
					v-model.number="settingsStore.dataRefreshInterval"
					type="number"
					min="30"
					max="3600"
				/>
				<p class="help-text">
					How often sites, servers, and plugins are automatically
					reloaded (Default: 300s).
				</p>
			</div>

			<div class="form-group max-w-320">
				<label for="vuln-cvss-threshold"
					>Vulnerability CVSS Threshold</label
				>
				<input
					id="vuln-cvss-threshold"
					v-model.number="settingsStore.vulnCvssThreshold"
					type="number"
					min="0"
					max="10"
					step="0.1"
				/>
				<p class="help-text">
					Minimum CVSS score to show site in vulnerability widget
					(Default: 7.0).
				</p>
			</div>

			<div class="form-group max-w-320">
				<label for="vuln-total-threshold"
					>Total Vulnerabilities Threshold</label
				>
				<input
					id="vuln-total-threshold"
					v-model.number="settingsStore.vulnTotalThreshold"
					type="number"
					min="1"
					max="100"
				/>
				<p class="help-text">
					Minimum number of vulnerabilities to show site in
					vulnerability widget (Default: 8).
				</p>
			</div>
		</div>
	</section>

	<section v-if="authStore.canAdmin" class="card">
		<h2>Vulnerability Task Assignment</h2>
		<div class="content mt-4">
			<div class="form-group max-w-320">
				<label for="vuln-default-assignee">Default Assignee</label>
				<select
					id="vuln-default-assignee"
					v-model="vulnDefaultAssignee"
				>
					<option value="">— Unassigned —</option>
					<option
						v-for="user in userStore.users"
						:key="user.username"
						:value="user.username"
					>
						{{ user.displayName }}
					</option>
				</select>
				<p class="help-text">
					When set, newly-created vulnerability tasks are
					automatically assigned to this user. Existing tasks are not
					affected.
				</p>
			</div>

			<div v-if="vulnAssigneeSuccess" class="feedback success">
				{{ vulnAssigneeSuccess }}
			</div>
			<div v-if="vulnAssigneeError" class="feedback error">
				{{ vulnAssigneeError }}
			</div>

			<button
				class="btn btn-primary"
				:disabled="vulnAssigneeSaving"
				@click="saveVulnAssignee"
			>
				{{ vulnAssigneeSaving ? "Saving..." : "Save" }}
			</button>
		</div>
	</section>
</template>

<style scoped>
/* Scoped styles removed in favor of global form and utility classes */
</style>
