import {
	createRouter,
	createWebHistory,
	type RouteLocationNormalized,
	type RouteRecordRaw,
} from "vue-router";
import { useAuthStore } from "../stores/auth";
import { findNavItem, type NavKey } from "../navigation";

declare module "vue-router" {
	interface RouteMeta {
		/** Reachable without logging in. */
		public?: boolean;
		/** The sidebar item the route belongs to. */
		nav?: NavKey;
		/**
		 * How wide the page may grow: lists and dashboards use the screen
		 * (wide), detail pages are capped (default), forms are narrow.
		 */
		width?: "wide" | "default" | "narrow";
		/** The page's name when it isn't the nav item's label. */
		title?: string;
	}
}

// Lists used to keep the page and rows per page in the path (/sites/2/100);
// they are in the query now (?page=2&per=100), like the rest of a list's
// state. These keep the old URLs working.
function pagingQuery(to: Pick<RouteLocationNormalized, "params" | "query">) {
	const { page, rowsPerPage } = to.params;
	return {
		...to.query,
		page: page && page !== "1" ? String(page) : undefined,
		per: rowsPerPage ? String(rowsPerPage) : undefined,
	};
}

function legacyPaging(path: string, name: string): RouteRecordRaw {
	return {
		path: `${path}/:page(\\d+)/:rowsPerPage(\\d+)?`,
		redirect: (to) => ({ name, query: pagingQuery(to) }),
	};
}

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
			meta: { nav: "sites", title: "Site" },
			component: () => import("../views/SiteDetailView.vue"),
			props: true,
		},
		{
			path: "/plugins",
			name: "plugins",
			meta: { nav: "plugins", width: "wide" },
			component: () => import("../views/PluginsView.vue"),
		},
		legacyPaging("/plugins", "plugins"),
		{
			path: "/users",
			name: "users",
			meta: { nav: "users", width: "wide" },
			component: () => import("../views/UsersView.vue"),
		},
		legacyPaging("/users", "users"),
		{
			path: "/plugin/:name",
			name: "plugin-detail",
			meta: { nav: "plugins", title: "Plugin" },
			component: () => import("../views/PluginDetailView.vue"),
			props: true,
		},
		{
			path: "/sites",
			name: "sites",
			meta: { nav: "sites", width: "wide" },
			component: () => import("../views/SitesView.vue"),
		},
		legacyPaging("/sites", "sites"),
		{
			path: "/organizations",
			name: "organizations",
			meta: { nav: "organizations", width: "wide" },
			component: () => import("../views/OrganizationsView.vue"),
		},
		{
			path: "/organization/:id",
			name: "organization-detail",
			meta: { nav: "organizations", title: "Organization" },
			component: () => import("../views/OrganizationDetailView.vue"),
			props: true,
		},
		{
			path: "/billing",
			name: "assets",
			meta: { nav: "assets", width: "wide" },
			component: () => import("../views/Assets/AssetsListView.vue"),
		},
		legacyPaging("/billing", "assets"),
		{
			path: "/billing/templates",
			name: "asset-templates",
			meta: { nav: "assets", width: "wide", title: "Templates" },
			component: () => import("../views/Assets/TemplatesView.vue"),
		},
		{
			path: "/billing/payment-methods",
			name: "payment-methods",
			meta: { nav: "assets", title: "Payment methods" },
			component: () => import("../views/Assets/PaymentMethodsView.vue"),
		},
		{
			// /inventory[/<type>]: the type is lower-case letters, so it
			// matches neither an item's UUID nor the old numeric billing
			// URLs below. The rest of the list's state is in the query.
			path: "/inventory/:type([a-z]+)?",
			name: "inventory",
			meta: { nav: "inventory", width: "wide" },
			component: () => import("../views/Inventory/InventoryListView.vue"),
			props: (route) => ({
				type:
					route.params.type && route.params.type !== "all"
						? route.params.type
						: undefined,
			}),
			// The search was ?search= before the lists shared ?q=.
			beforeEnter: (to) => {
				if (typeof to.query.search !== "string") return true;
				const { search, ...query } = to.query;
				return { ...to, query: { ...query, q: search } };
			},
		},
		{
			path: "/inventory/:type([a-z]+)/:page(\\d+)/:rowsPerPage(\\d+)?",
			redirect: (to) => ({
				name: "inventory",
				params: {
					type: to.params.type === "all" ? "" : to.params.type,
				},
				query: pagingQuery(to),
			}),
		},
		// The billing assets used to live under /inventory: keep old
		// bookmarks working. Item IDs are UUIDs, so these can't be items.
		{ path: "/inventory/templates", redirect: { name: "asset-templates" } },
		{
			path: "/inventory/payment-methods",
			redirect: { name: "payment-methods" },
		},
		{
			path: "/inventory/:page(\\d+)/:rowsPerPage(\\d+)?",
			redirect: (to) => ({ name: "assets", query: pagingQuery(to) }),
		},
		{
			path: "/inventory/:id",
			name: "inventory-detail",
			meta: { nav: "inventory", title: "Item" },
			component: () =>
				import("../views/Inventory/InventoryDetailView.vue"),
			props: true,
		},
		{
			path: "/monitoring",
			name: "monitoring",
			meta: { nav: "monitoring", width: "wide" },
			component: () => import("../views/MonitoringView.vue"),
		},
		{
			path: "/incidents",
			redirect: (to) => ({ path: "/monitoring", query: to.query }),
		},
		{
			path: "/tasks",
			name: "tasks",
			meta: { nav: "tasks", width: "wide" },
			component: () => import("../views/TasksView.vue"),
		},
		{
			path: "/reports",
			name: "reports",
			meta: { nav: "reports" },
			component: () => import("../views/Reports/ReportsListView.vue"),
		},
		{
			path: "/reports/:id",
			name: "report-runner",
			meta: { nav: "reports", width: "wide", title: "Report" },
			component: () => import("../views/Reports/ReportRunnerView.vue"),
			props: true,
		},
		{
			path: "/",
			name: "home",
			meta: { nav: "dashboard", width: "wide" },
			component: () => import("../views/DashboardView.vue"),
		},
		{
			path: "/settings",
			name: "settings",
			meta: { nav: "settings" },
			component: () => import("../views/SettingsView.vue"),
		},
		{
			path: "/cli-login",
			name: "cli-login",
			meta: { width: "narrow", title: "CLI login" },
			component: () => import("../views/CliLoginView.vue"),
		},
	],
});

router.beforeEach(async (to) => {
	const authStore = useAuthStore();
	// Wait for the startup session restore before deciding.
	await authStore.ready;

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
		// Come back here after logging in, e.g. to approve a CLI login.
		const query = to.fullPath === "/" ? {} : { redirect: to.fullPath };
		return { name: "login", query };
	}

	return true;
});

// The tab title follows the page; views with a better name (a site's
// domain) can set document.title themselves after this.
router.afterEach((to) => {
	const name = to.meta.title ?? findNavItem(to.meta.nav)?.item.label;
	document.title = name ? `${name} · jman` : "jman";
});

export default router;
