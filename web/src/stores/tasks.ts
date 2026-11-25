import { ref } from "vue";
import { defineStore } from "pinia";
import type {
	Task,
	TaskStatus,
	CreateTaskPayload,
	UpdateTaskPayload,
	TaskFilters,
	TaskChecklistItem,
	NewTaskChecklistItem,
} from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const useTaskStore = defineStore("tasks", () => {
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
			const res = await apiFetch(url.toString(), {});
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
			const res = await apiFetch(`${BASE_URL}/tasks/${id}`);
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

	// Puts a changed checklist on the listed task, so its progress shows
	// at once.
	function patchChecklist(id: number, checklist: TaskChecklistItem[]) {
		const t = tasks.value.find((t) => t.id === id);
		if (!t) return;
		t.checklist = checklist;
		t.checklist_total = checklist.length;
		t.checklist_done = checklist.filter((i) => i.done).length;
	}

	async function checklistRequest(
		id: number,
		path: string,
		method: string,
		body?: unknown,
	): Promise<TaskChecklistItem[]> {
		const res = await apiFetch(`${BASE_URL}/tasks/${id}/checklist${path}`, {
			method,
			headers:
				body === undefined
					? undefined
					: { "Content-Type": "application/json" },
			body: body === undefined ? undefined : JSON.stringify(body),
		});
		if (!res.ok) await handleErrorResponse(res);
		const checklist: TaskChecklistItem[] = await res.json();
		patchChecklist(id, checklist);
		return checklist;
	}

	const addChecklistItems = (id: number, items: NewTaskChecklistItem[]) =>
		checklistRequest(id, "", "POST", { items });
	const updateChecklistItem = (
		id: number,
		itemId: number,
		changes: { text?: string; done?: boolean },
	) => checklistRequest(id, `/${itemId}`, "PATCH", changes);
	const deleteChecklistItem = (id: number, itemId: number) =>
		checklistRequest(id, `/${itemId}`, "DELETE");
	const reorderChecklist = (id: number, ids: number[]) =>
		checklistRequest(id, "/order", "PUT", { ids });

	/** Watches (or unwatches) a task as the signed-in user. */
	async function setWatching(id: number, watch: boolean): Promise<string[]> {
		const res = await apiFetch(`${BASE_URL}/tasks/${id}/watch`, {
			method: watch ? "PUT" : "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
		const watchers: string[] = await res.json();
		const t = tasks.value.find((t) => t.id === id);
		if (t) t.watchers = watchers;
		return watchers;
	}

	async function createTask(payload: CreateTaskPayload): Promise<Task> {
		const res = await apiFetch(`${BASE_URL}/tasks`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
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
		const res = await apiFetch(`${BASE_URL}/tasks/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
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
		const res = await apiFetch(`${BASE_URL}/tasks/${id}/complete`, {
			method: "POST",
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
		const res = await apiFetch(`${BASE_URL}/tasks/${id}`, {
			method: "DELETE",
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
		upsertTask,
		addChecklistItems,
		updateChecklistItem,
		deleteChecklistItem,
		reorderChecklist,
		setWatching,
	};
});
