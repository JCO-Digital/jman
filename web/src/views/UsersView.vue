<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import { useSiteUsersStore } from "../stores/siteUsers";
import type { SiteUser } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import ListToolbar from "../components/ListToolbar.vue";
import SortableTh from "../components/SortableTh.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import AppIcon from "../components/AppIcon.vue";
import SiteUserDeleteModal from "../components/SiteUserDeleteModal.vue";
import { useListState } from "../composables/useListState";
import { useClientList } from "../composables/useClientList";

type UserRow = SiteUser & { domain: string };

const router = useRouter();
const authStore = useAuthStore();
const dataStore = useDataStore();
const siteUsersStore = useSiteUsersStore();

onMounted(() => {
	dataStore.initData();
	siteUsersStore.fetchAll(true);
});

// /users?q=&sort=&dir=&page=&per=&role=&site=<id>. The site filter lets
// a site's page link to its users.
const list = useListState({
	sortKeys: [
		"domain",
		"login",
		"email",
		"display_name",
		"roles",
		"registered",
		"posts",
	],
	defaultSort: "domain",
	descFirst: ["registered", "posts"],
	filters: { role: true, site: true },
});

const filterSite = computed(() => list.filters.value.site);
const filterSiteDomain = computed(
	() => dataStore.getSiteById(filterSite.value)?.domain ?? filterSite.value,
);

const splitRoles = (roles: string) =>
	roles
		.split(",")
		.map((r) => r.trim())
		.filter(Boolean);

// Users of known sites only (as the site list does, deleted sites drop out),
// with the site's domain.
const usersWithSite = computed(() =>
	siteUsersStore.users.flatMap((u) => {
		if (filterSite.value && u.site_id !== filterSite.value) return [];
		const site = dataStore.getSiteById(u.site_id);
		return site ? [{ ...u, domain: site.domain }] : [];
	}),
);

// Users that sort the same stay in login order.
const usersByLogin = computed(() =>
	[...usersWithSite.value].sort((a, b) =>
		a.login.toLowerCase().localeCompare(b.login.toLowerCase()),
	),
);

const roleOptions = computed(() => {
	const roles = new Set<string>();
	for (const u of usersWithSite.value) {
		for (const r of splitRoles(u.roles)) roles.add(r);
	}
	return [...roles].sort();
});

const users = useClientList(usersByLogin, list, {
	search: (u, q) =>
		u.login.toLowerCase().includes(q) ||
		u.email.toLowerCase().includes(q) ||
		u.display_name.toLowerCase().includes(q) ||
		u.domain.toLowerCase().includes(q),
	filter: (u) =>
		!list.filters.value.role ||
		splitRoles(u.roles).includes(list.filters.value.role),
	sortValue: (u, key) => (key === "posts" ? u.post_count : u[key]),
});

const paginatedUsers = users.pageItems;

const knownSummaries = computed(() =>
	siteUsersStore.summaries.filter(
		(s) =>
			(!filterSite.value || s.site_id === filterSite.value) &&
			dataStore.getSiteById(s.site_id),
	),
);

const excludedTotals = computed(() => {
	const totals: Record<string, number> = {};
	for (const s of knownSummaries.value) {
		for (const [role, n] of Object.entries(s.excluded_counts)) {
			totals[role] = (totals[role] ?? 0) + n;
		}
	}
	return totals;
});

const truncatedSites = computed(() =>
	knownSummaries.value
		.filter((s) => s.truncated)
		.map((s) => dataStore.getSiteById(s.site_id)?.domain ?? s.site_id),
);

const formatCount = (n: number) => n.toLocaleString();

const excludedText = computed(() =>
	Object.entries(excludedTotals.value)
		.sort(([a], [b]) => a.localeCompare(b))
		.map(([role, n]) => `${formatCount(n)} ${role}${n === 1 ? "" : "s"}`)
		.join(", "),
);

// Selecting users to delete. Rows stay selected across filters and pages.
const selectMode = ref(false);
const selected = ref(new Map<string, UserRow>());
const showDeleteModal = ref(false);

const rowKey = (u: SiteUser) => `${u.site_id}-${u.id}`;

/** Why a user can't be deleted from here, or "" if it can. */
const deleteBlocker = (u: UserRow) => {
	const site = dataStore.getSiteById(u.site_id);
	if (site?.wp_flags?.is_multisite) {
		return "Deleting users on multisite sites isn't supported";
	}
	if (!site?.can_wp_cli) return "The site isn't reachable over WP-CLI";
	return "";
};

const selectableOnPage = computed(() =>
	paginatedUsers.value.filter((u) => !deleteBlocker(u)),
);

const allOnPageSelected = computed(
	() =>
		selectableOnPage.value.length > 0 &&
		selectableOnPage.value.every((u) => selected.value.has(rowKey(u))),
);

const toggleSelectAllOnPage = () => {
	const select = !allOnPageSelected.value;
	for (const u of selectableOnPage.value) {
		if (select) selected.value.set(rowKey(u), u);
		else selected.value.delete(rowKey(u));
	}
};

const toggleSelected = (u: UserRow) => {
	if (selected.value.has(rowKey(u))) selected.value.delete(rowKey(u));
	else selected.value.set(rowKey(u), u);
};

const toggleSelectMode = () => {
	selectMode.value = !selectMode.value;
	selected.value.clear();
};

