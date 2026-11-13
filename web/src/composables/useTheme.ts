import { computed, ref, watch } from "vue";

// The color theme: the system's (auto) or one chosen in the user menu,
// remembered in this browser. The choice is html[data-theme], which the
// light-dark() colors in variables.css follow; index.html applies a stored
// choice before the app loads, so the page doesn't flash the other theme.

export type ThemeChoice = "auto" | "light" | "dark";

const STORAGE_KEY = "jman.theme";

function readChoice(): ThemeChoice {
	try {
		const v = localStorage.getItem(STORAGE_KEY);
		return v === "light" || v === "dark" ? v : "auto";
	} catch {
		return "auto";
	}
}

const choice = ref<ThemeChoice>(readChoice());

const media = window.matchMedia("(prefers-color-scheme: dark)");
const systemDark = ref(media.matches);
media.addEventListener("change", (e) => (systemDark.value = e.matches));

watch(
	choice,
	(value) => {
		const root = document.documentElement;
		if (value === "auto") delete root.dataset.theme;
		else root.dataset.theme = value;
		try {
			if (value === "auto") localStorage.removeItem(STORAGE_KEY);
			else localStorage.setItem(STORAGE_KEY, value);
		} catch {
			// Not remembering it is fine.
		}
	},
	{ immediate: true },
);

/** Whether the page shows dark colors now. */
const isDark = computed(() =>
	choice.value === "auto" ? systemDark.value : choice.value === "dark",
);

export function useTheme() {
	return { choice, isDark };
}

/**
 * A color variable's current value as rgb(), for code that draws colors
 * itself (charts on a canvas). A variable holding light-dark(...) only
 * resolves on an element, so this asks a probe element for it.
 */
export function cssColor(name: string): string {
	const probe = document.createElement("span");
	probe.style.color = `var(${name})`;
	probe.style.display = "none";
	document.body.appendChild(probe);
	const color = getComputedStyle(probe).color;
	probe.remove();
	return color;
}

/** A color from cssColor() with an opacity, e.g. for a chart's fill. */
export function withAlpha(rgb: string, alpha: number): string {
	const m = rgb.match(/rgba?\(([^)]+)\)/);
	if (!m) return rgb;
	const [r, g, b] = m[1]!.split(/[\s,/]+/);
	return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}
