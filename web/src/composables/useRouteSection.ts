import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

// Query keys that belong to one section's list (its page, an opened
// incident), dropped when another section is chosen.
const SECTION_QUERY = ["page", "incident"];

/**
 * The current section of a Monitoring area, kept in the path
 * (/monitoring/<area>/<section>) so it survives reloads, can be linked to,
 * and browser back returns to the previous section. The default section
 * is left out of the URL.
 */
export function useRouteSection<T extends string>(
	keys: readonly T[],
	fallback: T,
) {
	const route = useRoute();
	const router = useRouter();

	return computed<T>({
		get() {
			const s = route.params.section;
			return typeof s === "string" && keys.includes(s as T)
				? (s as T)
				: fallback;
		},
		set(value) {
			const query = { ...route.query };
			for (const k of SECTION_QUERY) delete query[k];
			router.push({
				params: {
					...route.params,
					section: value === fallback ? "" : value,
				},
				query,
			});
		},
	});
}
