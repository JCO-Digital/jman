<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import ViewHeader from "../components/ViewHeader.vue";
import SettingsTabs from "../components/settings/SettingsTabs.vue";
import GeneralSettings from "../components/settings/GeneralSettings.vue";
import IgnoreManager from "../components/settings/IgnoreManager.vue";
import AccountSettings from "../components/settings/AccountSettings.vue";
import UserManagement from "../components/settings/UserManagement.vue";

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();

const validTabs = computed(() => {
	const tabs = ["account", "general"];
	if (authStore.canEdit) tabs.push("ignored");
	if (authStore.canAdmin) tabs.push("users");
	return tabs;
});

const activeTab = ref(getInitialTab());

function getInitialTab(): string {
	const tab = route.query.tab as string;
	if (tab && validTabs.value.includes(tab)) return tab;
	return "account";
}

// Sync tab to URL query parameter
watch(activeTab, (newTab) => {
	router.replace({ query: { ...route.query, tab: newTab } });
});

// React to URL changes (e.g. browser back/forward)
watch(
	() => route.query.tab,
	(newTab) => {
		if (typeof newTab === "string" && validTabs.value.includes(newTab)) {
			activeTab.value = newTab;
		}
	},
);
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Settings" />

		<main class="content mt-4">
			<SettingsTabs
				v-model="activeTab"
				:show-users-tab="authStore.canAdmin"
				:show-ignored-tab="authStore.canEdit"
			/>

			<div class="settings-content">
				<GeneralSettings v-if="activeTab === 'general'" />
				<IgnoreManager v-else-if="activeTab === 'ignored'" />
				<AccountSettings v-else-if="activeTab === 'account'" />
				<UserManagement
					v-else-if="activeTab === 'users' && authStore.canAdmin"
				/>
			</div>
		</main>
	</div>
</template>
