import { ref } from "vue";
import { defineStore } from "pinia";

export type ToastType = "error" | "success" | "info";

export interface Toast {
	id: number;
	message: string;
	type: ToastType;
	duration: number;
}

export const useToastStore = defineStore("toast", () => {
	const toasts = ref<Toast[]>([]);
	let nextId = 0;

	function addToast(
		message: string,
		type: ToastType = "error",
		duration: number = 5000,
	) {
		const id = nextId++;
		toasts.value.push({ id, message, type, duration });
		setTimeout(() => removeToast(id), duration);
	}

	function removeToast(id: number) {
		toasts.value = toasts.value.filter((t) => t.id !== id);
	}

	return {
		toasts,
		addToast,
		removeToast,
	};
});
