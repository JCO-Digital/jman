import { computed, toValue, type MaybeRefOrGetter } from "vue";
import type { ListState } from "./useListState";

export type SortValue = string | number | boolean | null | undefined;

export interface ClientListOptions<T, S extends string> {
	/** Whether the item matches the search, given in lower case. */
	search?: (item: T, query: string) => boolean;
	/** Whether the item passes the list's filters. */
	filter?: (item: T) => boolean;
	/** The value a column sorts by. Empty values sort last either way. */
	sortValue: (item: T, key: S) => SortValue;
}

const collator = new Intl.Collator(undefined, {
	numeric: true,
	sensitivity: "base",
});

function isEmpty(v: SortValue): boolean {
	return v === null || v === undefined || v === "";
}

function compare(a: SortValue, b: SortValue): number {
	if (typeof a === "string" && typeof b === "string") {
		return collator.compare(a, b);
	}
	const x = Number(a);
	const y = Number(b);
	return x < y ? -1 : x > y ? 1 : 0;
}

/**
 * Searches, filters, sorts and pages a list held in memory, following a
 * list state from useListState.
 */
export function useClientList<T, S extends string>(
	items: MaybeRefOrGetter<readonly T[]>,
	list: Pick<ListState<S>, "query" | "sort" | "dir" | "page" | "perPage">,
	options: ClientListOptions<T, S>,
) {
	const matching = computed(() => {
		let result = toValue(items);
		const q = list.query.value.trim().toLowerCase();
		if (q && options.search) {
			result = result.filter((item) => options.search!(item, q));
		}
		if (options.filter) result = result.filter(options.filter);
		return result;
	});

	const sorted = computed(() => {
		const key = list.sort.value;
		if (key === "") return matching.value;
		const sign = list.dir.value === "asc" ? 1 : -1;
		// Sorting decorated pairs computes each value once, and the index
		// keeps equal items in their original order.
		return matching.value
			.map((item, i) => ({ item, i, v: options.sortValue(item, key) }))
			.sort((a, b) => {
				const ea = isEmpty(a.v);
				const eb = isEmpty(b.v);
				if (ea || eb) return ea === eb ? a.i - b.i : ea ? 1 : -1;
				return sign * compare(a.v, b.v) || a.i - b.i;
			})
			.map((d) => d.item);
	});

	const total = computed(() => sorted.value.length);
	const totalPages = computed(() =>
		Math.max(1, Math.ceil(total.value / list.perPage.value)),
	);
	// A page past the end (the list shrank since the link was made) shows
	// the last page.
	const page = computed(() => Math.min(list.page.value, totalPages.value));
	const pageItems = computed(() => {
		const start = (page.value - 1) * list.perPage.value;
		return sorted.value.slice(start, start + list.perPage.value);
	});

	return { matching, sorted, total, totalPages, page, pageItems };
}
