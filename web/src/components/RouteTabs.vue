<script setup lang="ts">
import { RouterLink, type RouteLocationRaw } from "vue-router";

export interface RouteTab {
	key: string;
	label: string;
	to: RouteLocationRaw;
	/** A count shown after the label, e.g. open incidents. */
	count?: number;
}

// Tabs that are routes: each is a link, so tabs can be bookmarked and
// opened in a new tab. On a top-level page browser back returns to the
// previous tab. A detail page's tabs replace the history entry instead
// (`replace`), so its back link still finds the list it was opened from.
defineProps<{
	tabs: RouteTab[];
	active: string;
	label?: string;
	replace?: boolean;
}>();
</script>

<template>
	<nav class="route-tabs" :aria-label="label ?? 'Sections'">
		<RouterLink
			v-for="tab in tabs"
			:key="tab.key"
			:to="tab.to"
			:replace="replace"
			class="route-tab"
			:class="{ active: tab.key === active }"
			:aria-current="tab.key === active ? 'page' : undefined"
		>
			{{ tab.label }}
			<span v-if="tab.count" class="tab-badge">{{ tab.count }}</span>
		</RouterLink>
	</nav>
</template>
