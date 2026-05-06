<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useAuthStore } from "../../stores/auth";
import { useUserStore } from "../../stores/user";
import { useToastStore } from "../../stores/toast";
import LoadingSpinner from "../LoadingSpinner.vue";
import UserFormModal from "./UserFormModal.vue";
import type { AdminUser } from "../../types";

const authStore = useAuthStore();
const userStore = useUserStore();
const toast = useToastStore();

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

	if (!confirm(`Are you sure you want to delete user "${user.username}"?`))
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
		case "execute":
			return "level-execute";
		case "edit":
			return "level-edit";
		default:
			return "level-basic";
	}
}
</script>

<template>
	<section class="card">
		<div class="section-header">
			<h2>User Management</h2>
			<button class="btn btn-primary" @click="openCreateModal">
				Create User
			</button>
		</div>

		<div v-if="userStore.isLoading" class="state-container">
			<LoadingSpinner message="Loading users..." />
		</div>

		<div v-else-if="userStore.users.length > 0" class="table-container">
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
						<td class="font-medium">{{ user.username }}</td>
						<td>{{ user.displayName }}</td>
						<td>
							<span
								:class="['level-badge', levelClass(user.level)]"
							>
								{{ user.level }}
							</span>
						</td>
						<td class="hide-mobile">
							<span v-if="user.has2FA" class="twofa-enabled">
								<svg
									class="check-icon"
									viewBox="0 0 20 20"
									fill="currentColor"
									width="16"
									height="16"
								>
									<path
										fill-rule="evenodd"
										d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
										clip-rule="evenodd"
									/>
								</svg>
								Enabled
							</span>
							<span v-else class="twofa-disabled">Disabled</span>
						</td>
						<td class="text-right">
							<div class="actions-cell">
								<button
									class="btn-text"
									@click="openEditModal(user)"
								>
									Edit
								</button>
								<button
									class="btn-text danger"
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

		<div v-else class="state-container">
			<p class="empty-text">No users found.</p>
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
.section-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 16px;
}

.section-header h2 {
	margin: 0;
	border-bottom: none;
	padding-bottom: 0;
}

.state-container {
	padding: 40px;
	text-align: center;
	color: var(--text-muted);
}

.empty-text {
	margin: 0;
	font-style: italic;
}

.font-medium {
	font-weight: 500;
}

.text-right {
	text-align: right;
}

.actions-cell {
	display: inline-flex;
	justify-content: flex-end;
	gap: 12px;
}

.level-badge {
	display: inline-block;
	padding: 2px 10px;
	border-radius: 9999px;
	font-size: 12px;
	font-weight: 500;
	text-transform: capitalize;
}

.level-basic {
	background-color: var(--badge-default-bg);
	color: var(--badge-default-text);
}

.level-edit {
	background-color: var(--badge-must-use-bg);
	color: var(--badge-must-use-text);
}

.level-execute {
	background-color: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.twofa-enabled {
	display: inline-flex;
	align-items: center;
	gap: 4px;
	color: var(--badge-active-text);
	font-size: 13px;
	font-weight: 500;
}

.check-icon {
	flex-shrink: 0;
}

.twofa-disabled {
	color: var(--text-muted);
	font-size: 13px;
}

.btn-text {
	background: none;
	border: none;
	color: var(--primary);
	cursor: pointer;
	font-size: 14px;
	padding: 0;
	font-weight: 500;
}

.btn-text.danger {
	color: var(--error-text);
}

.btn-text:disabled {
	color: var(--text-disabled);
	cursor: not-allowed;
}
</style>
