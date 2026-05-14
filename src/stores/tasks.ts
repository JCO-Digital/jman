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

const BASE_URL = import.meta.env.VITE_API_BASE_URL || "/api";

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
			if (!res.ok) throw new Error("Failed to fetch tasks");
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
			if (!res.ok) throw new Error("Failed to fetch task");
			return await res.json();
		} catch (e) {
			console.error(e);
			return null;
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
		if (!res.ok) throw new Error("Failed to create task");
		return await res.json();
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
		if (!res.ok) throw new Error("Failed to update task");
		return await res.json();
	}

	async function setStatus(id: number, status: TaskStatus): Promise<Task> {
		return updateTask(id, { status });
	}

	async function completeTask(id: number): Promise<void> {
		const res = await fetch(`${BASE_URL}/tasks/${id}/complete`, {
			method: "POST",
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to complete task");
	}

	async function deleteTask(id: number): Promise<void> {
		const res = await fetch(`${BASE_URL}/tasks/${id}`, {
			method: "DELETE",
			headers: authStore.authHeader,
		});
		if (!res.ok) throw new Error("Failed to delete task");
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
