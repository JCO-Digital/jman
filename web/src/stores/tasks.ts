import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	Task,
	TaskStatus,
	CreateTaskPayload,
	UpdateTaskPayload,
	TaskFilters,
} from "../types";
import { useAuthStore } from "./auth";
import { handleErrorResponse, BASE_URL } from "../utils/api";

export const useTaskStore = defineStore("tasks", () => {
	const authStore = useAuthStore();

	const tasks = ref<Task[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchTasks(filters?: TaskFilters) {
		isLoading.value = true;
		error.value = null;
		try {
			const url = new URL(`${BASE_URL}/tasks`, window.location.origin);
			if (filters) {
				for (const [key, value] of Object.entries(filters)) {
					if (value !== undefined && value !== null && value !== "") {
						url.searchParams.append(key, String(value));
					}
				}
			}
			const res = await fetch(url.toString(), {
				headers: authStore.authHeader,
			});
			if (!res.ok) await handleErrorResponse(res);
			tasks.value = await res.json();
		} catch (e: any) {
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	async function getTask(id: number): Promise<Task | null> {
		try {
			const res = await fetch(`${BASE_URL}/tasks/${id}`, {
				headers: authStore.authHeader,
			});
			if (!res.ok) await handleErrorResponse(res);
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
		}
	}

	// Patches the in-memory list so list views (Tasks page, dashboard widget)
	// reflect a change the instant the request resolves, instead of waiting
	// for a caller to trigger a separate full fetchTasks().
	function upsertTask(task: Task) {
		const idx = tasks.value.findIndex((t) => t.id === task.id);
		if (idx !== -1) {
			tasks.value[idx] = task;
		} else {
			tasks.value.push(task);
		}
	}

	async function createTask(payload: CreateTaskPayload): Promise<Task> {
		const res = await fetch(`${BASE_URL}/tasks`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(payload),
		});
		if (!res.ok) await handleErrorResponse(res);
		const task = await res.json();
		upsertTask(task);
		return task;
	}

	async function updateTask(
		id: number,
		payload: UpdateTaskPayload,
	): Promise<Task> {
		const res = await fetch(`${BASE_URL}/tasks/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
				...authStore.authHeader,
			},
			body: JSON.stringify(payload),
		});
		if (!res.ok) await handleErrorResponse(res);
		const task = await res.json();
		upsertTask(task);
		return task;
	}

	async function setStatus(id: number, status: TaskStatus): Promise<Task> {
		return updateTask(id, { status });
	}

	async function completeTask(id: number): Promise<Task> {
		const res = await fetch(`${BASE_URL}/tasks/${id}/complete`, {
			method: "POST",
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		const task = await res.json();
		upsertTask(task);
		// Completing a repeating/dynamic task spawns its next occurrence
		// server-side, which this response doesn't include — refresh in the
		// background so it appears without blocking the visible status change.
		if (task.type !== "one-time") {
			void fetchTasks();
		}
		return task;
	}

	async function deleteTask(id: number): Promise<void> {
		const res = await fetch(`${BASE_URL}/tasks/${id}`, {
			method: "DELETE",
			headers: authStore.authHeader,
		});
		if (!res.ok) await handleErrorResponse(res);
		tasks.value = tasks.value.filter((t) => t.id !== id);
	}

	return {
		tasks,
		isLoading,
		error,
		fetchTasks,
		getTask,
		createTask,
		updateTask,
		setStatus,
		completeTask,
		deleteTask,
	};
});
