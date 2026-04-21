<script setup lang="ts">
import { RouterLink, useRoute } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import LoadingSpinner from "./LoadingSpinner.vue";

const route = useRoute();
const authStore = useAuthStore();
const dataStore = useDataStore();

const emit = defineEmits<{
	(e: "logout"): void;
}>();

const handleLogout = () => {
	emit("logout");
};

const handleRefresh = () => {
	dataStore.refreshData();
};
</script>

<template>
	<nav v-if="authStore.isAuthenticated" class="app-nav">
		<div class="nav-container">
			<div class="nav-links">
				<RouterLink
					to="/"
					class="nav-item"
					:class="{
						active: route.name === 'home',
					}"
				>
					Dashboard
				</RouterLink>
				<RouterLink
					to="/sites"
					class="nav-item"
					:class="{
						active: route.name === 'sites' || route.name === 'site-detail',
					}"
				>
					Sites
				</RouterLink>
				<RouterLink
					to="/plugins"
					class="nav-item"
					:class="{
						active: route.name === 'plugins' || route.name === 'plugin-detail',
					}"
				>
					Plugins
				</RouterLink>
			</div>
			<div class="nav-actions">
				<button
					class="icon-btn refresh-btn"
					@click="handleRefresh"
					:disabled="dataStore.isLoading"
					title="Refresh data"
				>
					<svg
						v-if="!dataStore.isLoading"
						xmlns="http://www.w3.org/2000/svg"
						width="18"
						height="18"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d="M21 2v6h-6"></path>
						<path d="M3 12a9 9 0 0 1 15-6.7L21 8"></path>
						<path d="M3 22v-6h6"></path>
						<path d="M21 12a9 9 0 0 1-15 6.7L3 16"></path>
					</svg>
					<LoadingSpinner v-else small />
				</button>
				<RouterLink
					to="/settings"
					class="icon-btn settings-btn"
					:class="{ active: route.name === 'settings' }"
					title="Settings"
				>
					<svg
						xmlns="http://www.w3.org/2000/svg"
						width="18"
						height="18"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<circle cx="12" cy="12" r="3"></circle>
						<path
							d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
						></path>
					</svg>
				</RouterLink>
				<div class="nav-user">
					<span v-if="authStore.user" class="user-display-name">
						{{ authStore.user.displayName }}
					</span>
					<button class="logout-btn" @click="handleLogout">Logout</button>
				</div>
			</div>
		</div>
	</nav>
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

.nav-actions {
	display: flex;
	align-items: center;
	gap: 16px;
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

.icon-btn {
	background: none;
	border: none;
	padding: 8px;
	cursor: pointer;
	color: var(--text-muted);
	display: flex;
	align-items: center;
	justify-content: center;
	border-radius: 4px;
	transition:
		background-color 0.2s,
		color 0.2s;
}

.icon-btn:hover:not(:disabled),
.icon-btn.active {
	background-color: var(--bg-hover);
	color: var(--primary);
}

.icon-btn:disabled {
	cursor: not-allowed;
	opacity: 0.6;
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
