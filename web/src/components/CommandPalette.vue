<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter, type RouteLocationRaw } from "vue-router";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useAuthStore } from "../stores/auth";
import { useCommandPalette } from "../composables/useCommandPalette";
import { navGroups, settingsNavItem } from "../navigation";
import { matchScore } from "../utils/fuzzy";
import type { Organization } from "../types";
import AppIcon from "./AppIcon.vue";

// The quick-jump: type part of a site's domain, a plugin, an organization
// or a page, and Enter opens it. Ctrl+K / ⌘K opens it from anywhere.

interface Item {
	id: string;
	label: string;
	detail?: string;
	icon: string;
	to: RouteLocationRaw;
}

interface Group {
	label: string;
	items: Item[];
}

const router = useRouter();
const dataStore = useDataStore();
const organizationStore = useOrganizationStore();
const authStore = useAuthStore();
const { open, show, hide } = useCommandPalette();

const query = ref("");
const activeIndex = ref(0);
const input = ref<HTMLInputElement | null>(null);
const listEl = ref<HTMLElement | null>(null);
const organizations = ref<Organization[]>([]);

const GROUP_LIMIT = 8;

// The sections and their sub-pages, so "sec" finds Monitoring › Security.
const pages = computed<Item[]>(() => {
	const items: Item[] = [];
	for (const group of navGroups) {
		for (const item of group.items) {
			items.push({
				id: `page-${item.key}`,
				label: item.label,
				detail: group.label,
				icon: item.icon,
				to: item.to,
			});
		}
	}
	items.push({
		id: "page-settings",
		label: settingsNavItem.label,
		icon: settingsNavItem.icon,
		to: settingsNavItem.to,
	});
	const sub = (
		parent: string,
		icon: string,
		entries: [string, RouteLocationRaw][],
	) => {
		for (const [label, to] of entries) {
			items.push({
				id: `page-${parent}-${label}`,
				label: `${parent} › ${label}`,
				icon,
				to,
			});
		}
	};
	sub("Monitoring", "activity", [
		["Uptime", "/monitoring/uptime"],
		["Uptime checks", "/monitoring/uptime/checks"],
		["Security", "/monitoring/security"],
		["Vulnerabilities", "/monitoring/vulnerabilities"],
	]);
	sub("Assets", "credit-card", [
		["Templates", "/assets/templates"],
		["Payment methods", "/assets/payment-methods"],
	]);
	const settings: [string, RouteLocationRaw][] = [
		["My account", "/settings"],
		["General", "/settings/general"],
		["CLI tokens", "/settings/cli-tokens"],
	];
	if (authStore.canEdit) {
		settings.push(
			["Ignore list", "/settings/ignored"],
			["Backports", "/settings/backports"],
			["Sites", "/settings/sites"],
		);
	}
	if (authStore.canAdmin) {
		settings.push(
			["Users", "/settings/users"],
			["Agent tokens", "/settings/agent-tokens"],
		);
	}
	sub(settingsNavItem.label, settingsNavItem.icon, settings);
	return items;
});

const sites = computed<Item[]>(() =>
	dataStore.enrichedSites.map((s) => ({
		id: `site-${s.id}`,
		label: s.domain,
		detail: [s.environment, s.server].filter(Boolean).join(" · "),
		icon: "site",
		to: { name: "site-detail", params: { id: s.id } },
	})),
);

const plugins = computed<Item[]>(() =>
	dataStore.enrichedPlugins.map((p) => ({
		id: `plugin-${p.slug}`,
		label: p.shortName || p.name,
		detail: `${p.slug} · ${p.count} ${p.count === 1 ? "site" : "sites"}`,
		icon: "plugin",
		to: { name: "plugin-detail", params: { name: p.slug } },
	})),
);

const orgItems = computed<Item[]>(() =>
	organizations.value.map((o) => ({
		id: `org-${o.id}`,
		label: o.name,
		detail: o.vat_number ?? undefined,
		icon: "organization",
		to: { name: "organization-detail", params: { id: o.id } },
	})),
);

// The best matches of each kind, and the best score among them. Plugins
// also match by slug.
function best(
	items: Item[],
	extra?: (item: Item) => string,
): { items: Item[]; score: number } {
	const q = query.value;
	const ranked = items
		.map((item) => {
			const scores = [matchScore(item.label, q)];
			if (extra) scores.push(matchScore(extra(item), q));
			const valid = scores.filter((s): s is number => s !== null);
			return { item, score: valid.length ? Math.min(...valid) : null };
		})
		.filter((r): r is { item: Item; score: number } => r.score !== null)
		.sort(
			(a, b) =>
				a.score - b.score || a.item.label.localeCompare(b.item.label),
		)
		.slice(0, GROUP_LIMIT);
	return {
		items: ranked.map((r) => r.item),
		score: ranked[0]?.score ?? Infinity,
	};
}

