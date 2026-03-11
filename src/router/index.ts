import { createRouter, createWebHistory } from "vue-router";

const router = createRouter({
	history: createWebHistory(import.meta.env.BASE_URL),
	routes: [
		{
			path: "/",
			name: "home",
			component: () => import("../views/HomeView.vue"),
		},
		{
			path: "/site/:id",
			name: "site-detail",
			component: () => import("../views/SiteDetailView.vue"),
			props: true,
		},
		{
			path: "/plugins",
			name: "plugins",
			component: () => import("../views/PluginsView.vue"),
		},
		{
			path: "/plugin/:name",
			name: "plugin-detail",
			component: () => import("../views/PluginDetailView.vue"),
			props: true,
		},
	],
});

export default router;
