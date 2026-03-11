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
/* All global and shell styles have been moved to style.css */
</style>
