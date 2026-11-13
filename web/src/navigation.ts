// The main navigation: the sidebar groups and their items. A route says
// which item it belongs to with meta.nav, which drives the active item,
// the breadcrumbs and the page title.

export type NavKey =
	| "dashboard"
	| "sites"
	| "plugins"
	| "users"
	| "monitoring"
	| "tasks"
	| "organizations"
	| "assets"
	| "inventory"
	| "reports"
	| "settings";

export interface NavItem {
	key: NavKey;
	label: string;
	to: string;
	icon: string;
	/** Shows the count of open incidents and overdue vulnerabilities. */
	monitoringBadge?: boolean;
}

export interface NavGroup {
	/** Shown above the group's items; the first group has none. */
	label?: string;
	items: NavItem[];
}

export const navGroups: NavGroup[] = [
	{
		items: [
			{
				key: "dashboard",
				label: "Dashboard",
				to: "/",
				icon: "dashboard",
			},
		],
	},
	{
		label: "Technical",
		items: [
			{ key: "sites", label: "Sites", to: "/sites", icon: "site" },
			{
				key: "plugins",
				label: "Plugins",
				to: "/plugins",
				icon: "plugin",
			},
			{ key: "users", label: "Users", to: "/users", icon: "users" },
			{
				key: "monitoring",
				label: "Monitoring",
				to: "/monitoring",
				icon: "activity",
				monitoringBadge: true,
			},
		],
	},
	{
		label: "Organisational",
		items: [
			{ key: "tasks", label: "Tasks", to: "/tasks", icon: "task" },
			{
				key: "organizations",
				label: "Organizations",
				to: "/organizations",
				icon: "organization",
			},
			{
				key: "assets",
				label: "Assets",
				to: "/assets",
				icon: "credit-card",
			},
			{
				key: "inventory",
				label: "Inventory",
				to: "/inventory",
				icon: "box",
			},
			{
				key: "reports",
				label: "Reports",
				to: "/reports",
				icon: "report",
			},
		],
	},
];

export const settingsNavItem: NavItem = {
	key: "settings",
	label: "Settings",
	to: "/settings",
	icon: "settings",
};

/** The item with this key and the group it is in. */
export function findNavItem(
	key: NavKey | undefined,
): { item: NavItem; group?: NavGroup } | undefined {
	if (!key) return undefined;
	if (key === settingsNavItem.key) return { item: settingsNavItem };
	for (const group of navGroups) {
		const item = group.items.find((i) => i.key === key);
		if (item) return { item, group };
	}
	return undefined;
}
