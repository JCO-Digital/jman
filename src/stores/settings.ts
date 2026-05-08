import { ref, watch } from "vue";
import { defineStore } from "pinia";

const LS_SETTINGS = "jman_settings";

interface AppSettings {
	monitorRefreshInterval: number;
	dataRefreshInterval: number;
	vulnCvssThreshold: number;
	vulnTotalThreshold: number;
}

export const useSettingsStore = defineStore("settings", () => {
	// State
	const monitorRefreshInterval = ref(60); // Default 60 seconds
	const dataRefreshInterval = ref(300); // Default 300 seconds
	const vulnCvssThreshold = ref(7); // Default CVSS 7
	const vulnTotalThreshold = ref(8); // Default 8 vulnerabilities

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
				if (data.vulnCvssThreshold !== undefined) {
					vulnCvssThreshold.value = data.vulnCvssThreshold;
				}
				if (data.vulnTotalThreshold !== undefined) {
					vulnTotalThreshold.value = data.vulnTotalThreshold;
				}
			} catch (e) {
				console.error("Failed to parse settings from localStorage", e);
			}
		}
	}

	// Persist changes
	watch(
		[
			() => monitorRefreshInterval.value,
			() => dataRefreshInterval.value,
			() => vulnCvssThreshold.value,
			() => vulnTotalThreshold.value,
		],
		([monitorInterval, dataInterval, cvssThreshold, totalThreshold]) => {
			localStorage.setItem(
				LS_SETTINGS,
				JSON.stringify({
					monitorRefreshInterval: monitorInterval,
					dataRefreshInterval: dataInterval,
					vulnCvssThreshold: cvssThreshold,
					vulnTotalThreshold: totalThreshold,
				}),
			);
		},
	);

	return {
		monitorRefreshInterval,
		dataRefreshInterval,
		vulnCvssThreshold,
		vulnTotalThreshold,
		initialize,
	};
});
