<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useTaskStore } from "../stores/tasks";
import { useSettingsStore } from "../stores/settings";
import { useAuthStore } from "../stores/auth";
import type { DashboardWidgetType } from "../types";
import ViewHeader from "../components/ViewHeader.vue";
import StatSummaryWidget from "../components/StatSummaryWidget.vue";
import TaskReminderWidget from "../components/TaskReminderWidget.vue";
import VulnerabilityWidget from "../components/VulnerabilityWidget.vue";
import UpcomingRenewalsWidget from "../components/UpcomingRenewalsWidget.vue";
import AppIcon from "../components/AppIcon.vue";

const dataStore = useDataStore();
const organizationStore = useOrganizationStore();
const taskStore = useTaskStore();
const settingsStore = useSettingsStore();
const authStore = useAuthStore();

const isEditMode = ref(false);

const toggleEditMode = () => {
	isEditMode.value = !isEditMode.value;
};

const layout = computed(() => settingsStore.dashboardLayout);

const availableWidgets = [
	{ id: "stats", name: "Stat Summary" },
	{ id: "tasks", name: "Task Reminders" },
	{ id: "vulnerabilities", name: "Vulnerabilities" },
	{ id: "renewals", name: "Upcoming Renewals" },
] as const;

const missingWidgets = computed(() => {
	return availableWidgets.filter((w) => !layout.value.includes(w.id));
});

const removeWidget = (id: DashboardWidgetType) => {
	settingsStore.dashboardLayout = settingsStore.dashboardLayout.filter(
		(w) => w !== id,
	);
};

const addWidget = (id: DashboardWidgetType) => {
	if (!settingsStore.dashboardLayout.includes(id)) {
		settingsStore.dashboardLayout = [...settingsStore.dashboardLayout, id];
	}
};

// Simple native Drag & Drop reordering
const draggedIndex = ref<number | null>(null);

const onDragStart = (index: number) => {
	if (!isEditMode.value) return;
	draggedIndex.value = index;
};

const onDragOver = (e: DragEvent) => {
	if (!isEditMode.value) return;
	e.preventDefault();
	if (e.dataTransfer) {
		e.dataTransfer.dropEffect = "move";
	}
};

const onDrop = (index: number) => {
	if (!isEditMode.value || draggedIndex.value === null) return;

	const newLayout = [...settingsStore.dashboardLayout];
	const movedItem = newLayout.splice(draggedIndex.value, 1)[0];
	if (movedItem) {
		newLayout.splice(index, 0, movedItem);
		settingsStore.dashboardLayout = newLayout;
	}
	draggedIndex.value = null;
};

onMounted(() => {
	dataStore.initData();
	organizationStore.fetchOrganizations();
	taskStore.fetchTasks();
});
</script>

<template>
	<div class="view-container">
		<ViewHeader title="Dashboard">
			<template #actions>
				<div v-if="authStore.canEdit" class="flex-row">
					<button
						class="icon-btn"
						:class="{ active: isEditMode }"
						:title="isEditMode ? 'Done Editing' : 'Edit Layout'"
						@click="toggleEditMode"
					>
						<AppIcon
							:name="isEditMode ? 'check' : 'edit'"
							size="20"
						/>
					</button>
				</div>
			</template>
		</ViewHeader>

		<div v-if="dataStore.error" class="error-banner">
			<p><strong>Error loading data:</strong> {{ dataStore.error }}</p>
		</div>

		<div
			v-if="isEditMode && missingWidgets.length > 0"
			class="card p-4"
			style="margin-top: var(--space-6)"
		>
			<h3 class="mb-3 font-medium">Add Widgets</h3>
			<div class="flex-row gap-2">
				<button
					v-for="widget in missingWidgets"
					:key="widget.id"
					class="btn btn-outline btn-sm"
					@click="addWidget(widget.id as DashboardWidgetType)"
				>
					<AppIcon name="plus-circle" size="14" class="mr-1" />
					{{ widget.name }}
				</button>
			</div>
		</div>

		<main class="dashboard-widgets">
			<div
				v-for="(widgetId, index) in layout"
				:key="widgetId"
				class="widget-container"
				:class="{ 'is-editing': isEditMode }"
				:draggable="isEditMode"
				@dragstart="onDragStart(index)"
				@dragover="onDragOver"
				@drop="onDrop(index)"
			>
				<div v-if="isEditMode" class="widget-overlay">
					<div class="drag-handle">
						<AppIcon name="drag-handle" size="20" />
					</div>
					<button
						class="remove-btn"
						title="Remove widget"
						@click="removeWidget(widgetId)"
					>
						<AppIcon name="trash" size="18" />
					</button>
				</div>

				<StatSummaryWidget v-if="widgetId === 'stats'" />
				<TaskReminderWidget v-else-if="widgetId === 'tasks'" />
				<VulnerabilityWidget
					v-else-if="widgetId === 'vulnerabilities'"
				/>
				<UpcomingRenewalsWidget v-else-if="widgetId === 'renewals'" />
			</div>
		</main>
	</div>
</template>

<style scoped>
.dashboard-widgets {
	display: flex;
	flex-direction: column;
	gap: var(--space-6);
	margin-top: var(--space-6);
}

.widget-container {
	position: relative;
	transition: all 0.2s ease;
}

.widget-container.is-editing {
	cursor: move;
	border: 2px dashed var(--border-color);
	border-radius: var(--radius-md);
	padding: var(--space-2);
	background: var(--bg-hover);
}

.widget-overlay {
	position: absolute;
	top: var(--space-4);
	right: var(--space-4);
	display: flex;
	gap: var(--space-2);
	z-index: 10;
}

.drag-handle {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: var(--radius-sm);
	padding: var(--space-1);
	display: flex;
	align-items: center;
	justify-content: center;
	cursor: grab;
	opacity: 0.7;
}

.remove-btn {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: var(--radius-sm);
	padding: var(--space-1);
	color: var(--error-text);
	display: flex;
	align-items: center;
	justify-content: center;
	cursor: pointer;
	transition: all 0.2s ease;
}

.remove-btn:hover {
	background: var(--error-bg);
}

.mb-3 {
	margin-bottom: var(--space-3);
}

.mr-1 {
	margin-right: var(--space-1);
}

.p-4 {
	padding: var(--space-4);
}
</style>
