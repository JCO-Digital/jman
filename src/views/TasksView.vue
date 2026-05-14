<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import type { Task, TaskStatus, TaskPriority, TaskType } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import TaskInfoModal from "../components/TaskInfoModal.vue";
import TaskFormModal from "../components/TaskFormModal.vue";

const taskStore = useTaskStore();
const authStore = useAuthStore();

const selectedTask = ref<Task | null>(null);
const showInfoModal = ref(false);
const showFormModal = ref(false);
const editingTask = ref<Task | null>(null);

// Filters
const searchQuery = ref("");
const filterStatus = ref<TaskStatus | "">("");
const filterPriority = ref<TaskPriority | "">("");
const filterType = ref<TaskType | "">("");
const filterAssignedTo = ref("");
const filterDueDate = ref<"" | "past" | "week" | "month" | "quarter">("");

onMounted(() => {
	taskStore.fetchTasks();
});

const now = new Date();

function getDueDateBound(option: string): Date | null {
	const d = new Date(now);
	if (option === "week") {
		d.setDate(d.getDate() + 7);
		return d;
	}
	if (option === "month") {
		d.setDate(d.getDate() + 30);
		return d;
	}
	if (option === "quarter") {
		d.setDate(d.getDate() + 90);
		return d;
	}
	return null;
}

const filteredTasks = computed(() => {
	let result = taskStore.tasks;

	if (searchQuery.value) {
		const q = searchQuery.value.toLowerCase();
		result = result.filter(
			(t) =>
				t.title.toLowerCase().includes(q) ||
				t.description?.toLowerCase().includes(q),
		);
	}

	if (filterStatus.value) {
		result = result.filter((t) => t.status === filterStatus.value);
	}

	if (filterPriority.value) {
		result = result.filter((t) => t.priority === filterPriority.value);
	}

	if (filterType.value) {
		result = result.filter((t) => t.type === filterType.value);
	}

	if (filterAssignedTo.value) {
		const q = filterAssignedTo.value.toLowerCase();
		result = result.filter((t) =>
			t.assigned_to?.toLowerCase().includes(q),
		);
	}

	if (filterDueDate.value) {
		if (filterDueDate.value === "past") {
			result = result.filter(
				(t) => t.due_date && new Date(t.due_date) < now,
			);
		} else {
			const bound = getDueDateBound(filterDueDate.value);
			if (bound) {
				result = result.filter(
					(t) =>
						t.due_date &&
						new Date(t.due_date) >= now &&
						new Date(t.due_date) <= bound,
				);
			}
		}
	}

	return result;
});

function openTask(task: Task) {
	selectedTask.value = task;
	showInfoModal.value = true;
}

function openCreate() {
	editingTask.value = null;
	showFormModal.value = true;
}

function openEdit(task: Task) {
	editingTask.value = task;
	showInfoModal.value = false;
	showFormModal.value = true;
}

function handleSaved(task: Task) {
	showFormModal.value = false;
	taskStore.fetchTasks();
	if (editingTask.value) {
		selectedTask.value = task;
		showInfoModal.value = true;
	}
}

function handleTaskUpdated(task: Task) {
	selectedTask.value = task;
	const idx = taskStore.tasks.findIndex((t) => t.id === task.id);
	if (idx !== -1) taskStore.tasks[idx] = task;
}

function handleTaskDeleted() {
	showInfoModal.value = false;
	taskStore.fetchTasks();
}

const priorityClass: Record<TaskPriority, string> = {
	low: "default",
	medium: "warning",
	high: "error",
};

const statusClass: Record<TaskStatus, string> = {
	pending: "default",
	in_progress: "active",
	completed: "success",
	skipped: "default",
	overdue: "error",
};

