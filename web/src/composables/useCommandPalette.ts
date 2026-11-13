import { ref } from "vue";

// Whether the quick-jump (CommandPalette) is open. Ctrl+K / ⌘K and the top
// bar's search button open it.
const open = ref(false);

export function useCommandPalette() {
	return {
		open,
		show: () => (open.value = true),
		hide: () => (open.value = false),
	};
}
