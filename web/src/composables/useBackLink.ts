import { computed, toValue, type MaybeRefOrGetter } from "vue";
import { useRouter, type RouteLocationRaw } from "vue-router";

/**
 * A detail page's way back. When the previous history entry is one of the
 * pages it leads back to (e.g. the list the item was opened from), going
 * back pops that entry, so the list returns as it was left: same search,
 * filters, sort and page, which are all in its URL. Reached any other way
 * (a link from Slack, a reload, another section), it opens the fallback.
 */
export function useBackLink(
	fallback: MaybeRefOrGetter<RouteLocationRaw>,
	/** Route names that count as "where we came from"; default: the fallback's. */
	from?: readonly string[],
) {
	const router = useRouter();

	const names = computed(() => {
		if (from) return from;
		const name = router.resolve(toValue(fallback)).name;
		return name ? [String(name)] : [];
	});

	/** True when going back would return to one of the expected pages. */
	function canPop(): boolean {
		const back = (window.history.state as { back?: unknown } | null)?.back;
		if (typeof back !== "string") return false;
		const name = router.resolve(back).name;
		return name != null && names.value.includes(String(name));
	}

	function go() {
		if (canPop()) router.back();
		else router.push(toValue(fallback));
	}

	return { go, to: computed(() => toValue(fallback)) };
}
