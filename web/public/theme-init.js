// The chosen color theme, applied before the app loads so the page doesn't
// flash the other one (see src/composables/useTheme.ts). Loaded from
// index.html as a file because the CSP (default-src 'self') blocks inline
// scripts.
try {
	var t = localStorage.getItem("jman.theme");
	if (t === "light" || t === "dark")
		document.documentElement.dataset.theme = t;
} catch (e) {}
