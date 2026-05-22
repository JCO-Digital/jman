<script setup lang="ts">
import { ref, onMounted } from "vue";
import { RouterLink } from "vue-router";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import type { Task } from "../types";
import TaskInfoModal from "./TaskInfoModal.vue";

const taskStore = useTaskStore();
const authStore = useAuthStore();

const reminderTasks = ref<Task[]>([]);
const isTasksLoading = ref(false);
const selectedTask = ref<Task | null>(null);
const showTaskModal = ref(false);

const loadReminderTasks = async () => {
	isTasksLoading.value = true;
	try {
		await taskStore.fetchTasks();
		const now = new Date();
		const currentUsername = authStore.user?.username;
		reminderTasks.value = taskStore.tasks.filter((t) => {
			if (!t.reminder_date) return false;
			if (t.status === "completed" || t.status === "skipped")
				return false;

			// Filter: only show unassigned or assigned to current user
			const isUnassigned = !t.assigned_to;
			const isAssignedToMe =
				currentUsername && t.assigned_to === currentUsername;
			if (!isUnassigned && !isAssignedToMe) return false;

			return new Date(t.reminder_date) <= now;
		});
	} catch (e) {
		console.error("Failed to load reminder tasks", e);
	} finally {
		isTasksLoading.value = false;
	}
};

function openTask(task: Task) {
	selectedTask.value = task;
	showTaskModal.value = true;
}

async function completeTask(task: Task) {
	try {
		await taskStore.completeTask(task.id);
		reminderTasks.value = reminderTasks.value.filter(
			(t) => t.id !== task.id,
		);
	} catch (e) {
		console.error("Failed to complete task", e);
	}
}

function handleTaskUpdated(updated: Task) {
	selectedTask.value = updated;
	if (updated.status === "completed" || updated.status === "skipped") {
		reminderTasks.value = reminderTasks.value.filter(
			(t) => t.id !== updated.id,
		);
	}
}

function handleTaskDeleted() {
	showTaskModal.value = false;
	if (selectedTask.value) {
		reminderTasks.value = reminderTasks.value.filter(
			(t) => t.id !== selectedTask.value?.id,
		);
	}
}

const formatDate = (dateString: string | null) => {
	if (!dateString) return "-";
	return new Date(dateString).toLocaleDateString("de-DE");
};

onMounted(loadReminderTasks);
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Tasks Needing Attention</h2>
			<RouterLink to="/tasks" class="view-all-link">View all</RouterLink>
		</div>
		<div v-if="isTasksLoading" class="loading-state p-4">
			<p>Loading tasks…</p>
		</div>
		<div v-else-if="reminderTasks.length === 0" class="p-4 text-muted">
			<p>No tasks needing attention right now.</p>
		</div>
		<div v-else class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Task</th>
						<th>Priority</th>
						<th class="hide-mobile">Due Date</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="task in reminderTasks"
						:key="task.id"
						class="clickable-row"
						@click="openTask(task)"
					>
						<td class="task-title-cell">{{ task.title }}</td>
						<td>
							<span
								:class="[
									'status-badge',
									task.priority === 'high'
										? 'error'
										: task.priority === 'medium'
											? 'warning'
											: 'default',
								]"
							>
								{{ task.priority }}
							</span>
						</td>
						<td
							class="hide-mobile"
							:class="{
								overdue:
									task.due_date &&
									new Date(task.due_date) < new Date(),
							}"
						>
							{{
								task.due_date ? formatDate(task.due_date) : "—"
							}}
						</td>
						<td class="text-right" @click.stop>
							<button
								v-if="authStore.canEdit"
								class="btn btn-primary btn-sm"
								@click="completeTask(task)"
							>
								Complete
							</button>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<TaskInfoModal
			v-if="showTaskModal && selectedTask"
			:task="selectedTask"
			@close="showTaskModal = false"
			@edit="showTaskModal = false"
			@updated="handleTaskUpdated"
			@deleted="handleTaskDeleted"
		/>
	</section>
</template>

<style scoped>
.loading-state,
.p-4 {
	padding: var(--space-5);
	text-align: center;
}
.view-all-link {
	font-size: var(--font-size-sm);
	color: var(--primary);
	text-decoration: none;
}
.view-all-link:hover {
	text-decoration: underline;
}
</style>
