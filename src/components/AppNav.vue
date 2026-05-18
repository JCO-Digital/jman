<script setup lang="ts">
import { RouterLink, useRoute } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import LoadingSpinner from "./LoadingSpinner.vue";
import AppIcon from "./AppIcon.vue";

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
		<div class="nav-container flex-between">
			<div class="flex-row">
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
						active:
							route.name === 'sites' ||
							route.name === 'site-detail',
					}"
				>
					Sites
				</RouterLink>
				<RouterLink
					to="/plugins"
					class="nav-item"
					:class="{
						active:
							route.name === 'plugins' ||
							route.name === 'plugin-detail',
					}"
				>
					Plugins
				</RouterLink>
				<RouterLink
					to="/organizations"
					class="nav-item"
					:class="{
						active:
							route.name === 'organizations' ||
							route.name === 'organization-detail',
					}"
				>
					Organizations
				</RouterLink>
				<RouterLink
					to="/tasks"
					class="nav-item"
					:class="{ active: route.name === 'tasks' }"
				>
					Tasks
				</RouterLink>
				<RouterLink
					to="/inventory"
					class="nav-item"
					:class="{
						active:
							route.name === 'assets' ||
							route.name === 'asset-templates',
					}"
				>
					Assets
				</RouterLink>
			</div>
			<div class="flex-row gap-4">
				<button
					class="icon-btn"
					:disabled="dataStore.isLoading"
					title="Refresh data"
					@click="handleRefresh"
				>
					<AppIcon
						v-if="!dataStore.isLoading"
						name="refresh"
						size="18"
					/>
					<LoadingSpinner v-else small />
				</button>
				<RouterLink
					to="/settings"
					class="icon-btn"
					:class="{ active: route.name === 'settings' }"
					title="Settings"
				>
					<AppIcon name="settings" size="18" />
				</RouterLink>
				<div class="flex-row gap-3">
					<span
						v-if="authStore.user"
						class="sub-text font-medium hide-mobile"
					>
						{{ authStore.user.displayName }}
					</span>
					<button
						class="btn btn-outline btn-sm"
						@click="handleLogout"
					>
						Logout
					</button>
				</div>
			</div>
		</div>
	</nav>
</template>
