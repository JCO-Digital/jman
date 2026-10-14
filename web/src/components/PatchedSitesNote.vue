<script setup lang="ts">
import { computed } from "vue";
import { useDataStore } from "../stores/data";
import { useAuthStore } from "../stores/auth";
import type { VulnerabilitySite } from "../types";

/**
 * Lists the sites a backport rule marks as not affected by a vulnerability,
 * shown alongside the sites that still are.
 */
const props = defineProps<{
	sites: VulnerabilitySite[];
}>();

const dataStore = useDataStore();
const authStore = useAuthStore();

const patched = computed(() =>
	props.sites
		.map((s) => ({
			id: s.site_id,
			domain: dataStore.getSiteById(s.site_id)?.domain || s.site_name,
			version: s.version,
		}))
		.sort((a, b) => a.domain.localeCompare(b.domain)),
);
</script>

<template>
	<div class="mt-2">
		<div class="sub-text font-xs font-medium">
			Patched via backport rule ({{ patched.length }})
		</div>
		<div class="flex-row gap-2 mt-1">
			<span v-for="site in patched" :key="site.id" class="font-xs">
				<router-link
					:to="{ name: 'site-detail', params: { id: site.id } }"
					class="font-medium"
				>
					{{ site.domain }}
				</router-link>
				<span class="sub-text"> v{{ site.version }}</span>
			</span>
			<router-link
				v-if="authStore.canEdit"
				:to="{ name: 'settings', query: { tab: 'backports' } }"
				class="font-xs"
			>
				Manage rules
			</router-link>
		</div>
	</div>
</template>
