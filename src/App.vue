<script setup lang="ts">
import { watch } from "vue";
import { RouterView } from "vue-router";
import { useDataStore } from "./stores/data";
import { useAuthStore } from "./stores/auth";
import AppNav from "./components/AppNav.vue";
import packageInfo from "../package.json";

const dataStore = useDataStore();
const authStore = useAuthStore();

authStore.initialize();

// Load data whenever the user becomes authenticated
watch(
	() => authStore.isAuthenticated,
	(authenticated) => {
		if (authenticated) {
			dataStore.initData();
		}
	},
	{ immediate: true },
);

const version = packageInfo.version;

const handleLogout = () => {
	dataStore.clearCache();
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
