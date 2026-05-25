import { ref, watch, nextTick } from "vue";
import { defineStore } from "pinia";
import { useAuthStore } from "./auth";
import type { DashboardWidgetType } from "../types";
import { BASE_URL } from "../utils/api";

const LS_SETTINGS = "jman_settings";

interface AppSettings {
	monitorRefreshInterval: number;
	dataRefreshInterval: number;
	vulnCvssThreshold: number;
	vulnTotalThreshold: number;
	dashboardLayout: DashboardWidgetType[];
}

export const useSettingsStore = defineStore("settings", () => {
	const authStore = useAuthStore();

	// State
	const monitorRefreshInterval = ref(60); // Default 60 seconds
	const dataRefreshInterval = ref(300); // Default 300 seconds
	const vulnCvssThreshold = ref(7); // Default CVSS 7
	const vulnTotalThreshold = ref(8); // Default 8 vulnerabilities
	const dashboardLayout = ref<DashboardWidgetType[]>([
		"stats",
		"tasks",
		"vulnerabilities",
		"renewals",
	]);

	const isInitializing = ref(false);
	let debounceTimeout: ReturnType<typeof setTimeout> | null = null;

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
		if (data.monitorRefreshInterval != null) {
			monitorRefreshInterval.value = Math.max(
				10,
				Math.min(3600, data.monitorRefreshInterval),
			);
		}
		if (data.dataRefreshInterval != null) {
			dataRefreshInterval.value = Math.max(
				10,
				Math.min(3600, data.dataRefreshInterval),
			);
		}
		if (data.vulnCvssThreshold !== undefined) {
			vulnCvssThreshold.value = data.vulnCvssThreshold;
		}
		if (data.vulnTotalThreshold !== undefined) {
			vulnTotalThreshold.value = data.vulnTotalThreshold;
		}
		if (data.dashboardLayout && Array.isArray(data.dashboardLayout)) {
			dashboardLayout.value = data.dashboardLayout;
		}
	}

	async function saveSettings() {
		const settings: AppSettings = {
			monitorRefreshInterval: monitorRefreshInterval.value,
			dataRefreshInterval: dataRefreshInterval.value,
			vulnCvssThreshold: vulnCvssThreshold.value,
			vulnTotalThreshold: vulnTotalThreshold.value,
			dashboardLayout: dashboardLayout.value,
		};

		// Always save to LS
		localStorage.setItem(LS_SETTINGS, JSON.stringify(settings));

		if (debounceTimeout) {
			clearTimeout(debounceTimeout);
		}

		// Save to API if authenticated and not in the middle of initializing.
		// The isInitializing guard prevents the watchers that fire when
		// applySettings() writes reactive state from immediately writing
		// those values back to the API before the full fetch has settled.
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
			() => dashboardLayout.value,
		],
		() => {
			saveSettings();
		},
		{ deep: true },
	);

	return {
		monitorRefreshInterval,
		dataRefreshInterval,
		vulnCvssThreshold,
		vulnTotalThreshold,
		dashboardLayout,
		initialize,
	};
});
