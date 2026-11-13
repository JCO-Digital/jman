<script setup lang="ts">
import { computed, watch } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import ViewHeader from "../components/ViewHeader.vue";
import RouteTabs, { type RouteTab } from "../components/RouteTabs.vue";
import GeneralSettings from "../components/settings/GeneralSettings.vue";
import IgnoreManager from "../components/settings/IgnoreManager.vue";
import BackportManager from "../components/settings/BackportManager.vue";
import AccountSettings from "../components/settings/AccountSettings.vue";
import UserManagement from "../components/settings/UserManagement.vue";
import AgentTokenManagement from "../components/settings/AgentTokenManagement.vue";
import SiteManagement from "../components/settings/SiteManagement.vue";
import CliTokenManagement from "../components/settings/CliTokenManagement.vue";

// /settings[/<tab>]: My Account is the default and left out.
const props = defineProps<{
	tab?: string;
}>();

const router = useRouter();
const authStore = useAuthStore();

const ALL_TABS: { key: string; label: string; level?: "edit" | "admin" }[] = [
	{ key: "account", label: "My Account" },
	{ key: "general", label: "General" },
	{ key: "cli-tokens", label: "CLI Tokens" },
	{ key: "ignored", label: "Ignore List", level: "edit" },
	{ key: "backports", label: "Backports", level: "edit" },
	{ key: "sites", label: "Sites", level: "edit" },
	{ key: "users", label: "Users", level: "admin" },
	{ key: "agent-tokens", label: "Agent Tokens", level: "admin" },
];

const tabs = computed<RouteTab[]>(() =>
	ALL_TABS.filter(
		(t) =>
			!t.level ||
			(t.level === "edit" ? authStore.canEdit : authStore.canAdmin),
	).map((t) => ({
		key: t.key,
		label: t.label,
		to: {
			name: "settings",
			params: { tab: t.key === "account" ? "" : t.key },
		},
	})),
);

const activeTab = computed(() => {
	const t = props.tab || "account";
	return tabs.value.some((x) => x.key === t) ? t : "account";
});

// A tab this session can't see (an old link, a lower level) opens the
// default one instead.
watch(
	() => props.tab && activeTab.value !== props.tab,
	(unknown) => {
		if (unknown) router.replace({ name: "settings" });
	},
	{ immediate: true },
);
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Settings">
			<template #tabs>
				<RouteTabs :tabs="tabs" :active="activeTab" label="Settings" />
			</template>
		</ViewHeader>

		<main class="content">
			<div class="settings-content">
				<GeneralSettings v-if="activeTab === 'general'" />
				<CliTokenManagement v-else-if="activeTab === 'cli-tokens'" />
				<IgnoreManager v-else-if="activeTab === 'ignored'" />
				<BackportManager
					v-else-if="activeTab === 'backports' && authStore.canEdit"
				/>
				<SiteManagement
					v-else-if="activeTab === 'sites' && authStore.canEdit"
				/>
				<AccountSettings v-else-if="activeTab === 'account'" />
				<UserManagement
					v-else-if="activeTab === 'users' && authStore.canAdmin"
				/>
				<AgentTokenManagement
					v-else-if="
						activeTab === 'agent-tokens' && authStore.canAdmin
					"
				/>
			</div>
		</main>
	</div>
</template>
