<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import { useDataStore } from "../stores/data";
import AppIcon from "../components/AppIcon.vue";
import type { Task, TaskStatus, TaskPriority } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import ListToolbar from "../components/ListToolbar.vue";
import SortableTh from "../components/SortableTh.vue";
import Pagination from "../components/Pagination.vue";
import { useListState } from "../composables/useListState";
import { useClientList } from "../composables/useClientList";
import TaskFormModal from "../components/TaskFormModal.vue";
import { useTaskModal } from "../composables/useTaskModal";
import { getVulnerabilityStatus } from "../utils/taskVulnerability";

const taskStore = useTaskStore();
const authStore = useAuthStore();
const userStore = useUserStore();
const dataStore = useDataStore();

// Opening a task puts it in the URL; TaskModalHost shows it and its edit
// form. The form here is only for creating one.
const { openTask } = useTaskModal();
const showFormModal = ref(false);

// /tasks?q=&sort=&dir=&page=&per=&status=&priority=&type=&due=&assignee=
// The status filter's default ("") is every task but the completed ones;
// "all" shows those too. ?task=<id> (the open task) is kept alongside.
const list = useListState({
	sortKeys: ["title", "status", "priority", "type", "assignee", "due"],
	defaultSort: "",
	descFirst: ["priority"],
	filters: {
		status: [
			"all",
			"pending",
			"in_progress",
			"overdue",
			"completed",
			"skipped",
			"on_hold",
			"blocked",
		],
		priority: ["high", "medium", "low"],
		type: ["one-time", "repeating", "dynamic"],
		due: ["past", "week", "month", "quarter"],
		assignee: true,
	},
});

const setFilter = (name: keyof typeof list.filters.value, e: Event) =>
	list.setFilter(name, (e.target as HTMLSelectElement).value);

