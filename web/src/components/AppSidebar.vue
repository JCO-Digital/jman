<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from "vue";
import { RouterLink, useRoute } from "vue-router";
import { useIncidentStore } from "../stores/incidents";
import { useVulnStore } from "../stores/vulns";
import { useSidebar } from "../composables/useSidebar";
import { navGroups, settingsNavItem, type NavItem } from "../navigation";
import AppIcon from "./AppIcon.vue";
import packageInfo from "../../package.json";

const route = useRoute();
const incidentStore = useIncidentStore();
const vulnStore = useVulnStore();
const { collapsed, mobileOpen, toggleCollapsed, closeMobile } = useSidebar();

const version = packageInfo.version;

/** Open incidents plus unmuted overdue vulnerability exposures. */
const monitoringCount = computed(
	() => incidentStore.activeCount + (vulnStore.summary?.overdue ?? 0),
);

const isActive = (item: NavItem) => route.meta.nav === item.key;

const badge = (item: NavItem) =>
	item.monitoringBadge && monitoringCount.value > 0
		? monitoringCount.value
		: 0;

// The rail shows only icons, so the label (and count) is the tooltip.
const tooltip = (item: NavItem) => {
	if (!collapsed.value) return undefined;
	const n = badge(item);
	return n ? `${item.label} (${n})` : item.label;
};

watch(() => route.fullPath, closeMobile);

const onKeydown = (e: KeyboardEvent) => {
	if (e.key === "Escape") closeMobile();
};
onMounted(() => document.addEventListener("keydown", onKeydown));
onUnmounted(() => {
	document.removeEventListener("keydown", onKeydown);
	closeMobile();
});
</script>

<template>
	<Transition name="nav-fade">
		<div
			v-if="mobileOpen"
			class="sidebar-backdrop"
			@click="closeMobile"
		></div>
	</Transition>
	<aside
		id="app-sidebar"
		class="app-sidebar"
		:class="{ 'is-open': mobileOpen }"
		aria-label="Main navigation"
	>
		<div class="sidebar-brand">
			<RouterLink to="/" class="sidebar-logo" title="Dashboard">
				<span class="sidebar-logo-mark">j</span>
				<span class="sidebar-label">jman</span>
			</RouterLink>
			<button
				class="sidebar-close"
				title="Close menu"
				@click="closeMobile"
			>
				<AppIcon name="x" size="20" />
			</button>
		</div>

		<nav class="sidebar-nav">
			<div
				v-for="(group, i) in navGroups"
				:key="group.label ?? i"
				class="sidebar-group"
			>
				<div v-if="group.label" class="sidebar-group-label">
					<span class="sidebar-label">{{ group.label }}</span>
				</div>
				<RouterLink
					v-for="item in group.items"
					:key="item.key"
					:to="item.to"
					class="sidebar-item"
					:class="{ active: isActive(item) }"
					:title="tooltip(item)"
					:aria-current="isActive(item) ? 'page' : undefined"
				>
					<AppIcon :name="item.icon" size="18" />
					<span class="sidebar-label">{{ item.label }}</span>
					<span v-if="badge(item)" class="sidebar-badge">
						{{ badge(item) }}
					</span>
				</RouterLink>
			</div>
		</nav>

		<div class="sidebar-footer">
			<RouterLink
				:to="settingsNavItem.to"
				class="sidebar-item"
				:class="{ active: isActive(settingsNavItem) }"
				:title="tooltip(settingsNavItem)"
			>
				<AppIcon :name="settingsNavItem.icon" size="18" />
				<span class="sidebar-label">{{ settingsNavItem.label }}</span>
			</RouterLink>
			<button
				class="sidebar-item sidebar-collapse"
				:title="collapsed ? 'Expand sidebar' : 'Collapse sidebar'"
				:aria-expanded="!collapsed"
				@click="toggleCollapsed"
			>
				<AppIcon name="sidebar" size="18" />
				<span class="sidebar-label">Collapse</span>
			</button>
			<div class="sidebar-version">
				<span class="sidebar-label">v{{ version }}</span>
			</div>
		</div>
	</aside>
</template>
