<script setup lang="ts">
import { ref } from "vue";
import { RouterLink } from "vue-router";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import type { Task, TaskStatus } from "../types";

const props = defineProps<{
	task: Task;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "edit", task: Task): void;
	(e: "updated", task: Task): void;
	(e: "deleted"): void;
}>();

const taskStore = useTaskStore();
const authStore = useAuthStore();
const userStore = useUserStore();

const isActioning = ref(false);
const actionError = ref<string | null>(null);

async function complete() {
	isActioning.value = true;
	actionError.value = null;
	try {
		await taskStore.completeTask(props.task.id);
		emit("updated", { ...props.task, status: "completed" });
	} catch (e: any) {
		actionError.value = e.message;
	} finally {
		isActioning.value = false;
	}
}

async function changeStatus(status: TaskStatus) {
	isActioning.value = true;
	actionError.value = null;
	try {
		const updated = await taskStore.setStatus(props.task.id, status);
		emit("updated", updated);
	} catch (e: any) {
		actionError.value = e.message;
	} finally {
		isActioning.value = false;
	}
}

const isDeleting = ref(false);

async function handleDelete() {
	if (!confirm(`Delete task "${props.task.title}"?`)) return;
	isDeleting.value = true;
	try {
		await taskStore.deleteTask(props.task.id);
		emit("deleted");
	} catch (e: any) {
		actionError.value = e.message;
	} finally {
		isDeleting.value = false;
	}
}

function formatDate(d: string | null) {
	if (!d) return "—";
	return new Date(d).toLocaleDateString("de-DE", { dateStyle: "medium" });
}

const priorityClass: Record<string, string> = {
	low: "default",
	medium: "warning",
	high: "error",
};

const statusClass: Record<string, string> = {
	pending: "default",
	in_progress: "active",
	completed: "success",
	skipped: "default",
	overdue: "error",
};

const isTerminal = (s: string) => s === "completed" || s === "skipped";
const canComplete = (s: string) =>
	s === "pending" || s === "in_progress" || s === "overdue";
</script>

<template>
	<Teleport to="body">
		<div class="modal-overlay" @click.self="emit('close')">
			<div class="modal-content card">
				<header class="modal-header">
					<div class="title-area">
						<div class="flex-row gap-2 mb-4">
							<span
								:class="[
									'status-badge',
									'badge-sm',
									statusClass[task.status],
								]"
							>
								{{ task.status.replace("_", " ") }}
							</span>
							<span
								:class="[
									'status-badge',
									'badge-sm',
									priorityClass[task.priority],
								]"
							>
								{{ task.priority }}
							</span>
							<span class="status-badge badge-sm info">{{
								task.type
							}}</span>
						</div>
						<h2>{{ task.title }}</h2>
					</div>
					<button class="modal-close" @click="emit('close')">
						&times;
					</button>
				</header>

				<div class="content">
					<p v-if="task.description" class="description">
						{{ task.description }}
					</p>

					<div class="info-grid">
						<div class="info-item">
							<span class="label">Assigned to</span>
							<span class="value">{{
								userStore.resolveDisplayName(task.assigned_to)
							}}</span>
						</div>
						<div class="info-item">
							<span class="label">Due date</span>
							<span
								class="value"
								:class="{
									overdue:
										task.due_date &&
										new Date(task.due_date) < new Date() &&
										!isTerminal(task.status),
								}"
							>
								{{ formatDate(task.due_date) }}
							</span>
						</div>
						<div class="info-item">
							<span class="label">Reminder date</span>
							<span class="value">{{
								formatDate(task.reminder_date)
							}}</span>
						</div>
						<div v-if="task.interval" class="info-item">
							<span class="label">Interval</span>
							<span class="value">{{ task.interval }}</span>
						</div>
						<div class="info-item">
							<span class="label">Created by</span>
							<span class="value">{{ task.created_by }}</span>
						</div>
						<div class="info-item">
							<span class="label">Created at</span>
							<span class="value">{{
								formatDate(task.created_at)
							}}</span>
						</div>
					</div>

					<!-- Linked entities -->
					<div
						v-if="
							task.site_id ||
							task.server_id ||
							task.organization_id ||
							task.plugin_slug
						"
						class="section-divider"
					>
						<h3 class="sub-text font-medium mb-4">Linked to</h3>
						<div class="info-grid">
							<div v-if="task.site_id" class="info-item">
								<span class="label">Site</span>
								<RouterLink
									:to="`/site/${task.site_id}`"
									class="font-sm font-medium"
									@click="emit('close')"
								>
									Site #{{ task.site_id }}
								</RouterLink>
							</div>
							<div v-if="task.server_id" class="info-item">
								<span class="label">Server</span>
								<span class="value"
									>Server #{{ task.server_id }}</span
								>
							</div>
							<div v-if="task.organization_id" class="info-item">
								<span class="label">Organization</span>
								<RouterLink
									:to="`/organization/${task.organization_id}`"
									class="font-sm font-medium"
									@click="emit('close')"
								>
									Organization #{{ task.organization_id }}
								</RouterLink>
							</div>
							<div v-if="task.plugin_slug" class="info-item">
								<span class="label">Plugin</span>
								<RouterLink
									:to="`/plugin/${task.plugin_slug}`"
									class="font-sm font-medium"
									@click="emit('close')"
								>
									{{ task.plugin_slug }}
								</RouterLink>
							</div>
						</div>
					</div>

					<!-- Status actions -->
					<div
						v-if="!isTerminal(task.status)"
						class="section-divider"
					>
						<h3 class="sub-text font-medium mb-4">Quick Actions</h3>
						<div class="flex-row gap-3">
							<button
								v-if="canComplete(task.status)"
								class="btn btn-primary"
								:disabled="isActioning"
								@click="complete"
							>
								Complete Task
							</button>
							<button
								v-if="task.status === 'pending'"
								class="btn btn-outline"
								:disabled="isActioning"
								@click="changeStatus('in_progress')"
							>
								Start Task
							</button>
							<button
								v-if="
									task.status === 'pending' ||
									task.status === 'in_progress' ||
									task.status === 'overdue'
								"
								class="btn btn-outline"
								:disabled="isActioning"
								@click="changeStatus('skipped')"
							>
								Skip Task
							</button>
						</div>
					</div>

					<p v-if="actionError" class="error-banner">
						{{ actionError }}
					</p>
				</div>

				<footer class="form-actions flex-between mt-4">
					<button
						v-if="authStore.canEdit"
						class="btn btn-text danger"
						:disabled="isDeleting"
						@click="handleDelete"
					>
						{{ isDeleting ? "Deleting…" : "Delete Task" }}
					</button>
					<div class="flex-row gap-3">
						<button class="btn btn-outline" @click="emit('close')">
							Close
						</button>
						<button
							v-if="authStore.canEdit"
							class="btn btn-primary"
							@click="emit('edit', task)"
						>
							Edit Task
						</button>
					</div>
				</footer>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
/* Scoped styles removed in favor of global utility classes and component styles */
</style>
