import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

/**
 * The current section of a Monitoring area, kept in the URL as
 * ?section=<key> so it survives reloads and can be linked to. The default
 * section is left out of the URL.
 */
export function useRouteSection<T extends string>(
	keys: readonly T[],
	fallback: T,
) {
	const route = useRoute();
	const router = useRouter();

	return computed<T>({
		get() {
			const s = route.query.section;
			return typeof s === "string" && keys.includes(s as T)
				? (s as T)
				: fallback;
		},
		set(value) {
			router.replace({
				query: {
					...route.query,
					section: value === fallback ? undefined : value,
				},
			});
		},
	});
}
