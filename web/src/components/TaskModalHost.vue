<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useToastStore } from "../stores/toast";
import { useTaskModal } from "../composables/useTaskModal";
import type { Task } from "../types";
import TaskDetailModal from "./TaskDetailModal.vue";

// Shows the task in the URL's ?task=<id> (see useTaskModal) over any view.
// Mounted once in App.vue for signed-in users.

const taskStore = useTaskStore();
const toast = useToastStore();
const { taskId, closeTask } = useTaskModal();

// The task as read on its own: with its checklist and watchers, which the
// list doesn't carry, and for a task that isn't in the list at all.
const fetched = ref<Task | null>(null);

// The store's copy wins, so changes made anywhere show up here at once.
const task = computed<Task | null>(() => {
	const id = taskId.value;
	if (id == null) return null;
	return (
		taskStore.tasks.find((t) => t.id === id) ??
		(fetched.value?.id === id ? fetched.value : null)
	);
});

watch(
	taskId,
	async (id) => {
		if (id == null) return;
		const result = await taskStore.getTask(id);
		if (taskId.value !== id) return;
		if (result) {
			fetched.value = result;
		} else if (!task.value) {
			toast.addToast(`Task #${id} wasn't found`, "error");
			closeTask();
		}
	},
	{ immediate: true },
);

function handleUpdated(updated: Task) {
	if (fetched.value?.id === updated.id) fetched.value = updated;
}
</script>

<template>
	<TaskDetailModal
		v-if="task"
		:task="task"
		:details="fetched?.id === task.id ? fetched : null"
		@close="closeTask"
		@updated="handleUpdated"
	/>
</template>