onMounted(() => {
	taskStore.fetchTasks();
	userStore.ensureUsers();
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

const PRIORITY_RANK: Record<TaskPriority, number> = {
	low: 1,
	medium: 2,
	high: 3,
};

function matchesDue(task: Task, due: string): boolean {
	if (!due) return true;
	if (!task.due_date) return false;
	const date = new Date(task.due_date);
	if (due === "past") return date < now;
	const bound = getDueDateBound(due);
	return !bound || (date >= now && date <= bound);
}

// Unsorted, paused tasks (on hold, blocked) go after the others.
const tasksPausedLast = computed(() =>
	[...taskStore.tasks].sort(
		(a, b) => Number(isPaused(a.status)) - Number(isPaused(b.status)),
	),
);

// The people tasks are assigned to, for the filter.
const assignees = computed(() =>
	[...new Set(taskStore.tasks.map((t) => t.assigned_to).filter(Boolean))]
		.map((a) => ({ value: a!, label: userStore.resolveDisplayName(a) }))
		.sort((a, b) => a.label.localeCompare(b.label)),
);

const tasks = useClientList(tasksPausedLast, list, {
	search: (t, q) =>
		t.title.toLowerCase().includes(q) ||
		Boolean(t.description?.toLowerCase().includes(q)),
	filter: (t) => {
		const f = list.filters.value;
		if (f.status === "" && t.status === "completed") return false;
		if (f.status && f.status !== "all" && t.status !== f.status)
			return false;
		if (f.priority && t.priority !== f.priority) return false;
		if (f.type && t.type !== f.type) return false;
		if (f.assignee && t.assigned_to !== f.assignee) return false;
		return matchesDue(t, f.due);
	},
	sortValue: (t, key) => {
		switch (key) {
			case "title":
				return t.title;
			case "status":
				return t.status;
			case "priority":
				return PRIORITY_RANK[t.priority];
			case "type":
				return t.type;
			case "assignee":
				return userStore.resolveDisplayName(t.assigned_to);
			case "due":
				return t.due_date;
		}
	},
});

function openCreate() {
	showFormModal.value = true;
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
	on_hold: "warning",
	blocked: "error",
	overdue: "error",
};

const isPaused = (s: TaskStatus) => s === "on_hold" || s === "blocked";

function formatDate(d: string | null) {
	if (!d) return "—";
	return new Date(d).toLocaleDateString("de-DE");
}

function vulnStatus(task: Task) {
	return getVulnerabilityStatus(task, dataStore.enrichedSites);
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

		<ListToolbar :list="list" placeholder="Search tasks">
			<select
				:value="list.filters.value.status"
				aria-label="Status"
				@change="setFilter('status', $event)"
			>
				<option value="">All but completed</option>
				<option value="all">All statuses</option>
				<option value="pending">Pending</option>
				<option value="in_progress">In progress</option>
				<option value="overdue">Overdue</option>
				<option value="completed">Completed</option>
				<option value="skipped">Skipped</option>
				<option value="on_hold">On hold</option>
				<option value="blocked">Blocked</option>
			</select>

			<select
				:value="list.filters.value.priority"
				aria-label="Priority"
				@change="setFilter('priority', $event)"
			>
				<option value="">All priorities</option>
				<option value="high">High</option>
				<option value="medium">Medium</option>
				<option value="low">Low</option>
			</select>

			<select
				:value="list.filters.value.type"
				aria-label="Type"
				@change="setFilter('type', $event)"
			>
				<option value="">All types</option>
				<option value="one-time">One-time</option>
				<option value="repeating">Repeating</option>
				<option value="dynamic">Dynamic</option>
			</select>

			<select
				:value="list.filters.value.due"
				aria-label="Due date"
				@change="setFilter('due', $event)"
			>
				<option value="">Any due date</option>
				<option value="past">Past due</option>
				<option value="week">Due this week</option>
				<option value="month">Due this month</option>
				<option value="quarter">Due this quarter</option>
			</select>

			<select
				:value="list.filters.value.assignee"
				aria-label="Assigned to"
				@change="setFilter('assignee', $event)"
			>
				<option value="">Anyone</option>
				<option v-for="a in assignees" :key="a.value" :value="a.value">
					{{ a.label }}
				</option>
			</select>
		</ListToolbar>

		<div v-if="taskStore.error" class="error-banner">
			<p><strong>Error:</strong> {{ taskStore.error }}</p>
		</div>

		<main class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<SortableTh :list="list" sort-key="title"
							>Title</SortableTh
						>
						<SortableTh :list="list" sort-key="status">
							Status
						</SortableTh>
						<SortableTh :list="list" sort-key="priority">
							Priority
						</SortableTh>
						<SortableTh :list="list" sort-key="type"
							>Type</SortableTh
						>
						<SortableTh :list="list" sort-key="assignee">
							Assigned To
						</SortableTh>
						<SortableTh :list="list" sort-key="due">
							Due Date
						</SortableTh>
					</tr>
				</thead>
				<tbody>
					<tr
						v-if="
							taskStore.isLoading && taskStore.tasks.length === 0
						"
					>
						<td colspan="6" class="loading-state">
							Loading tasks…
						</td>
					</tr>
					<tr v-else-if="tasks.pageItems.value.length === 0">
						<td colspan="6" class="empty-state">No tasks found.</td>
					</tr>
					<tr
						v-for="task in tasks.pageItems.value"
						:key="task.id"
						class="clickable-row"
						@click="openTask(task.id)"
					>
						<td class="max-w-320 truncate font-medium">
							<div class="flex-between gap-2">
								<span class="truncate">{{ task.title }}</span>
								<span
									v-if="vulnStatus(task).isVuln"
									:class="[
										'status-badge',
										'badge-sm',
										vulnStatus(task).remaining === 0
											? 'success'
											: 'warning',
									]"
									style="font-size: 10px; padding: 1px 6px"
									:title="
										vulnStatus(task).remaining === 0
											? 'All vulnerabilities resolved!'
											: `${vulnStatus(task).remaining} remaining vulnerabilities`
									"
								>
									<AppIcon
										:name="
											vulnStatus(task).remaining === 0
												? 'check'
												: 'vulnerability'
										"
										size="12"
										style="margin-right: 4px"
									/>
									{{
										vulnStatus(task).remaining === 0
											? "Resolved"
											: `${vulnStatus(task).remaining} left`
									}}
								</span>
							</div>
						</td>
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
						<td>{{ task.type }}</td>
						<td>
							{{ userStore.resolveDisplayName(task.assigned_to) }}
						</td>
						<td
							:class="{
								overdue:
									task.due_date &&
									new Date(task.due_date) < new Date() &&
									task.status !== 'completed' &&
									task.status !== 'skipped' &&
									!isPaused(task.status),
							}"
						>
							{{ formatDate(task.due_date) }}
						</td>
					</tr>
				</tbody>
			</table>
			<Pagination
				:list="list"
				:page="tasks.page.value"
				:total="tasks.total.value"
			/>
		</main>

		<TaskFormModal
			v-if="showFormModal"
			@close="showFormModal = false"
			@saved="showFormModal = false"
		/>
	</div>
</template>
