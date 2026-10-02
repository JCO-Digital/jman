import { onUnmounted, ref, watch, type Ref } from "vue";

/**
 * A reactive Date.now() that ticks every intervalMs while active is true,
 * for elapsed and relative times.
 */
export function useNow(active: Ref<boolean>, intervalMs = 1000): Ref<number> {
	const now = ref(Date.now());
	let timer: ReturnType<typeof setInterval> | null = null;

	const stop = () => {
		if (timer) clearInterval(timer);
		timer = null;
	};

	watch(
		active,
		(on) => {
			stop();
			now.value = Date.now();
			if (on)
				timer = setInterval(() => (now.value = Date.now()), intervalMs);
		},
		{ immediate: true },
	);
	onUnmounted(stop);
	return now;
}
