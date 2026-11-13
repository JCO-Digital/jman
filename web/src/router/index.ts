import {
	createRouter,
	createWebHistory,
	type RouteLocationNormalized,
	type RouteRecordRaw,
} from "vue-router";
import { useAuthStore } from "../stores/auth";
import { findNavItem, type NavKey } from "../navigation";
import { clearPageTitle, pageTitle } from "../composables/usePageTitle";

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

function asArray(v: string | string[] | undefined): string[] {
	return v === undefined ? [] : Array.isArray(v) ? v : [v];
}

// Monitoring's area and section were ?tab=<area>&section=<section>, and
// before the areas had sections ?tab=active|checks|history. Slack alerts
// still link /monitoring?tab=security&incident=<id>.
const legacyMonitoringAreas: Record<
	string,
	{ area: string; section?: string }
> = {
	uptime: { area: "uptime" },
	security: { area: "security" },
	vulnerabilities: { area: "vulnerabilities" },
	active: { area: "uptime" },
	checks: { area: "uptime", section: "checks" },
	history: { area: "uptime", section: "history" },
};

function legacyMonitoringTab(to: RouteLocationNormalized) {
	const tab = to.query.tab;
	if (typeof tab !== "string") return null;
	const { tab: _, section, ...query } = to.query;
	const legacy = legacyMonitoringAreas[tab];
	if (!legacy) return { name: "monitoring", query };
	const sec = typeof section === "string" ? section : legacy.section;
	return {
		name: "monitoring",
		params: { area: legacy.area, section: sec ?? "" },
		query,
	};
}

// URLs from before the tabs were routes and the lists shared ?q=: the
// location they now are, or null. These run on every navigation, not only
// when a route is entered, since links within a page use them too.
function legacyLocation(to: RouteLocationNormalized) {
	if (to.name === "monitoring") return legacyMonitoringTab(to);
	if (to.name === "settings" && typeof to.query.tab === "string") {
		const { tab, ...query } = to.query;
		return { name: "settings", params: { tab }, query };
	}
	if (to.name === "inventory" && typeof to.query.search === "string") {
		const { search, ...query } = to.query;
		return {
			name: "inventory",
			params: to.params,
			query: { ...query, q: search },
		};
	}
	return null;
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
			// /site/<id>[/<tab>]: the overview is the default and left out.
			path: "/site/:id/:tab(updates|users|traffic|ledger)?",
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
			// /organization/<id>[/<tab>]: the overview is the default.
			path: "/organization/:id/:tab(sites|assets)?",
			name: "organization-detail",
			meta: { nav: "organizations", title: "Organization" },
			component: () => import("../views/OrganizationDetailView.vue"),
			props: true,
		},
		{
			path: "/assets",
			name: "assets",
			meta: { nav: "assets", width: "wide" },
			component: () => import("../views/Assets/AssetsListView.vue"),
		},
		legacyPaging("/assets", "assets"),
		{
			path: "/assets/templates",
			name: "asset-templates",
			meta: { nav: "assets", width: "wide", title: "Templates" },
			component: () => import("../views/Assets/TemplatesView.vue"),
		},
		{
			path: "/assets/payment-methods",
			name: "payment-methods",
			meta: { nav: "assets", title: "Payment methods" },
			component: () => import("../views/Assets/PaymentMethodsView.vue"),
		},
		// The assets were under /billing before they had their own section.
		{
			path: "/billing/:rest(.*)*",
			redirect: (to) => ({
				path: [
					"/assets",
					...asArray(to.params.rest).filter(Boolean),
				].join("/"),
				query: to.query,
			}),
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
			// /monitoring[/<area>[/<section>]]: the summary is the default,
			// and each area's first section is left out too.
			path: "/monitoring/:area(uptime|security|vulnerabilities)?/:section?",
			name: "monitoring",
			meta: { nav: "monitoring", width: "wide" },
			component: () => import("../views/MonitoringView.vue"),
			props: true,
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
			path: "/settings/:tab?",
			name: "settings",
			meta: { nav: "settings" },
			component: () => import("../views/SettingsView.vue"),
			props: true,
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
	const legacy = legacyLocation(to);
	if (legacy) return legacy;

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
router.afterEach((to, from) => {
	// The page's own name (a site's domain) stays while only its tab or
	// query changes; a view on another page or item sets it again.
	if (to.name !== from.name || to.params.id !== from.params.id) {
		clearPageTitle();
	}
	const name =
		pageTitle.value ??
		to.meta.title ??
		findNavItem(to.meta.nav)?.item.label;
	document.title = name ? `${name} · jman` : "jman";
});

export default router;
