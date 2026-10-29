<script setup lang="ts">
import { useSettingsStore } from "../../stores/settings";

const settingsStore = useSettingsStore();
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
</template>

<style scoped>
/* Scoped styles removed in favor of global form and utility classes */
</style>
