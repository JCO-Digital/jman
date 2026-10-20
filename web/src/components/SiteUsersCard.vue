<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useSiteUsersStore } from "../stores/siteUsers";
import type { SiteUserList } from "../types";
import LoadingSpinner from "./LoadingSpinner.vue";

const props = defineProps<{ siteId: string }>();

const siteUsersStore = useSiteUsersStore();
const list = ref<SiteUserList | null>(null);
const isLoading = ref(false);
const error = ref<string | null>(null);

watch(
	() => props.siteId,
	async (id) => {
		list.value = null;
		error.value = null;
		isLoading.value = true;
		try {
			list.value = await siteUsersStore.fetchSiteUsers(id);
		} catch (e: any) {
			error.value = e.message;
		} finally {
			isLoading.value = false;
		}
	},
	{ immediate: true },
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
			<router-link
				v-if="list?.updated_at"
				:to="{ name: 'users', query: { site: siteId } }"
				class="btn btn-outline btn-sm"
			>
				All users ({{ list.users.length.toLocaleString() }})
			</router-link>
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
	</section>
</template>
