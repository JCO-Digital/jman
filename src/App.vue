<script setup lang="ts">
import { RouterView, RouterLink, useRoute } from "vue-router";
import { useData } from "./composables/useData";
import packageInfo from "../package.json";

const route = useRoute();
const { initData } = useData();
initData();

const version = packageInfo.version;
</script>

<template>
	<div class="app-container">
		<nav class="app-nav">
			<div class="nav-container">
				<RouterLink
					to="/"
					class="nav-item"
					:class="{
						active: route.name === 'home' || route.name === 'site-detail',
					}"
				>
					Sites
				</RouterLink>
				<RouterLink
					to="/plugins"
					class="nav-item"
					:class="{
						active: route.name === 'plugins' || route.name === 'plugin-detail',
					}"
				>
					Plugins
				</RouterLink>
			</div>
		</nav>
		<div class="main-content">
			<RouterView />
		</div>
		<footer class="app-footer">v{{ version }}</footer>
	</div>
</template>

<style>
/* Global styles are mostly in style.css, but we can add some app-level resets here */
html,
body {
	margin: 0;
	padding: 0;
	font-family:
		-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial,
		sans-serif;
	background-color: var(--bg-body);
	color: var(--text-main);
	min-height: 100vh;
}

.app-container {
	min-height: 100vh;
	width: 100%;
	display: flex;
	flex-direction: column;
}

.main-content {
	flex: 1;
	display: flex;
	flex-direction: column;
}

.app-nav {
	background-color: var(--bg-card);
	border-bottom: 1px solid var(--border-color);
	padding: 0 20px;
}

.nav-container {
	max-width: 1200px;
	margin: 0 auto;
	display: flex;
	gap: 24px;
}

.nav-item {
	padding: 16px 4px;
	color: var(--text-muted);
	text-decoration: none;
	font-weight: 500;
	font-size: 14px;
	border-bottom: 2px solid transparent;
	transition: all 0.2s ease;
}

.nav-item:hover {
	color: var(--text-main);
}

.nav-item.active {
	color: var(--primary);
	border-bottom-color: var(--primary);
}

.app-footer {
	margin-top: auto;
	padding: 1rem;
	text-align: center;
	font-size: 0.875rem;
	color: var(--text-muted, #888);
}

/* Reset some default Vite styling that might clash */
#app {
	max-width: none;
	margin: 0;
	padding: 0;
	text-align: left;
}

/* Basic resets */
* {
	box-sizing: border-box;
}

table {
	width: 100%;
	border-collapse: collapse;
}
</style>
