import { computed } from "vue";
import { useRoute, useRouter } from "vue-router";

/**
 * The open task, kept in the URL as ?task=<id> on whatever view is showing,
 * so a task can be linked to (Slack messages link /tasks?task=<id>) and
 * survives a reload. TaskModalHost, mounted once in App.vue, shows it.
 */
export function useTaskModal() {
	const route = useRoute();
	const router = useRouter();

	const taskId = computed<number | null>(() => {
		const v = route.query.task;
		const id = typeof v === "string" ? Number(v) : NaN;
		return Number.isInteger(id) && id > 0 ? id : null;
	});

	/** Opens a task over the current view; Back closes it again. */
	function openTask(id: number) {
		router.push({ query: { ...route.query, task: String(id) } });
	}

	/**
	 * Closes the open task. If opening it added the history entry, that
	 * entry is popped, so Back doesn't reopen it; a task reached by a link
	 * is closed in place instead, so the app isn't left.
	 */
	function closeTask() {
		if (taskId.value == null) return;
		const query = { ...route.query, task: undefined };
		const target = router.resolve({ query }).fullPath;
		if (window.history.state?.back === target) {
			router.back();
		} else {
			router.replace({ query });
		}
	}

	return { taskId, openTask, closeTask };
}