function formatDate(d: string | null) {
	if (!d) return "—";
	return new Date(d).toLocaleDateString("de-DE");
}
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Tasks">
			<template #actions>
				<button
					v-if="authStore.canEdit"
					class="btn btn-primary"
					@click="openCreate"
				>
					New Task
				</button>
			</template>
		</ViewHeader>

		<div class="controls">
			<input
				v-model="searchQuery"
				type="text"
				placeholder="Search tasks..."
				class="search-input"
			/>

			<select v-model="filterStatus" class="filter-select">
				<option value="">All Statuses</option>
				<option value="pending">Pending</option>
				<option value="in_progress">In Progress</option>
				<option value="overdue">Overdue</option>
				<option value="completed">Completed</option>
				<option value="skipped">Skipped</option>
			</select>

			<select v-model="filterPriority" class="filter-select">
				<option value="">All Priorities</option>
				<option value="high">High</option>
				<option value="medium">Medium</option>
				<option value="low">Low</option>
			</select>

			<select v-model="filterType" class="filter-select">
				<option value="">All Types</option>
				<option value="one-time">One-time</option>
				<option value="repeating">Repeating</option>
				<option value="dynamic">Dynamic</option>
			</select>

			<select v-model="filterDueDate" class="filter-select">
				<option value="">Any Due Date</option>
				<option value="past">Past Due</option>
				<option value="week">Due This Week</option>
				<option value="month">Due This Month</option>
				<option value="quarter">Due This Quarter</option>
			</select>

			<input
				v-model="filterAssignedTo"
				type="text"
				placeholder="Assigned to..."
				class="search-input filter-narrow"
			/>
		</div>

		<div v-if="taskStore.error" class="error-banner">
			<p><strong>Error:</strong> {{ taskStore.error }}</p>
		</div>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Title</th>
						<th>Status</th>
						<th>Priority</th>
						<th class="hide-mobile">Type</th>
						<th class="hide-mobile">Assigned To</th>
						<th class="hide-mobile">Due Date</th>
					</tr>
				</thead>
				<tbody>
					<tr v-if="taskStore.isLoading && taskStore.tasks.length === 0">
						<td colspan="6" class="center-state">
							Loading tasks…
						</td>
					</tr>
					<tr v-else-if="filteredTasks.length === 0">
						<td colspan="6" class="empty-state">No tasks found.</td>
					</tr>
					<tr
						v-for="task in filteredTasks"
						:key="task.id"
						class="clickable-row"
						@click="openTask(task)"
					>
						<td class="task-title">{{ task.title }}</td>
						<td>
							<span
								:class="[
									'status-badge',
									statusClass[task.status],
								]"
							>
								{{ task.status.replace("_", " ") }}
							</span>
						</td>
						<td>
							<span
								:class="[
									'status-badge',
									priorityClass[task.priority],
								]"
							>
								{{ task.priority }}
							</span>
						</td>
						<td class="hide-mobile">{{ task.type }}</td>
						<td class="hide-mobile">
							{{ task.assigned_to ?? "—" }}
						</td>
						<td
							class="hide-mobile"
							:class="{
								overdue:
									task.due_date &&
									new Date(task.due_date) < new Date() &&
									task.status !== 'completed' &&
									task.status !== 'skipped',
							}"
						>
							{{ formatDate(task.due_date) }}
						</td>
					</tr>
				</tbody>
			</table>
		</main>

		<TaskInfoModal
			v-if="showInfoModal && selectedTask"
			:task="selectedTask"
			@close="showInfoModal = false"
			@edit="openEdit"
			@updated="handleTaskUpdated"
			@deleted="handleTaskDeleted"
		/>

		<TaskFormModal
			v-if="showFormModal"
			:task="editingTask"
			@close="showFormModal = false"
			@saved="handleSaved"
		/>
	</div>
</template>

<style scoped>
.controls {
	display: flex;
	flex-wrap: wrap;
	gap: 10px;
	margin-bottom: 20px;
}

.filter-select {
	padding: 8px 12px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	background: var(--bg-input);
	color: var(--text-main);
	font-size: 14px;
	cursor: pointer;
}

.filter-narrow {
	max-width: 160px;
}

.task-title {
	font-weight: 500;
	max-width: 320px;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.center-state {
	text-align: center;
	padding: 2rem;
	color: var(--text-muted);
}

.overdue {
	color: var(--error-text);
	font-weight: 600;
}
</style>
