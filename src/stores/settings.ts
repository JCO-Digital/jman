import { ref, watch } from "vue";
import { defineStore } from "pinia";

const LS_SETTINGS = "jman_settings";

interface AppSettings {
	monitorRefreshInterval: number;
	dataRefreshInterval: number;
}

export const useSettingsStore = defineStore("settings", () => {
	// State
	const monitorRefreshInterval = ref(60); // Default 60 seconds
	const dataRefreshInterval = ref(300); // Default 300 seconds

	// Initialize from localStorage
	function initialize() {
		const stored = localStorage.getItem(LS_SETTINGS);
		if (stored) {
			try {
				const data = JSON.parse(stored) as AppSettings;
				if (data.monitorRefreshInterval) {
					monitorRefreshInterval.value = data.monitorRefreshInterval;
				}
				if (data.dataRefreshInterval) {
					dataRefreshInterval.value = data.dataRefreshInterval;
				}
			} catch (e) {
				console.error("Failed to parse settings from localStorage", e);
			}
		}
	}

	// Persist changes
	watch(
		[() => monitorRefreshInterval.value, () => dataRefreshInterval.value],
		([monitorInterval, dataInterval]) => {
			localStorage.setItem(
				LS_SETTINGS,
				JSON.stringify({
					monitorRefreshInterval: monitorInterval,
					dataRefreshInterval: dataInterval,
				}),
			);
		},
	);

	return {
		monitorRefreshInterval,
		dataRefreshInterval,
		initialize,
	};
});
