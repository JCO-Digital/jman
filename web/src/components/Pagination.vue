<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, type RouteLocationRaw } from "vue-router";
import { PER_PAGE_OPTIONS } from "../composables/useListState";

// The footer of a list: which rows are shown, the pages as links (so
// they open in new tabs and work with back) and the rows per page.
const props = defineProps<{
	list: {
		perPage: { value: number };
		setPerPage: (n: number) => void;
		location: (changes: { page: number }) => RouteLocationRaw;
	};
	/** The page shown, which can be less than the URL's past the end. */
	page: number;
	total: number;
}>();

const totalPages = computed(() =>
	Math.max(1, Math.ceil(props.total / props.list.perPage.value)),
);

const range = computed(() => {
	if (props.total === 0) return "No rows";
	const start = (props.page - 1) * props.list.perPage.value + 1;
	const end = Math.min(props.total, props.page * props.list.perPage.value);
	return `${start}–${end} of ${props.total}`;
});

// The first and last page, and two on each side of the current one, with
// gaps (null) between.
const pages = computed<(number | null)[]>(() => {
	const n = totalPages.value;
	const cur = props.page;
	const out: (number | null)[] = [];
	for (let p = 1; p <= n; p++) {
		if (p === 1 || p === n || Math.abs(p - cur) <= 2) out.push(p);
		else if (out[out.length - 1] !== null) out.push(null);
	}
	return out;
});

const onPerPage = (e: Event) => {
	props.list.setPerPage(parseInt((e.target as HTMLSelectElement).value, 10));
};
</script>

<template>
	<nav class="pagination" aria-label="Pages">
		<span class="pagination-range">{{ range }}</span>
		<div v-if="totalPages > 1" class="pagination-pages">
			<RouterLink
				v-if="page > 1"
				:to="list.location({ page: page - 1 })"
				class="page-link"
				aria-label="Previous page"
			>
				‹
			</RouterLink>
			<span v-else class="page-link disabled" aria-hidden="true">‹</span>
			<template v-for="(p, i) in pages" :key="p ?? `gap-${i}`">
				<span v-if="p === null" class="page-gap">…</span>
				<span
					v-else-if="p === page"
					class="page-link current"
					aria-current="page"
				>
					{{ p }}
				</span>
				<RouterLink
					v-else
					:to="list.location({ page: p })"
					class="page-link"
				>
					{{ p }}
				</RouterLink>
			</template>
			<RouterLink
				v-if="page < totalPages"
				:to="list.location({ page: page + 1 })"
				class="page-link"
				aria-label="Next page"
			>
				›
			</RouterLink>
			<span v-else class="page-link disabled" aria-hidden="true">›</span>
		</div>
		<label class="pagination-per-page">
			Rows
			<select :value="list.perPage.value" @change="onPerPage">
				<option v-for="n in PER_PAGE_OPTIONS" :key="n" :value="n">
					{{ n }}
				</option>
			</select>
		</label>
	</nav>
</template>
