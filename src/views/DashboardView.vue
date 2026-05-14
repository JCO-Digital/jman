<script setup lang="ts">
import { ref, onMounted } from "vue";
import { RouterLink } from "vue-router";
import { useDataStore } from "../stores/data";
import { useAssetStore } from "../stores/assetStore";
import { useTaskStore } from "../stores/tasks";
import type { OrganizationAsset, Task } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import StatCard from "../components/StatCard.vue";
import VulnerabilityWidget from "../components/VulnerabilityWidget.vue";
import TaskInfoModal from "../components/TaskInfoModal.vue";

const dataStore = useDataStore();
const assetStore = useAssetStore();
const taskStore = useTaskStore();

const upcomingRenewals = ref<OrganizationAsset[]>([]);
const isRenewalsLoading = ref(false);

const selectedTask = ref<Task | null>(null);
const showTaskModal = ref(false);

const reminderTasks = ref<Task[]>([]);
const isTasksLoading = ref(false);

const loadReminderTasks = async () => {
	isTasksLoading.value = true;
	try {
		await taskStore.fetchTasks();
		const now = new Date();
		reminderTasks.value = taskStore.tasks.filter((t) => {
			if (!t.reminder_date) return false;
			if (
				t.status === "completed" ||
				t.status === "skipped"
			)
				return false;
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

const loadRenewals = async () => {
	isRenewalsLoading.value = true;
	try {
		const thirtyDaysFromNow = new Date();
		thirtyDaysFromNow.setDate(thirtyDaysFromNow.getDate() + 30);

		upcomingRenewals.value = await assetStore.fetchAllOrganizationAssets({
			status: "active",
			before: thirtyDaysFromNow.toISOString(),
		});

		// Sort by next_billing
		upcomingRenewals.value.sort((a, b) => {
			if (!a.next_billing) return 1;
			if (!b.next_billing) return -1;
			return (
				new Date(a.next_billing).getTime() -
				new Date(b.next_billing).getTime()
			);
		});
	} catch (e) {
		console.error("Failed to load renewals", e);
	} finally {
		isRenewalsLoading.value = false;
	}
};

onMounted(() => {
	loadRenewals();
	loadReminderTasks();
});

const formatCurrency = (cents: number) => {
	return new Intl.NumberFormat("de-DE", {
		style: "currency",
		currency: "EUR",
	}).format(cents / 100);
};

const formatDate = (dateString: string | null) => {
	if (!dateString) return "-";
	return new Date(dateString).toLocaleDateString("de-DE");
};
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Dashboard" />

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<main class="dashboard-grid">
			<StatCard
				title="Sites"
				:value="dataStore.sites.length"
				label="Total sites in cache"
			/>

			<StatCard
				title="Plugins"
				:value="dataStore.pluginInfo.length"
				label="Unique plugins in cache"
			/>

			<StatCard
				title="Vulnerabilities"
				:value="dataStore.vulnerabilities.length"
				label="Active vulnerabilities detected"
				:loading="dataStore.isVulnsLoading"
				:value-style="{
					color:
						dataStore.vulnerabilities.length > 0
							? '#d32f2f'
							: 'inherit',
				}"
			/>
		</main>

		<VulnerabilityWidget />

		<section
			v-if="reminderTasks.length > 0 || isTasksLoading"
			class="card tasks-widget"
		>
			<div class="card-header">
				<h2>Tasks Needing Attention</h2>
				<RouterLink to="/tasks" class="view-all-link"
					>View all</RouterLink
				>
			</div>
			<div v-if="isTasksLoading" class="loading-state">
				<p>Loading tasks…</p>
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
									task.due_date
										? formatDate(task.due_date)
										: "—"
								}}
							</td>
							<td class="action-col" @click.stop>
								<button
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
		</section>

		<section
			v-if="upcomingRenewals.length > 0 || isRenewalsLoading"
			class="card renewals-widget"
		>
			<div class="card-header">
				<h2>Upcoming Renewals (30 Days)</h2>
			</div>
			<div v-if="isRenewalsLoading" class="loading-state">
				<p>Loading renewals...</p>
			</div>
			<div v-else class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th>Organization</th>
							<th>Asset</th>
							<th>Price</th>
							<th>Due Date</th>
						</tr>
					</thead>
					<tbody>
						<tr v-for="oa in upcomingRenewals" :key="oa.id">
							<td>{{ oa.organization_name }}</td>
							<td>
								<strong>{{
									oa.asset_name || oa.identifier
								}}</strong>
							</td>
							<td>{{ formatCurrency(oa.price) }}</td>
							<td
								:class="{
									overdue:
										new Date(oa.next_billing || '') <
										new Date(),
								}"
							>
								{{ formatDate(oa.next_billing) }}
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</section>

		<TaskInfoModal
			v-if="showTaskModal && selectedTask"
			:task="selectedTask"
			@close="showTaskModal = false"
			@edit="showTaskModal = false"
			@updated="handleTaskUpdated"
			@deleted="handleTaskDeleted"
		/>
	</div>
</template>

<style scoped>
.dashboard-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
	gap: 24px;
	margin-top: 24px;
}

.tasks-widget,
.renewals-widget {
	margin-top: 32px;
}

.card-header {
	display: flex;
	justify-content: space-between;
	align-items: center;
}

.tasks-widget h2,
.renewals-widget h2 {
	font-size: 1.25rem;
	margin: 0;
}

.view-all-link {
	font-size: 13px;
	color: var(--primary);
	text-decoration: none;
}

.view-all-link:hover {
	text-decoration: underline;
}

.loading-state {
	padding: 2rem;
	text-align: center;
	color: var(--text-muted);
}

.task-title-cell {
	font-weight: 500;
	max-width: 260px;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.action-col {
	text-align: right;
	white-space: nowrap;
}

.btn-sm {
	padding: 4px 12px;
	font-size: 13px;
}

.overdue {
	color: var(--error-text);
	font-weight: 600;
}
</style>
