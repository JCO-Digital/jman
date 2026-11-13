<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import { useIncidentStore } from "../stores/incidents";
import { useVulnStore } from "../stores/vulns";
import { useSidebar } from "../composables/useSidebar";
import { findNavItem } from "../navigation";
import { pageTitle } from "../composables/usePageTitle";
import AppIcon from "./AppIcon.vue";
import LoadingSpinner from "./LoadingSpinner.vue";

const emit = defineEmits<{
	(e: "logout"): void;
}>();

const route = useRoute();
const authStore = useAuthStore();
const dataStore = useDataStore();
const incidentStore = useIncidentStore();
const vulnStore = useVulnStore();
const { mobileOpen, openMobile } = useSidebar();

const monitoringCount = computed(
	() => incidentStore.activeCount + (vulnStore.summary?.overdue ?? 0),
);

interface Crumb {
	label: string;
	to?: string;
}

// Group › section › page. The section links back to its list when the
// page is something under it (a site, a plugin, the asset templates).
const crumbs = computed<Crumb[]>(() => {
	const found = findNavItem(route.meta.nav);
	const title = pageTitle.value ?? route.meta.title;
	if (!found) return title ? [{ label: title }] : [];
	const list: Crumb[] = [];
	if (found.group?.label) list.push({ label: found.group.label });
	if (title) {
		list.push({ label: found.item.label, to: found.item.to });
		list.push({ label: title });
	} else {
		list.push({ label: found.item.label });
	}
	return list;
});

const levelLabels: Record<string, string> = {
	execute: "Execute",
	admin: "Admin",
};

// Explains why the session has fewer privileges than the user's account.
const restrictedNotice = computed(() => {
	const from = authStore.restrictedFrom;
	if (!from) return null;
	return `Two-factor authentication is not enabled, so your ${levelLabels[from] ?? from} privileges are limited to Edit. Enable 2FA under Settings → Account.`;
});

const initials = computed(() => {
	const name = authStore.user?.displayName ?? "";
	const parts = name.trim().split(/\s+/).filter(Boolean);
	const first = parts[0]?.[0] ?? "?";
	const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
	return (first + last).toUpperCase();
});

const menuOpen = ref(false);
const menuRoot = ref<HTMLElement | null>(null);

const onDocumentClick = (e: MouseEvent) => {
	if (menuRoot.value && !menuRoot.value.contains(e.target as Node)) {
		menuOpen.value = false;
	}
};
const onKeydown = (e: KeyboardEvent) => {
	if (e.key === "Escape") menuOpen.value = false;
};

watch(
	() => route.fullPath,
	() => (menuOpen.value = false),
);
onMounted(() => {
	document.addEventListener("click", onDocumentClick);
	document.addEventListener("keydown", onKeydown);
});
onUnmounted(() => {
	document.removeEventListener("click", onDocumentClick);
	document.removeEventListener("keydown", onKeydown);
});

const handleLogout = () => {
	menuOpen.value = false;
	emit("logout");
};
</script>

<template>
	<header class="app-topbar">
		<button
			class="icon-btn topbar-menu"
			title="Open menu"
			aria-controls="app-sidebar"
			:aria-expanded="mobileOpen"
			@click="openMobile"
		>
			<AppIcon name="menu" size="22" />
			<span v-if="monitoringCount > 0" class="topbar-menu-dot"></span>
		</button>

		<nav class="breadcrumbs" aria-label="Breadcrumb">
			<template v-for="(crumb, i) in crumbs" :key="i">
				<AppIcon
					v-if="i > 0"
					name="chevron-right"
					size="14"
					class="breadcrumb-sep"
				/>
				<RouterLink v-if="crumb.to" :to="crumb.to" class="breadcrumb">
					{{ crumb.label }}
				</RouterLink>
				<span
					v-else
					class="breadcrumb"
					:class="{ current: i === crumbs.length - 1 }"
				>
					{{ crumb.label }}
				</span>
			</template>
		</nav>

		<div class="topbar-actions">
			<button
				class="icon-btn"
				:disabled="dataStore.isLoading"
				title="Refresh data"
				@click="dataStore.refreshData()"
			>
				<AppIcon v-if="!dataStore.isLoading" name="refresh" size="18" />
				<LoadingSpinner v-else small />
			</button>

			<div ref="menuRoot" class="user-menu">
				<button
					class="user-menu-toggle"
					:class="{ restricted: restrictedNotice }"
					:title="restrictedNotice ?? undefined"
					aria-haspopup="menu"
					:aria-expanded="menuOpen"
					@click="menuOpen = !menuOpen"
				>
					<span class="user-avatar">{{ initials }}</span>
					<span class="user-name hide-mobile">
						{{ authStore.user?.displayName }}
					</span>
					<AppIcon
						v-if="restrictedNotice"
						name="alert"
						size="16"
						class="user-restricted-icon"
					/>
					<AppIcon
						name="chevron-down"
						size="14"
						class="hide-mobile"
					/>
				</button>
				<div v-if="menuOpen" class="user-menu-panel" role="menu">
					<div class="user-menu-name">
						{{ authStore.user?.displayName }}
					</div>
					<RouterLink
						v-if="restrictedNotice"
						to="/settings"
						class="nav-restricted-notice"
						role="menuitem"
					>
						<AppIcon name="alert" size="16" />
						<span>{{ restrictedNotice }}</span>
					</RouterLink>
					<RouterLink
						to="/settings"
						class="user-menu-item"
						role="menuitem"
					>
						<AppIcon name="settings" size="16" />
						Account settings
					</RouterLink>
					<button
						class="user-menu-item"
						role="menuitem"
						@click="handleLogout"
					>
						<AppIcon name="logout" size="16" />
						Log out
					</button>
				</div>
			</div>
		</div>
	</header>
</template>
