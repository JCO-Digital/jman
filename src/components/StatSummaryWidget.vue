<script setup lang="ts">
import { computed } from "vue";
import { RouterLink } from "vue-router";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useTaskStore } from "../stores/tasks";
import AppIcon from "./AppIcon.vue";

const dataStore = useDataStore();
const orgStore = useOrganizationStore();
const taskStore = useTaskStore();

const openTasksCount = computed(() => {
	return taskStore.tasks.filter(
		(t) => t.status === "pending" || t.status === "in_progress",
	).length;
});

const stats = computed(() => [
	{
		label: "Organizations",
		value: orgStore.organizations.length,
		icon: "organization",
		link: "/organizations",
	},
	{
		label: "Sites",
		value: dataStore.sites.length,
		icon: "site",
		link: "/sites",
	},
	{
		label: "Plugins",
		value: dataStore.pluginInfo.length,
		icon: "plugin",
		link: "/plugins",
	},
	{
		label: "Tasks",
		value: openTasksCount.value,
		icon: "task",
		link: "/tasks",
	},
	{
		label: "Vulnerabilities",
		value: dataStore.activeVulnerabilities.length,
		icon: "vulnerability",
		link: "/plugins", // Linking to plugins as vulnerabilities are plugin-based
		isError: dataStore.activeVulnerabilities.length > 0,
		loading: dataStore.isVulnsLoading,
	},
]);
</script>

<template>
	<div class="stat-summary-grid">
		<RouterLink
			v-for="stat in stats"
			:key="stat.label"
			:to="stat.link"
			class="stat-summary-item card clickable-card"
		>
			<div class="stat-icon">
				<AppIcon :name="stat.icon" size="20" />
			</div>
			<div class="stat-details">
				<div class="stat-value" :class="{ 'error-text': stat.isError }">
					<template v-if="stat.loading">...</template>
					<template v-else>{{ stat.value }}</template>
				</div>
				<div class="stat-label">{{ stat.label }}</div>
			</div>
		</RouterLink>
	</div>
</template>

<style scoped>
.stat-summary-grid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
	gap: var(--space-4);
}

.stat-summary-item {
	display: flex;
	align-items: center;
	padding: var(--space-5);
	gap: var(--space-4);
	margin-top: 0;
	text-decoration: none;
	transition: all 0.2s ease;
	color: inherit;
}

.clickable-card:hover {
	border-color: var(--primary);
	transform: translateY(-2px);
	box-shadow:
		0 4px 6px -1px rgba(0, 0, 0, 0.1),
		0 2px 4px -1px rgba(0, 0, 0, 0.06);
}

.stat-icon {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 40px;
	height: 40px;
	border-radius: 50%;
	background-color: var(--bg-main);
	color: var(--primary);
}

.stat-details {
	display: flex;
	flex-direction: column;
}

.stat-value {
	font-size: 1.25rem;
	font-weight: 700;
	line-height: 1.2;
}

.stat-label {
	font-size: var(--font-size-xs);
	color: var(--text-muted);
	text-transform: uppercase;
	letter-spacing: 0.025em;
}

@media (max-width: 640px) {
	.stat-summary-grid {
		grid-template-columns: repeat(2, 1fr);
	}
}
</style>
