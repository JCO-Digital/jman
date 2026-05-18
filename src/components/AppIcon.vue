<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
	name: string;
	size?: string | number;
}>();

// Import all SVG icons from the assets folder as raw strings using Vite's ?raw query
const icons = import.meta.glob<string>("../assets/icons/*.svg", {
	eager: true,
	query: "?raw",
	import: "default",
});

const svgContent = computed(() => {
	const path = `../assets/icons/${props.name}.svg`;
	const content = icons[path];
	if (!content) {
		console.warn(`Icon "${props.name}" not found at ${path}`);
		return "";
	}
	return content;
});

const sizeStyle = computed(() => {
	const s = props.size || "1em";
	const value = typeof s === "number" ? `${s}px` : s;
	return {
		width: value,
		height: value,
	};
});
</script>

<template>
	<span
		class="app-icon"
		v-html="svgContent"
		:style="sizeStyle"
		role="img"
		:aria-label="name + ' icon'"
	></span>
</template>

<style scoped>
.app-icon {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	vertical-align: middle;
	line-height: 1;
}

/* Ensure the injected SVG fills the component dimensions */
.app-icon :deep(svg) {
	width: 100%;
	height: 100%;
	display: block;
}
</style>
