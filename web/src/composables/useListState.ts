import {
	computed,
	onBeforeUnmount,
	ref,
	watch,
	type ComputedRef,
	type Ref,
} from "vue";
import { useRoute, useRouter, type RouteLocationRaw } from "vue-router";

/**
 * The state of a list view (search, sort, filters, page) kept in the URL's
 * query string, so reloads, links, browser back and the detail views' back
 * links all return to the same list:
 *
 *   /sites?q=shop&sort=plugins&dir=desc&page=2&per=100&env=staging
 *
 * Values that equal their defaults are left out, so the plain list is just
 * /sites. Query keys the list doesn't manage (e.g. /tasks?task=12, which
 * opens a task) are kept as they are.
 */

export type SortDir = "asc" | "desc";

/**
 * A filter's allowed values, or `true` for free text. The empty string is
 * always "no filter" and is left out of the URL.
 */
export type FilterDef = readonly string[] | true;

export interface ListStateOptions<
	S extends string,
	F extends Record<string, FilterDef>,
> {
	/**
	 * The keys the list sorts by, or a getter for lists whose columns are
	 * known only at runtime (a report's).
	 */
	sortKeys: readonly S[] | (() => readonly S[]);
	/** The default sort; "" keeps the items in the order they come in. */
	defaultSort: S | "";
	defaultDir?: SortDir;
	/** Keys that sort descending on their first click: counts, dates. */
	descFirst?: readonly S[];
	filters?: F;
	/**
	 * The sort a filter implies, e.g. an expiry filter sorts soonest
	 * first. Applies while the URL names no sort of its own.
	 */
	impliedSort?: (
		filters: FilterValues<F>,
	) => { sort: S; dir: SortDir } | null;
}

export type FilterValues<F> = { readonly [K in keyof F]: string };

export interface ListState<
	S extends string = string,
	F extends Record<string, FilterDef> = Record<string, FilterDef>,
> {
	/** The search box's text; the URL follows it after a pause. */
	search: Ref<string>;
	/** The search the list is showing: the one in the URL. */
	query: ComputedRef<string>;
	/** The sort key, or "" for the items' own order. */
	sort: ComputedRef<S | "">;
	dir: ComputedRef<SortDir>;
	page: ComputedRef<number>;
	perPage: ComputedRef<number>;
	filters: ComputedRef<FilterValues<F>>;
	/** True when a search or any filter narrows the list. */
	narrowed: ComputedRef<boolean>;
	toggleSort: (key: S) => void;
	setFilter: (name: keyof F & string, value: string) => void;
	clearFilters: () => void;
	setPage: (page: number) => void;
	setPerPage: (perPage: number) => void;
	/** The URL of this list with some state changed, for links. */
	location: (changes?: ListChanges<S, F>) => RouteLocationRaw;
}

export interface ListChanges<S extends string, F> {
	search?: string;
	sort?: S | "";
	dir?: SortDir;
	page?: number;
	perPage?: number;
	filters?: Partial<Record<keyof F, string>>;
}

export const PER_PAGE_OPTIONS = [25, 50, 100, 200] as const;
const DEFAULT_PER_PAGE = 50;
const PER_PAGE_KEY = "jman.perPage";
const SEARCH_DELAY_MS = 250;

const RESERVED = ["q", "sort", "dir", "page", "per"];

// The rows-per-page choice is a preference of this browser, used by every
// list whose URL doesn't say otherwise.
function storedPerPage(): number {
	try {
		const n = Number(localStorage.getItem(PER_PAGE_KEY));
		return PER_PAGE_OPTIONS.includes(n as never) ? n : DEFAULT_PER_PAGE;
	} catch {
		return DEFAULT_PER_PAGE;
	}
}

function storePerPage(n: number) {
	try {
		localStorage.setItem(PER_PAGE_KEY, String(n));
	} catch {
		// Not remembering it is fine.
	}
}

function queryString(v: unknown): string {
	if (Array.isArray(v)) v = v[0];
	return typeof v === "string" ? v : "";
}

function positiveInt(v: unknown): number | null {
	const s = queryString(v);
	if (!/^\d+$/.test(s)) return null;
	const n = parseInt(s, 10);
	return n > 0 ? n : null;
}

export function useListState<
	S extends string,
	F extends Record<string, FilterDef> = Record<string, never>,
