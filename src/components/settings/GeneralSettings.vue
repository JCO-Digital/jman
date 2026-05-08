<script setup lang="ts">
import { useSettingsStore } from "../../stores/settings";

const settingsStore = useSettingsStore();
</script>

<template>
	<section class="card">
		<h2>Application Settings</h2>
		<div class="settings-form">
			<div class="setting-group">
				<label for="monitor-refresh-interval"
					>Monitor History Refresh Interval (seconds)</label
				>
				<input
					id="monitor-refresh-interval"
					v-model.number="settingsStore.monitorRefreshInterval"
					type="number"
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
					v-model.number="settingsStore.dataRefreshInterval"
					type="number"
					min="30"
					max="3600"
					class="refresh-input"
				/>
				<p class="help-text">
					How often sites, servers, and plugins are automatically
					reloaded (Default: 300s).
				</p>
			</div>

			<div class="setting-group refresh-interval-group">
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
					class="refresh-input"
				/>
				<p class="help-text">
					Minimum CVSS score to show site in vulnerability widget
					(Default: 7.0).
				</p>
			</div>

			<div class="setting-group refresh-interval-group">
				<label for="vuln-total-threshold"
					>Total Vulnerabilities Threshold</label
				>
				<input
					id="vuln-total-threshold"
					v-model.number="settingsStore.vulnTotalThreshold"
					type="number"
					min="1"
					max="100"
					class="refresh-input"
				/>
				<p class="help-text">
					Minimum number of vulnerabilities to show site in
					vulnerability widget (Default: 8).
				</p>
			</div>
		</div>
	</section>
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

	@media (max-width: 640px) {
		max-width: none;
	}
}

.setting-group label {
	font-weight: 600;
	font-size: 14px;
	color: var(--text-heading);
}

.refresh-input {
	padding: 8px 12px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	font-size: 14px;
	width: 120px;

	@media (max-width: 640px) {
		width: 100%;
	}
}

.help-text {
	font-size: 12px;
	color: var(--text-muted);
	margin: 0;
}
</style>
