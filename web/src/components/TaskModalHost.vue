<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useToastStore } from "../stores/toast";
import { useTaskModal } from "../composables/useTaskModal";
import type { Task } from "../types";
import TaskInfoModal from "./TaskInfoModal.vue";
import TaskFormModal from "./TaskFormModal.vue";

// Shows the task in the URL's ?task=<id> (see useTaskModal) over any view,
// and its edit form. Mounted once in App.vue for signed-in users.

const taskStore = useTaskStore();
const toast = useToastStore();
const { taskId, closeTask } = useTaskModal();

// The task as fetched by ID, for one that isn't in the store's list (a
// link to a completed task, or a view that hasn't loaded the list).
const fetched = ref<Task | null>(null);
const editing = ref(false);

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
		editing.value = false;
		if (id == null || fetched.value?.id === id) return;
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

function handleSaved(saved: Task) {
	fetched.value = saved;
	editing.value = false;
}

function handleUpdated(updated: Task) {
	fetched.value = updated;
}
</script>

<template>
	<TaskFormModal
		v-if="task && editing"
		:task="task"
		@close="editing = false"
		@saved="handleSaved"
	/>
	<TaskInfoModal
		v-else-if="task"
		:task="task"
		@close="closeTask"
		@edit="editing = true"
		@updated="handleUpdated"
	/>
</template>
