<script setup lang="ts">
import type { Ref } from "vue";
import AppIcon from "./AppIcon.vue";

// The bar above a list: the search box, the list's filters (default
// slot) and its actions (the actions slot), which go to the right.
const props = defineProps<{
	list: {
		search: Ref<string>;
		narrowed: { value: boolean };
		clearFilters: () => void;
	};
	placeholder?: string;
	/** Hides the search box, for lists that are only filtered. */
	noSearch?: boolean;
}>();

// The list object is the same for the view's life, so its search ref can
// be taken once; the box writes to it and the list state follows.
const search = props.list.search;
</script>

<template>
	<div class="list-toolbar">
		<label v-if="!noSearch" class="list-search">
			<AppIcon name="search" size="16" class="list-search-icon" />
			<input
				v-model="search"
				type="search"
				:placeholder="placeholder ?? 'Search…'"
				:aria-label="placeholder ?? 'Search'"
			/>
		</label>
		<div v-if="$slots.default" class="list-filters">
			<slot />
		</div>
		<button
			v-if="list.narrowed.value"
			type="button"
			class="btn btn-text btn-sm"
			@click="list.clearFilters()"
		>
			Clear
		</button>
		<div v-if="$slots.actions" class="list-actions">
			<slot name="actions" />
		</div>
	</div>
</template>
