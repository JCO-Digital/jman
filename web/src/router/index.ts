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
			path: "/sites/:page(\\d+)?/:rowsPerPage(\\d+)?",
			name: "sites",
			component: () => import("../views/SitesView.vue"),
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
			path: "/organizations",
			name: "organizations",
			component: () => import("../views/OrganizationsView.vue"),
		},
		{
			path: "/organization/:id",
			name: "organization-detail",
			component: () => import("../views/OrganizationDetailView.vue"),
			props: true,
		},
		{
			path: "/inventory/:page(\\d+)?/:rowsPerPage(\\d+)?",
			name: "assets",
			component: () => import("../views/Assets/AssetsListView.vue"),
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
			path: "/inventory/templates",
			name: "asset-templates",
			component: () => import("../views/Assets/TemplatesView.vue"),
		},
		{
			path: "/inventory/payment-methods",
			name: "payment-methods",
			component: () => import("../views/Assets/PaymentMethodsView.vue"),
		},
		{
			path: "/tasks",
			name: "tasks",
			component: () => import("../views/TasksView.vue"),
		},
		{
			path: "/reports",
			name: "reports",
			component: () => import("../views/Reports/ReportsListView.vue"),
		},
		{
			path: "/reports/:id",
			name: "report-runner",
			component: () => import("../views/Reports/ReportRunnerView.vue"),
			props: true,
		},
		{
			path: "/",
			name: "home",
			component: () => import("../views/DashboardView.vue"),
		},
		{
			path: "/settings",
			name: "settings",
			component: () => import("../views/SettingsView.vue"),
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

	// Protected route: only authentication is checked here; role-based access
	// control is enforced inside each view via canEdit/canExecute/canAdmin.
	if (!authStore.isAuthenticated) {
		return { name: "login" };
	}

	return true;
});

export default router;
