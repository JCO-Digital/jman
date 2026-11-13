import { ref, watch } from "vue";

const STORAGE_KEY = "jman.sidebarCollapsed";

function readCollapsed(): boolean {
	try {
		return localStorage.getItem(STORAGE_KEY) === "1";
	} catch {
		return false;
	}
}

// Shared by the sidebar and the top bar. On wide screens the sidebar can be
// collapsed to an icon rail, which is remembered in this browser; on narrow
// screens it is a drawer that the top bar's menu button opens.
const collapsed = ref(readCollapsed());
const mobileOpen = ref(false);

watch(collapsed, (value) => {
	try {
		localStorage.setItem(STORAGE_KEY, value ? "1" : "0");
	} catch {
		// Storage can be unavailable (private windows); not remembering is fine.
	}
});

watch(mobileOpen, (open) => {
	document.body.classList.toggle("nav-open", open);
});

export function useSidebar() {
	return {
		collapsed,
		mobileOpen,
		toggleCollapsed: () => (collapsed.value = !collapsed.value),
		openMobile: () => (mobileOpen.value = true),
		closeMobile: () => (mobileOpen.value = false),
	};
}
