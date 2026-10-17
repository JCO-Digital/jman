<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import { useSiteUsersStore } from "../stores/siteUsers";
import ViewHeader from "../components/ViewHeader.vue";
import Pagination from "../components/Pagination.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import AppIcon from "../components/AppIcon.vue";

const props = defineProps<{
	page?: number;
	rowsPerPage?: number;
}>();

type SortKey =
	| "domain"
	| "login"
	| "display_name"
	| "email"
	| "roles"
	| "registered";

const route = useRoute();
const router = useRouter();
const dataStore = useDataStore();
const siteUsersStore = useSiteUsersStore();

onMounted(() => {
	dataStore.initData();
	siteUsersStore.fetchAll(true);
});

const searchQuery = ref("");
const filterRole = ref("");
const sortKey = ref<SortKey>("domain");
const sortOrder = ref<"asc" | "desc">("asc");
const currentPage = ref(props.page || 1);
const rowsPerPage = ref(props.rowsPerPage || 50);

watch(
	() => props.page,
	(newVal) => {
		currentPage.value = newVal || 1;
	},
);

watch(
	() => props.rowsPerPage,
	(newVal) => {
		rowsPerPage.value = newVal || 50;
	},
);

// The site filter lives in the URL (?site=<id>), so a site's page can link
// to its users.
const filterSite = computed(() =>
	typeof route.query.site === "string" ? route.query.site : "",
);
const filterSiteDomain = computed(
	() => dataStore.getSiteById(filterSite.value)?.domain ?? filterSite.value,
);

const updateRoute = (page: number, rpp: number, site = filterSite.value) => {
	router.push({
		name: "users",
		params: {
			page: page.toString(),
			rowsPerPage: rpp.toString(),
		},
		query: site ? { site } : {},
	});
};

const clearSiteFilter = () => updateRoute(1, rowsPerPage.value, "");

const handleSort = (key: SortKey) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
};

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

const roleOptions = computed(() => {
	const roles = new Set<string>();
	for (const u of usersWithSite.value) {
		for (const r of splitRoles(u.roles)) roles.add(r);
	}
	return [...roles].sort();
});

const filteredUsers = computed(() => {
	let filtered = usersWithSite.value;

	if (searchQuery.value) {
		const query = searchQuery.value.toLowerCase();
		filtered = filtered.filter(
			(u) =>
				u.login.toLowerCase().includes(query) ||
				u.email.toLowerCase().includes(query) ||
				u.display_name.toLowerCase().includes(query) ||
				u.domain.toLowerCase().includes(query),
		);
	}
	if (filterRole.value) {
		filtered = filtered.filter((u) =>
			splitRoles(u.roles).includes(filterRole.value),
		);
	}

	const dir = sortOrder.value === "asc" ? 1 : -1;
	return [...filtered].sort((a, b) => {
		const cmp = a[sortKey.value]
			.toLowerCase()
			.localeCompare(b[sortKey.value].toLowerCase());
		if (cmp !== 0) return cmp * dir;
		return a.login.toLowerCase().localeCompare(b.login.toLowerCase());
	});
});

const totalPages = computed(
	() => Math.ceil(filteredUsers.value.length / rowsPerPage.value) || 1,
);

const paginatedUsers = computed(() => {
	const start = (currentPage.value - 1) * rowsPerPage.value;
	return filteredUsers.value.slice(start, start + rowsPerPage.value);
});

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

const prevPage = () => {
	if (currentPage.value > 1) {
		updateRoute(currentPage.value - 1, rowsPerPage.value);
	}
};

const nextPage = () => {
	if (currentPage.value < totalPages.value) {
		updateRoute(currentPage.value + 1, rowsPerPage.value);
	}
};

const handleRowsPerPageUpdate = (newRpp: number) => {
	updateRoute(1, newRpp);
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

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search by login, email, name or site..."
				class="search-input"
				@input="updateRoute(1, rowsPerPage)"
			/>

			<select v-model="filterRole" @change="updateRoute(1, rowsPerPage)">
				<option value="">All Roles</option>
				<option v-for="role in roleOptions" :key="role" :value="role">
					{{ role }}
				</option>
			</select>

			<button
				v-if="filterSite"
				type="button"
				class="btn btn-primary"
				title="Show users of all sites"
				@click="clearSiteFilter"
			>
				{{ filterSiteDomain }}
				<AppIcon name="x" size="14" />
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
			<table class="data-table sortable hide-col-4-sm">
				<thead>
					<tr>
						<th class="col-wide" @click="handleSort('domain')">
							Site
							<span v-if="sortKey === 'domain'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('login')">
							Login
							<span v-if="sortKey === 'login'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th class="col-expand" @click="handleSort('email')">
							Email
							<span v-if="sortKey === 'email'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('display_name')">
							Name
							<span v-if="sortKey === 'display_name'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('roles')">
							Roles
							<span v-if="sortKey === 'roles'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
						<th @click="handleSort('registered')">
							Registered
							<span v-if="sortKey === 'registered'">{{
								sortOrder === "asc" ? "↑" : "↓"
							}}</span>
						</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							(siteUsersStore.isLoading || dataStore.isLoading) &&
							usersWithSite.length === 0
						"
					>
						<td colspan="6">
							<LoadingSpinner message="Loading users..." />
						</td>
					</tr>
					<tr v-else-if="paginatedUsers.length === 0">
						<td colspan="6" class="empty-state">
							<span v-if="searchQuery || filterRole"
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
					</tr>
				</tbody>
			</table>

			<Pagination
				:current-page="currentPage"
				:total-pages="totalPages"
				:rows-per-page="rowsPerPage"
				@update:rows-per-page="handleRowsPerPageUpdate"
				@prev="prevPage"
				@next="nextPage"
			/>
		</main>
	</div>
</template>
