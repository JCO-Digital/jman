/**
 * A repeating task, run every `intervalMs` once started.
 *
 * With `pauseWhenHidden`, it doesn't run while the tab is hidden: a
 * background tab would otherwise keep reloading data no one sees, in every
 * open tab. When the tab is shown again, it runs at once if a run was due
 * meanwhile, then keeps its interval.
 */
export interface Poller {
	start(): void;
	stop(): void;
}

export function createPoller(
	run: () => unknown,
	intervalMs: () => number,
	options: { pauseWhenHidden?: boolean } = {},
): Poller {
	let timer: ReturnType<typeof setInterval> | null = null;
	let lastRun = 0;
	let missed = false;

	const hidden = () =>
		options.pauseWhenHidden === true &&
		typeof document !== "undefined" &&
		document.visibilityState === "hidden";

	const tick = () => {
		if (hidden()) {
			missed = true;
			return;
		}
		missed = false;
		lastRun = Date.now();
		void run();
	};

	const onVisibilityChange = () => {
		if (hidden() || timer === null) return;
		if (missed || Date.now() - lastRun >= intervalMs()) {
			tick();
			// Restart the interval so the next run is a full interval away.
			clearInterval(timer);
			timer = setInterval(tick, intervalMs());
		}
	};

	return {
		start() {
			this.stop();
			lastRun = Date.now();
			missed = false;
			timer = setInterval(tick, intervalMs());
			if (options.pauseWhenHidden) {
				document.addEventListener(
					"visibilitychange",
					onVisibilityChange,
				);
			}
		},
		stop() {
			if (timer !== null) {
				clearInterval(timer);
				timer = null;
			}
			if (options.pauseWhenHidden) {
				document.removeEventListener(
					"visibilitychange",
					onVisibilityChange,
				);
			}
		},
	};
}
