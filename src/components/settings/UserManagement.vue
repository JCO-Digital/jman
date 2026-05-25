<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import { useToastStore } from "../../stores/toast";
import LoadingSpinner from "../LoadingSpinner.vue";
import AppIcon from "../AppIcon.vue";
import UserFormModal from "./UserFormModal.vue";
import type { AdminUser } from "../../types";
import { useConfirm } from "../../composables/useConfirm";

const authStore = useAuthStore();
const userStore = useUserStore();
const toast = useToastStore();
const { confirm } = useConfirm();

const showModal = ref(false);
const editingUser = ref<{
	username: string;
	displayName: string;
	level: string;
} | null>(null);

onMounted(() => {
	userStore.fetchUsers();
});

function openCreateModal() {
	editingUser.value = null;
	showModal.value = true;
}

function openEditModal(user: AdminUser) {
	editingUser.value = {
		username: user.username,
		displayName: user.displayName,
		level: user.level,
	};
	showModal.value = true;
}

function closeModal() {
	showModal.value = false;
	editingUser.value = null;
}

function handleSaved() {
	toast.addToast("User saved successfully", "success");
	userStore.fetchUsers();
}

async function handleDelete(user: AdminUser) {
	if (user.username === authStore.user?.username) return;

	if (!await confirm(`Are you sure you want to delete user "${user.username}"?`, { danger: true }))
		return;

	try {
		await userStore.deleteUser(user.username);
		toast.addToast(
			`User "${user.username}" deleted successfully`,
			"success",
		);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to delete user", "error");
	}
}

function levelClass(level: string): string {
	switch (level) {
		case "admin":
			return "active";
		case "execute":
		case "edit":
			return "info";
		default:
			return "";
	}
}
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>User Management</h2>
			<button class="btn btn-primary" @click="openCreateModal">
				Create User
			</button>
		</div>

		<div v-if="userStore.isLoading" class="loading-state">
			<LoadingSpinner message="Loading users..." />
		</div>

		<div
			v-else-if="userStore.users.length > 0"
			class="table-container mt-4"
		>
			<table class="data-table">
				<thead>
					<tr>
						<th>Username</th>
						<th>Display Name</th>
						<th>Level</th>
						<th class="hide-mobile">2FA Status</th>
						<th class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="user in userStore.users" :key="user.username">
						<td class="font-medium text-main">
							{{ user.username }}
						</td>
						<td class="text-main">{{ user.displayName }}</td>
						<td>
							<span
								:class="[
									'status-badge',
									'badge-sm',
									levelClass(user.level),
								]"
							>
								{{ user.level }}
							</span>
						</td>
						<td class="hide-mobile">
							<span
								v-if="user.has2FA"
								class="flex-row gap-1 text-success font-sm font-medium"
							>
								<AppIcon name="check" size="16" />
								Enabled
							</span>
							<span v-else class="text-muted font-sm"
								>Disabled</span
							>
						</td>
						<td class="text-right">
							<div class="flex-row gap-2 justify-end">
								<button
									class="btn btn-text"
									@click="openEditModal(user)"
								>
									Edit
								</button>
								<button
									class="btn btn-text danger"
									:disabled="
										user.username ===
										authStore.user?.username
									"
									:title="
										user.username ===
										authStore.user?.username
											? 'Cannot delete yourself'
											: 'Delete user'
									"
									@click="handleDelete(user)"
								>
									Delete
								</button>
							</div>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">No users found.</p>
		</div>
	</section>

	<UserFormModal
		:visible="showModal"
		:edit-user="editingUser"
		@close="closeModal"
		@saved="handleSaved"
	/>
</template>

<style scoped>
/* Scoped styles removed in favor of global utility classes and component styles */
</style>
