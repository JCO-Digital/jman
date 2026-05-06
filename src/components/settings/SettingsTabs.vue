<script setup lang="ts">
interface Tab {
	id: string;
	label: string;
}

const props = defineProps<{
	modelValue: string;
	showUsersTab: boolean;
}>();

const emit = defineEmits<{
	"update:modelValue": [value: string];
}>();

const tabs: Tab[] = [
	{ id: "general", label: "General" },
	{ id: "account", label: "My Account" },
	{ id: "users", label: "Users" },
];

function visibleTabs(): Tab[] {
	return tabs.filter((tab) => tab.id !== "users" || props.showUsersTab);
}

function selectTab(id: string) {
	emit("update:modelValue", id);
}
</script>

<template>
	<nav class="settings-tabs">
		<button
			v-for="tab in visibleTabs()"
			:key="tab.id"
			class="settings-tab"
			:class="{ active: modelValue === tab.id }"
			@click="selectTab(tab.id)"
		>
			{{ tab.label }}
		</button>
	</nav>
</template>

<style scoped>
.settings-tabs {
	display: flex;
	border-bottom: 1px solid var(--border-color);
	background: var(--bg-card);
	gap: 0;
}

.settings-tab {
	padding: 0.75rem 1.25rem;
	font-size: 0.95rem;
	font-weight: 500;
	border: none;
	border-bottom: 2px solid transparent;
	background: none;
	color: var(--text-muted);
	cursor: pointer;
	transition:
		color 0.2s,
		border-color 0.2s,
		background-color 0.2s;
}

.settings-tab:hover {
	background-color: var(--bg-hover);
	color: var(--text-heading);
}

.settings-tab.active {
	color: var(--text-heading);
	border-bottom-color: var(--primary);
}

@media (max-width: 600px) {
	.settings-tab {
		padding: 0.6rem 0.85rem;
		font-size: 0.85rem;
	}
}
</style>
