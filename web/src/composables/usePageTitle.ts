import { ref, toValue, watchEffect, type MaybeRefOrGetter } from "vue";

// The name of the thing the current page shows (a site's domain), which the
// top bar's breadcrumbs and the browser tab use instead of the route's
// generic title. The router clears it on every navigation to another page.
export const pageTitle = ref<string | null>(null);

export function clearPageTitle() {
	pageTitle.value = null;
}

/** Names the current page after a value that can change (e.g. load). */
export function usePageTitle(
	title: MaybeRefOrGetter<string | null | undefined>,
) {
	watchEffect(() => {
		const t = toValue(title) || null;
		pageTitle.value = t;
		if (t) document.title = `${t} · jman`;
	});
}
