import { ref, watch, nextTick } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";

const LS_SETTINGS = "jman_settings";
const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

interface AppSettings {
	monitorRefreshInterval: number;
	dataRefreshInterval: number;
	vulnCvssThreshold: number;
	vulnTotalThreshold: number;
}

export const useSettingsStore = defineStore("settings", () => {
	const authStore = useAuthStore();

	// State
	const monitorRefreshInterval = ref(60); // Default 60 seconds
	const dataRefreshInterval = ref(300); // Default 300 seconds
	const vulnCvssThreshold = ref(7); // Default CVSS 7
	const vulnTotalThreshold = ref(8); // Default 8 vulnerabilities

	let debounceTimeout: ReturnType<typeof setTimeout> | null = null;

	const isInitializing = ref(false);

	// Initialize from API (with localStorage fallback)
	async function initialize() {
		// Load from LS first for immediate UI
		const stored = localStorage.getItem(LS_SETTINGS);
		if (stored) {
			try {
				applySettings(JSON.parse(stored));
			} catch (e) {
				console.error("Failed to parse settings from localStorage", e);
			}
		}

		if (!authStore.isAuthenticated) return;

		isInitializing.value = true;
		try {
			const response = await fetch(`${BASE_URL}/settings/general`, {
				headers: authStore.authHeader,
			});
			if (response.ok) {
				const data = await response.json();
				if (data && data.value) {
					applySettings(data.value);
				}
			}
		} catch (e) {
			console.error("Failed to fetch settings from API", e);
		} finally {
			// Wait for any watches triggered by applySettings to fire while isInitializing is still true
			await nextTick();
			isInitializing.value = false;
		}
	}

	function applySettings(data: Partial<AppSettings>) {
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
	}

	async function saveSettings() {
		const settings: AppSettings = {
			monitorRefreshInterval: monitorRefreshInterval.value,
			dataRefreshInterval: dataRefreshInterval.value,
			vulnCvssThreshold: vulnCvssThreshold.value,
			vulnTotalThreshold: vulnTotalThreshold.value,
		};

		// Always save to LS
		localStorage.setItem(LS_SETTINGS, JSON.stringify(settings));

		if (debounceTimeout) {
			clearTimeout(debounceTimeout);
		}

		// Save to API if authenticated and not in the middle of initializing
		if (authStore.isAuthenticated && !isInitializing.value) {
			debounceTimeout = setTimeout(async () => {
				try {
					await fetch(`${BASE_URL}/settings/general`, {
						method: "POST",
						headers: {
							...authStore.authHeader,
							"Content-Type": "application/json",
						},
						body: JSON.stringify(settings),
					});
				} catch (e) {
					console.error("Failed to save settings to API", e);
				} finally {
					debounceTimeout = null;
				}
			}, 2000);
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
		() => {
			saveSettings();
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
