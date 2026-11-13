<script setup lang="ts" generic="S extends string">
import { computed } from "vue";
import type { ListState } from "../composables/useListState";

// A sortable column header for a list from useListState. The header is a
// button, so it can be reached and sorted from the keyboard.
const props = defineProps<{
	list: Pick<ListState<S>, "sort" | "dir" | "toggleSort">;
	sortKey: S;
}>();

const active = computed(() => props.list.sort.value === props.sortKey);
const ariaSort = computed(() =>
	active.value
		? props.list.dir.value === "asc"
			? "ascending"
			: "descending"
		: undefined,
);
</script>

<template>
	<th :aria-sort="ariaSort" class="th-sortable" :class="{ sorted: active }">
		<button type="button" class="th-sort" @click="list.toggleSort(sortKey)">
			<slot />
			<span class="th-sort-indicator" aria-hidden="true">
				{{ active ? (list.dir.value === "asc" ? "↑" : "↓") : "↕" }}
			</span>
		</button>
	</th>
</template>
