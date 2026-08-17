<script setup lang="ts">
interface Tab {
	id: string;
	label: string;
}

const props = defineProps<{
	modelValue: string;
	showUsersTab: boolean;
	showIgnoredTab: boolean;
	showAgentTokensTab: boolean;
}>();

const emit = defineEmits<{
	"update:modelValue": [value: string];
}>();

const tabs: Tab[] = [
	{ id: "account", label: "My Account" },
	{ id: "general", label: "General" },
	{ id: "ignored", label: "Ignore List" },
	{ id: "users", label: "Users" },
	{ id: "agent-tokens", label: "Agent Tokens" },
];

function visibleTabs(): Tab[] {
	return tabs.filter((tab) => {
		if (tab.id === "users") return props.showUsersTab;
		if (tab.id === "ignored") return props.showIgnoredTab;
		if (tab.id === "agent-tokens") return props.showAgentTokensTab;
		return true;
	});
}

function selectTab(id: string) {
	emit("update:modelValue", id);
}
</script>

<template>
	<nav class="tabs">
		<button
			v-for="tab in visibleTabs()"
			:key="tab.id"
			class="tab"
			:class="{ active: modelValue === tab.id }"
			@click="selectTab(tab.id)"
		>
			{{ tab.label }}
		</button>
	</nav>
</template>

<style scoped>
/* Scoped styles removed in favor of global .tabs and .tab classes in components.css */
</style>
