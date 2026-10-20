<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useAuthStore } from "../stores/auth";
import { useSiteUsersStore } from "../stores/siteUsers";
import { useUpdateJobsStore } from "../stores/updateJobs";
import type { Site, SiteUserList } from "../types";
import LoadingSpinner from "./LoadingSpinner.vue";
import SiteUserCreateModal from "./SiteUserCreateModal.vue";

const props = defineProps<{ site: Site }>();

const authStore = useAuthStore();
const siteUsersStore = useSiteUsersStore();
const jobsStore = useUpdateJobsStore();
const list = ref<SiteUserList | null>(null);
const isLoading = ref(false);
const error = ref<string | null>(null);
const showCreateModal = ref(false);

const canManageUsers = computed(
	() => authStore.canExecute && !!props.site.can_wp_cli,
);

async function load(id: string, quiet = false) {
	if (!quiet) {
		list.value = null;
		isLoading.value = true;
	}
	error.value = null;
	try {
		list.value = await siteUsersStore.fetchSiteUsers(id);
	} catch (e: any) {
		error.value = e.message;
	} finally {
		isLoading.value = false;
	}
}

watch(
	() => props.site.id,
	(id) => load(id),
	{ immediate: true },
);

// A finished user job refreshed the site's users in jman-api.
watch(
	() => jobsStore.lastFinishedBySite[props.site.id],
	(finished) => {
		if (finished) load(props.site.id, true);
	},
);

const excludedText = computed(() =>
	Object.entries(list.value?.excluded_counts ?? {})
		.sort(([a], [b]) => a.localeCompare(b))
		.map(
			([role, n]) => `${n.toLocaleString()} ${role}${n === 1 ? "" : "s"}`,
		)
		.join(", "),
);

const splitRoles = (roles: string) =>
	roles
		.split(",")
		.map((r) => r.trim())
		.filter(Boolean);

const siteRoles = computed(() => [
	...new Set((list.value?.users ?? []).flatMap((u) => splitRoles(u.roles))),
]);

// Only administrators are shown here; the users page lists the rest.
const admins = computed(() =>
	(list.value?.users ?? []).filter((u) =>
		splitRoles(u.roles).includes("administrator"),
	),
);
</script>

<template>
	<section class="card mt-4">
		<div class="card-header">
			<h2>Administrators ({{ admins.length }})</h2>
			<div class="flex-row gap-2">
				<router-link
					v-if="list?.updated_at"
					:to="{ name: 'users', query: { site: site.id } }"
					class="btn btn-outline btn-sm"
				>
					All users ({{ list.users.length.toLocaleString() }})
				</router-link>
				<button
					v-if="canManageUsers"
					type="button"
					class="btn btn-primary btn-sm"
					@click="showCreateModal = true"
				>
					Add user
				</button>
			</div>
		</div>
		<LoadingSpinner v-if="isLoading" message="Loading users..." />
		<div v-else-if="error" class="error-banner">
			<p>{{ error }}</p>
		</div>
		<p v-else-if="!list?.updated_at" class="text-muted">
			Users haven't been collected for this site yet.
		</p>
		<div v-else class="table-container">
			<table class="data-table hide-col-3-sm">
				<thead>
					<tr>
						<th>Login</th>
						<th>Email</th>
						<th>Name</th>
						<th>Roles</th>
						<th>Registered</th>
						<th class="text-right">Posts</th>
					</tr>
				</thead>
				<tbody>
					<tr v-if="admins.length === 0">
						<td colspan="6" class="empty-state">
							No administrators found.
						</td>
					</tr>
					<tr v-for="user in admins" :key="user.id">
						<td class="font-medium">{{ user.login }}</td>
						<td>{{ user.email }}</td>
						<td>{{ user.display_name }}</td>
						<td>
							<span
								v-for="role in splitRoles(user.roles)"
								:key="role"
								class="status-badge badge-sm info"
							>
								{{ role }}
							</span>
						</td>
						<td class="text-muted">
							{{ user.registered.slice(0, 10) }}
						</td>
						<td class="text-right">
							{{ user.post_count?.toLocaleString() ?? "?" }}
						</td>
					</tr>
				</tbody>
			</table>
			<p v-if="excludedText || list.truncated" class="sub-text">
				<template v-if="excludedText">
					Not listed: {{ excludedText }}.
				</template>
				<template v-if="list.truncated">
					Only the first {{ list.users.length.toLocaleString() }}
					users were collected.
				</template>
			</p>
		</div>
		<SiteUserCreateModal
			:visible="showCreateModal"
			:site-id="site.id"
			:site-name="site.domain"
			:site-roles="siteRoles"
			@close="showCreateModal = false"
		/>
	</section>
</template>
