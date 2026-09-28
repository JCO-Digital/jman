<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import { useIncidentStore } from "../stores/incidents";
import LoadingSpinner from "./LoadingSpinner.vue";
import AppIcon from "./AppIcon.vue";
import packageInfo from "../../package.json";

const route = useRoute();
const authStore = useAuthStore();
const dataStore = useDataStore();
const incidentStore = useIncidentStore();

const emit = defineEmits<{
	(e: "logout"): void;
}>();

interface NavLink {
	to: string;
	label: string;
	match: string[];
	showIncidentBadge?: boolean;
}

const navLinks: NavLink[] = [
	{ to: "/", label: "Dashboard", match: ["home"] },
	{
		to: "/incidents",
		label: "Incidents",
		match: ["incidents"],
		showIncidentBadge: true,
	},
	{ to: "/sites", label: "Sites", match: ["sites", "site-detail"] },
	{ to: "/plugins", label: "Plugins", match: ["plugins", "plugin-detail"] },
	{ to: "/tasks", label: "Tasks", match: ["tasks"] },
	{
		to: "/organizations",
		label: "Organizations",
		match: ["organizations", "organization-detail"],
	},
	{
		to: "/inventory",
		label: "Assets",
		match: ["assets", "asset-templates", "payment-methods"],
	},
	{ to: "/reports", label: "Reports", match: ["reports", "report-runner"] },
];

const version = packageInfo.version;
const menuOpen = ref(false);

const isActive = (link: NavLink) =>
	link.match.includes(String(route.name ?? ""));

const currentSection = computed(() => {
	if (route.name === "settings") return "Settings";
	return navLinks.find(isActive)?.label ?? "jman";
});

const handleLogout = () => {
	menuOpen.value = false;
	emit("logout");
};

const handleRefresh = () => {
	dataStore.refreshData();
};

const onKeydown = (e: KeyboardEvent) => {
	if (e.key === "Escape") menuOpen.value = false;
};

watch(
	() => route.fullPath,
	() => {
		menuOpen.value = false;
	},
);

watch(menuOpen, (open) => {
	document.body.classList.toggle("nav-open", open);
});

onMounted(() => {
	document.addEventListener("keydown", onKeydown);
});

onUnmounted(() => {
	document.removeEventListener("keydown", onKeydown);
	document.body.classList.remove("nav-open");
});
</script>

<template>
	<nav v-if="authStore.isAuthenticated" class="app-nav">
		<!-- Desktop navigation -->
		<div class="nav-container desktop-nav flex-between">
			<div class="flex-row">
				<RouterLink
					v-for="link in navLinks"
					:key="link.to"
					:to="link.to"
					class="nav-item"
					:class="{
						active: isActive(link),
						'nav-item-with-badge': link.showIncidentBadge,
					}"
				>
					{{ link.label }}
					<span
						v-if="
							link.showIncidentBadge &&
							incidentStore.activeCount > 0
						"
						class="nav-badge"
					>
						{{ incidentStore.activeCount }}
					</span>
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
					<span v-if="authStore.user" class="sub-text font-medium">
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

		<!-- Mobile top bar -->
		<div class="mobile-bar">
			<button
				class="icon-btn menu-toggle"
				title="Open menu"
				aria-controls="nav-drawer"
				:aria-expanded="menuOpen"
				@click="menuOpen = true"
			>
				<AppIcon name="menu" size="22" />
				<span
					v-if="incidentStore.activeCount > 0"
					class="menu-toggle-dot"
				></span>
			</button>
			<span class="mobile-bar-title">{{ currentSection }}</span>
			<button
				class="icon-btn"
				:disabled="dataStore.isLoading"
				title="Refresh data"
				@click="handleRefresh"
			>
				<AppIcon v-if="!dataStore.isLoading" name="refresh" size="18" />
				<LoadingSpinner v-else small />
			</button>
		</div>

		<!-- Mobile drawer -->
		<Transition name="nav-fade">
			<div
				v-if="menuOpen"
				class="nav-backdrop"
				@click="menuOpen = false"
			></div>
		</Transition>
		<Transition name="nav-slide">
			<aside
				v-if="menuOpen"
				id="nav-drawer"
				class="nav-drawer"
				role="dialog"
				aria-modal="true"
				aria-label="Navigation"
			>
				<div class="nav-drawer-header">
					<span class="nav-drawer-title">jman</span>
					<button
						class="icon-btn"
						title="Close menu"
						@click="menuOpen = false"
					>
						<AppIcon name="x" size="20" />
					</button>
				</div>
				<div class="nav-drawer-links">
					<RouterLink
						v-for="link in navLinks"
						:key="link.to"
						:to="link.to"
						class="nav-drawer-item"
						:class="{ active: isActive(link) }"
					>
						{{ link.label }}
						<span
							v-if="
								link.showIncidentBadge &&
								incidentStore.activeCount > 0
							"
							class="nav-badge"
						>
							{{ incidentStore.activeCount }}
						</span>
					</RouterLink>
					<div class="nav-drawer-divider"></div>
					<RouterLink
						to="/settings"
						class="nav-drawer-item"
						:class="{ active: route.name === 'settings' }"
					>
						<span class="flex-row gap-2">
							<AppIcon name="settings" size="16" />
							Settings
						</span>
					</RouterLink>
				</div>
				<div class="nav-drawer-footer">
					<span
						v-if="authStore.user"
						class="sub-text font-medium truncate"
					>
						{{ authStore.user.displayName }}
					</span>
					<button
						class="btn btn-outline btn-sm w-full"
						@click="handleLogout"
					>
						Logout
					</button>
					<span class="sub-text text-center">v{{ version }}</span>
				</div>
			</aside>
		</Transition>
	</nav>
</template>