>(options: ListStateOptions<S, F>): ListState<S, F> {
	const route = useRoute();
	const router = useRouter();
	const filterDefs = (options.filters ?? {}) as F;
	const defaultDir = options.defaultDir ?? "asc";

	for (const name of Object.keys(filterDefs)) {
		if (RESERVED.includes(name)) {
			throw new Error(`list filter "${name}" clashes with a list key`);
		}
	}

	const isSortKey = (key: string): key is S => {
		const keys =
			typeof options.sortKeys === "function"
				? options.sortKeys()
				: options.sortKeys;
		return keys.includes(key as S);
	};

	const firstDir = (key: S | ""): SortDir =>
		key === options.defaultSort
			? defaultDir
			: options.descFirst?.includes(key as S)
				? "desc"
				: "asc";

	const query = computed(() => queryString(route.query.q));

	const filters = computed(() => {
		const out: Record<string, string> = {};
		for (const [name, def] of Object.entries(filterDefs)) {
			const v = queryString(route.query[name]);
			out[name] =
				def === true || (def as readonly string[]).includes(v) ? v : "";
		}
		return out as FilterValues<F>;
	});

	// The sort when the URL names none: the filter's implied one, or the
	// list's default.
	const baseSort = computed<{ sort: S | ""; dir: SortDir }>(() => {
		const implied = options.impliedSort?.(filters.value);
		return implied ?? { sort: options.defaultSort, dir: defaultDir };
	});

	const sort = computed<S | "">(() => {
		const s = queryString(route.query.sort);
		return isSortKey(s) ? s : baseSort.value.sort;
	});

	const dir = computed<SortDir>(() => {
		const d = queryString(route.query.dir);
		if (d === "asc" || d === "desc") return d;
		return queryString(route.query.sort) &&
			sort.value !== baseSort.value.sort
			? firstDir(sort.value)
			: baseSort.value.dir;
	});

	const page = computed(() => positiveInt(route.query.page) ?? 1);

	const perPage = computed(() => {
		const n = positiveInt(route.query.per);
		return n && PER_PAGE_OPTIONS.includes(n as never) ? n : storedPerPage();
	});

	const narrowed = computed(
		() =>
			query.value !== "" ||
			Object.values(filters.value).some((v) => v !== ""),
	);

	const location = (changes: ListChanges<S, F> = {}): RouteLocationRaw => {
		const nextFilters = { ...filters.value, ...(changes.filters ?? {}) };
		const implied = options.impliedSort?.(nextFilters as FilterValues<F>);
		const base: { sort: S | ""; dir: SortDir } = implied ?? {
			sort: options.defaultSort,
			dir: defaultDir,
		};
		// A filter change resets a sort the filters implied (the URL has
		// none of its own) and applies the one the new filters imply.
		const urlHasSort = queryString(route.query.sort) !== "";
		const filterImpliesSort =
			changes.filters !== undefined &&
			changes.sort === undefined &&
			((implied !== null && implied !== undefined) || !urlHasSort);
		const nextSort = filterImpliesSort
			? base.sort
			: (changes.sort ?? sort.value);
		const nextDir = filterImpliesSort
			? base.dir
			: (changes.dir ?? dir.value);
		const nextPerPage = changes.perPage ?? perPage.value;

		const q: Record<string, string | undefined> = {
			...(route.query as Record<string, string>),
		};
		const search = changes.search ?? query.value;
		q.q = search || undefined;
		q.sort = nextSort !== base.sort ? nextSort || undefined : undefined;
		q.dir =
			nextDir !== (nextSort === base.sort ? base.dir : firstDir(nextSort))
				? nextDir
				: undefined;
		// Any change other than the page itself starts from the first page.
		const nextPage =
			changes.page ?? (Object.keys(changes).length ? 1 : page.value);
		q.page = nextPage > 1 ? String(nextPage) : undefined;
		q.per =
			nextPerPage !== storedPerPage() ? String(nextPerPage) : undefined;
		for (const name of Object.keys(filterDefs)) {
			q[name] =
				(nextFilters as Record<string, string>)[name] || undefined;
		}
		return {
			name: route.name ?? undefined,
			params: route.params,
			query: q,
		};
	};

	const toggleSort = (key: S) => {
		const nextDir =
			sort.value === key
				? dir.value === "asc"
					? "desc"
					: "asc"
				: firstDir(key);
		router.push(location({ sort: key, dir: nextDir }));
	};

	const setFilter = (name: keyof F & string, value: string) => {
		router.push(
			location({
				filters: { [name]: value } as Partial<Record<keyof F, string>>,
			}),
		);
	};

	const clearFilters = () => {
		const empty: Record<string, string> = {};
		for (const name of Object.keys(filterDefs)) empty[name] = "";
		search.value = "";
		router.push(
			location({
				search: "",
				filters: empty as Partial<Record<keyof F, string>>,
			}),
		);
	};

	const setPage = (n: number) =>
		router.push(location({ page: Math.max(1, n) }));

	const setPerPage = (n: number) => {
		storePerPage(n);
		router.push(location({ perPage: n }));
	};

	// Typing waits for a pause, then shows the first page of the results.
	// Starting a search (or clearing it) adds a history entry, so back
	// returns to the list as it was before; refining it replaces the URL,
	// so back doesn't step through every pause in the typing.
	const search = ref(query.value);
	let timer: ReturnType<typeof setTimeout> | undefined;
	watch(search, (value) => {
		clearTimeout(timer);
		if (value === query.value) return;
		timer = setTimeout(() => {
			const to = location({ search: value });
			if (query.value !== "" && value !== "") router.replace(to);
			else router.push(to);
		}, SEARCH_DELAY_MS);
	});
	// Back/forward to a URL with another search shows it in the box.
	watch(query, (value) => {
		if (value !== search.value) search.value = value;
	});
	onBeforeUnmount(() => clearTimeout(timer));

	return {
		search,
		query,
		sort,
		dir,
		page,
		perPage,
		filters,
		narrowed,
		toggleSort,
		setFilter,
		clearFilters,
		setPage,
		setPerPage,
		location,
	};
}