const groups = computed<Group[]>(() => {
	if (!query.value.trim()) {
		return [{ label: "Pages", items: pages.value.slice(0, 10) }];
	}
	// The kind with the best match comes first ("settings" puts the pages
	// on top); equal ones keep this order.
	return [
		{ label: "Sites", ...best(sites.value) },
		{ label: "Plugins", ...best(plugins.value, (i) => i.detail ?? "") },
		{ label: "Organizations", ...best(orgItems.value) },
		{ label: "Pages", ...best(pages.value) },
	]
		.filter((g) => g.items.length > 0)
		.sort((a, b) => a.score - b.score)
		.map(({ label, items }) => ({ label, items }));
});

const flat = computed(() => groups.value.flatMap((g) => g.items));

watch(query, () => (activeIndex.value = 0));

watch(open, async (isOpen) => {
	if (!isOpen) return;
	query.value = "";
	activeIndex.value = 0;
	await nextTick();
	input.value?.focus();
	organizationStore
		.listAllOrganizations()
		.then((list) => (organizations.value = list))
		.catch(() => {});
});

function go(item: Item, newTab = false) {
	if (newTab) {
		window.open(router.resolve(item.to).href, "_blank");
	} else {
		router.push(item.to);
	}
	hide();
}

function move(delta: number) {
	const n = flat.value.length;
	if (!n) return;
	activeIndex.value = (activeIndex.value + delta + n) % n;
	nextTick(() => {
		listEl.value
			?.querySelector(`[data-index="${activeIndex.value}"]`)
			?.scrollIntoView({ block: "nearest" });
	});
}

function onInputKeydown(e: KeyboardEvent) {
	if (e.key === "ArrowDown") {
		e.preventDefault();
		move(1);
	} else if (e.key === "ArrowUp") {
		e.preventDefault();
		move(-1);
	} else if (e.key === "Enter") {
		e.preventDefault();
		const item = flat.value[activeIndex.value];
		if (item) go(item, e.ctrlKey || e.metaKey);
	} else if (e.key === "Escape") {
		e.preventDefault();
		hide();
	}
}

function onLinkClick(e: MouseEvent, item: Item) {
	// Middle and modified clicks open a tab the browser's way.
	if (e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0) return;
	e.preventDefault();
	go(item);
}

function indexOf(item: Item): number {
	return flat.value.indexOf(item);
}

// Ctrl+K / ⌘K toggles it from anywhere.
function onGlobalKeydown(e: KeyboardEvent) {
	if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
		e.preventDefault();
		if (open.value) hide();
		else show();
	}
}

onMounted(() => document.addEventListener("keydown", onGlobalKeydown));
onUnmounted(() => document.removeEventListener("keydown", onGlobalKeydown));
</script>

<template>
	<Teleport to="body">
		<div v-if="open" class="palette-overlay" @mousedown.self="hide">
			<div
				class="palette"
				role="dialog"
				aria-modal="true"
				aria-label="Go to"
			>
				<label class="palette-search">
					<AppIcon name="search" size="18" />
					<input
						ref="input"
						v-model="query"
						type="text"
						placeholder="Go to a site, plugin, organization or page…"
						role="combobox"
						aria-expanded="true"
						aria-controls="palette-results"
						aria-autocomplete="list"
						:aria-activedescendant="
							flat[activeIndex]
								? `palette-${flat[activeIndex]!.id}`
								: undefined
						"
						autocomplete="off"
						spellcheck="false"
						@keydown="onInputKeydown"
					/>
					<kbd>Esc</kbd>
				</label>

				<div
					id="palette-results"
					ref="listEl"
					class="palette-results"
					role="listbox"
				>
					<template v-for="group in groups" :key="group.label">
						<div class="palette-group" role="presentation">
							{{ group.label }}
						</div>
						<a
							v-for="item in group.items"
							:id="`palette-${item.id}`"
							:key="item.id"
							:href="router.resolve(item.to).href"
							:data-index="indexOf(item)"
							class="palette-item"
							:class="{ active: indexOf(item) === activeIndex }"
							role="option"
							:aria-selected="indexOf(item) === activeIndex"
							@mousemove="activeIndex = indexOf(item)"
							@click="onLinkClick($event, item)"
						>
							<AppIcon :name="item.icon" size="16" />
							<span class="palette-label">{{ item.label }}</span>
							<span v-if="item.detail" class="palette-detail">
								{{ item.detail }}
							</span>
						</a>
					</template>
					<p v-if="flat.length === 0" class="palette-empty">
						Nothing matches "{{ query }}".
					</p>
				</div>

				<div class="palette-footer">
					<span><kbd>↑</kbd><kbd>↓</kbd> choose</span>
					<span><kbd>Enter</kbd> open</span>
					<span><kbd>Ctrl</kbd><kbd>Enter</kbd> new tab</span>
				</div>
			</div>
		</div>
	</Teleport>
</template>
