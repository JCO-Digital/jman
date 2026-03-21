<script setup lang="ts">
import { watch } from "vue";
import { RouterView, RouterLink, useRoute } from "vue-router";
import { useDataStore } from "./stores/data";
import { useAuthStore } from "./stores/auth";
import packageInfo from "../package.json";

const route = useRoute();
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
		<nav v-if="authStore.isAuthenticated" class="app-nav">
			<div class="nav-container">
				<div class="nav-links">
					<RouterLink
						to="/"
						class="nav-item"
						:class="{
							active: route.name === 'home' || route.name === 'site-detail',
						}"
					>
						Sites
					</RouterLink>
					<RouterLink
						to="/plugins"
						class="nav-item"
						:class="{
							active:
								route.name === 'plugins' || route.name === 'plugin-detail',
						}"
					>
						Plugins
					</RouterLink>
				</div>
				<div class="nav-user">
					<span v-if="authStore.user" class="user-display-name">
						{{ authStore.user.displayName }}
					</span>
					<button class="logout-btn" @click="handleLogout">Logout</button>
				</div>
			</div>
		</nav>
		<div class="main-content">
			<RouterView />
		</div>
		<footer v-if="authStore.isAuthenticated" class="app-footer">
			v{{ version }}
		</footer>
	</div>
</template>

<style scoped>
.nav-container {
	display: flex;
	justify-content: space-between;
	align-items: center;
}

.nav-links {
	display: flex;
	gap: 24px;
}

.nav-user {
	display: flex;
	align-items: center;
	gap: 12px;
}

.user-display-name {
	font-size: 14px;
	color: var(--text-muted);
	font-weight: 500;
}

.logout-btn {
	padding: 6px 14px;
	background-color: transparent;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	color: var(--text-main);
	font-size: 13px;
	font-weight: 500;
	cursor: pointer;
	transition: background-color 0.2s;
}

.logout-btn:hover {
	background-color: var(--bg-hover);
}
</style>