const onDeletesQueued = (keys: string[], all: boolean) => {
	for (const k of keys) selected.value.delete(k);
	if (all) {
		showDeleteModal.value = false;
		selectMode.value = false;
	}
};

const goToSite = (id: string) => {
	router.push({ name: "site-detail", params: { id } });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="WordPress Users" />

		<div v-if="siteUsersStore.error" class="error-banner">
			<p>
				<strong>Error loading users:</strong>
				{{ siteUsersStore.error }}
			</p>
		</div>

		<ListToolbar
			:list="list"
			placeholder="Search by login, email, name or site"
		>
			<select
				:value="list.filters.value.role"
				aria-label="Role"
				@change="
					list.setFilter(
						'role',
						($event.target as HTMLSelectElement).value,
					)
				"
			>
				<option value="">All roles</option>
				<option v-for="role in roleOptions" :key="role" :value="role">
					{{ role }}
				</option>
			</select>

			<button
				v-if="filterSite"
				type="button"
				class="filter-chip"
				aria-pressed="true"
				title="Show users of all sites"
				@click="list.setFilter('site', '')"
			>
				{{ filterSiteDomain }}
				<AppIcon name="x" size="14" />
			</button>

			<template v-if="authStore.canExecute" #actions>
				<button
					type="button"
					class="btn btn-outline"
					@click="toggleSelectMode"
				>
					{{ selectMode ? "Cancel" : "Select" }}
				</button>
			</template>
		</ListToolbar>

		<div v-if="selectMode" class="card-muted batch-action-bar">
			<span class="font-medium">{{ selected.size }} selected</span>
			<button
				type="button"
				class="btn btn-danger btn-sm"
				:disabled="selected.size === 0"
				@click="showDeleteModal = true"
			>
				Delete…
			</button>
		</div>

		<p class="sub-text mb-4">
			{{ formatCount(usersWithSite.length) }} users on
			{{ knownSummaries.length }}
			{{ knownSummaries.length === 1 ? "site" : "sites" }}.
			<template v-if="excludedText">
				Not listed: {{ excludedText }}.
			</template>
			<template v-if="truncatedSites.length > 0">
				Only the first 1,000 users are listed for
				{{ truncatedSites.join(", ") }}.
			</template>
		</p>

		<main class="table-container">
			<table
				class="data-table"
				:class="selectMode ? 'hide-col-5-sm' : 'hide-col-4-sm'"
			>
				<thead>
					<tr>
						<th v-if="selectMode" class="col-narrow text-center">
							<input
								type="checkbox"
								:checked="allOnPageSelected"
								@click="toggleSelectAllOnPage"
							/>
						</th>
						<SortableTh
							:list="list"
							sort-key="domain"
							class="col-wide"
						>
							Site
						</SortableTh>
						<SortableTh :list="list" sort-key="login">
							Login
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="email"
							class="col-expand"
						>
							Email
						</SortableTh>
						<SortableTh :list="list" sort-key="display_name">
							Name
						</SortableTh>
						<SortableTh :list="list" sort-key="roles">
							Roles
						</SortableTh>
						<SortableTh :list="list" sort-key="registered">
							Registered
						</SortableTh>
						<SortableTh
							:list="list"
							sort-key="posts"
							class="text-right"
						>
							Posts
						</SortableTh>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							(siteUsersStore.isLoading || dataStore.isLoading) &&
							usersWithSite.length === 0
						"
					>
						<td :colspan="selectMode ? 8 : 7">
							<LoadingSpinner message="Loading users..." />
						</td>
					</tr>
					<tr v-else-if="paginatedUsers.length === 0">
						<td :colspan="selectMode ? 8 : 7" class="empty-state">
							<span v-if="list.narrowed.value"
								>No users found matching the filters.</span
							>
							<span v-else>No users collected yet.</span>
						</td>
					</tr>
					<tr
						v-for="user in paginatedUsers"
						:key="`${user.site_id}-${user.id}`"
						class="clickable-row"
						@click="goToSite(user.site_id)"
					>
						<td
							v-if="selectMode"
							class="col-narrow text-center"
							:title="deleteBlocker(user)"
							@click.stop
						>
							<input
								type="checkbox"
								:checked="selected.has(rowKey(user))"
								:disabled="!!deleteBlocker(user)"
								@change="toggleSelected(user)"
							/>
						</td>
						<td class="col-wide truncate">{{ user.domain }}</td>
						<td class="truncate font-medium">{{ user.login }}</td>
						<td class="col-expand truncate">{{ user.email }}</td>
						<td class="truncate">{{ user.display_name }}</td>
						<td>
							<span
								v-for="role in splitRoles(user.roles)"
								:key="role"
								class="status-badge badge-sm info"
							>
								{{ role }}
							</span>
						</td>
						<td class="text-muted truncate">
							{{ user.registered.slice(0, 10) }}
						</td>
						<td class="text-right">
							{{ user.post_count?.toLocaleString() ?? "?" }}
						</td>
					</tr>
				</tbody>
			</table>

			<Pagination
				:list="list"
				:page="users.page.value"
				:total="users.total.value"
			/>
		</main>

		<SiteUserDeleteModal
			:visible="showDeleteModal"
			:users="[...selected.values()]"
			@close="showDeleteModal = false"
			@queued="onDeletesQueued"
		/>
	</div>
</template>
