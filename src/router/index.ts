import { createRouter, createWebHistory } from "vue-router";
import { useAuthStore } from "../stores/auth";

const router = createRouter({
	history: createWebHistory(import.meta.env.BASE_URL),
	routes: [
		{
			path: "/login",
			name: "login",
			component: () => import("../views/LoginView.vue"),
			meta: { public: true },
		},
		{
			path: "/site/:id",
			name: "site-detail",
			component: () => import("../views/SiteDetailView.vue"),
			props: true,
		},
		{
			path: "/plugins/:page(\\d+)?/:rowsPerPage(\\d+)?",
			name: "plugins",
			component: () => import("../views/PluginsView.vue"),
			props: (route) => ({
				page: route.params.page
					? parseInt(route.params.page as string, 10)
					: undefined,
				rowsPerPage: route.params.rowsPerPage
					? parseInt(route.params.rowsPerPage as string, 10)
					: undefined,
			}),
		},
		{
			path: "/plugin/:name",
			name: "plugin-detail",
			component: () => import("../views/PluginDetailView.vue"),
			props: true,
		},
		{
			path: "/:page(\\d+)?/:rowsPerPage(\\d+)?",
			name: "home",
			component: () => import("../views/HomeView.vue"),
			props: (route) => ({
				page: route.params.page
					? parseInt(route.params.page as string, 10)
					: undefined,
				rowsPerPage: route.params.rowsPerPage
					? parseInt(route.params.rowsPerPage as string, 10)
					: undefined,
			}),
		},
	],
});

router.beforeEach((to) => {
	const authStore = useAuthStore();

	if (to.meta.public) {
		// If already authenticated and going to login, redirect to home
		if (to.name === "login" && authStore.isAuthenticated) {
			return { name: "home" };
		}
		return true;
	}

	// Protected route: check authentication
	if (!authStore.isAuthenticated) {
		return { name: "login" };
	}

	return true;
});

export default router;
