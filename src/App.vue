<script setup lang="ts">
import { watch, onUnmounted } from "vue";
import { RouterView } from "vue-router";
import { useDataStore } from "./stores/data";
import { useAuthStore } from "./stores/auth";
import { useSettingsStore } from "./stores/settings";
import { useMonitorStore } from "./stores/monitor";
import { useUserStore } from "./stores/user";
import AppNav from "./components/AppNav.vue";
import packageInfo from "../package.json";

const dataStore = useDataStore();
const authStore = useAuthStore();
const settingsStore = useSettingsStore();
const userStore = useUserStore();

authStore.initialize();
settingsStore.initialize();

const monitorStore = useMonitorStore();

let monitorIntervalId: ReturnType<typeof setInterval> | null = null;
let dataIntervalId: ReturnType<typeof setInterval> | null = null;

const startIntervals = () => {
	stopIntervals();
	if (!authStore.isAuthenticated) return;

	// Monitor history refresh
	monitorIntervalId = setInterval(() => {
		monitorStore.fetchHistory();
	}, settingsStore.monitorRefreshInterval * 1000);

	// General data refresh (sites, servers, plugins, etc.)
	dataIntervalId = setInterval(() => {
		dataStore.refreshData();
	}, settingsStore.dataRefreshInterval * 1000);
};

const stopIntervals = () => {
	if (monitorIntervalId) {
		clearInterval(monitorIntervalId);
		monitorIntervalId = null;
	}
	if (dataIntervalId) {
		clearInterval(dataIntervalId);
		dataIntervalId = null;
	}
};

// Load data whenever the user becomes authenticated and handle intervals
watch(
	() => authStore.isAuthenticated,
	(authenticated) => {
		if (authenticated) {
			dataStore.initData();
			startIntervals();
		} else {
			stopIntervals();
		}
	},
	{ immediate: true },
);

// Restart intervals if interval settings change
watch(
	[
		() => settingsStore.monitorRefreshInterval,
		() => settingsStore.dataRefreshInterval,
	],
	() => {
		if (authStore.isAuthenticated) {
			startIntervals();
		}
	},
);

onUnmounted(() => {
	stopIntervals();
});

const version = packageInfo.version;

const handleLogout = () => {
	stopIntervals();
	dataStore.clearCache();
	userStore.clearCache();
	authStore.logout();
};
</script>

<template>
	<div class="app-container">
		<AppNav @logout="handleLogout" />
		<div class="main-content">
			<RouterView />
		</div>
		<footer v-if="authStore.isAuthenticated" class="app-footer">
			v{{ version }}
		</footer>
	</div>
</template>

<style scoped>
/* Navigation styles moved to AppNav.vue */
</style>
