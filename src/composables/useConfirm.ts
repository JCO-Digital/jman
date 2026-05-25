import { ref } from "vue";

interface ConfirmOptions {
	confirmLabel?: string;
	danger?: boolean;
}

interface ConfirmState {
	visible: boolean;
	message: string;
	confirmLabel: string;
	danger: boolean;
	resolve: ((value: boolean) => void) | null;
}

// Module-level singleton — one confirm dialog for the whole app.
const state = ref<ConfirmState>({
	visible: false,
	message: "",
	confirmLabel: "Confirm",
	danger: false,
	resolve: null,
});

export function useConfirm() {
	function confirm(
		message: string,
		options?: ConfirmOptions,
	): Promise<boolean> {
		return new Promise((resolve) => {
			state.value = {
				visible: true,
				message,
				confirmLabel: options?.confirmLabel ?? "Confirm",
				danger: options?.danger ?? false,
				resolve,
			};
		});
	}

	function handleConfirm() {
		state.value.resolve?.(true);
		state.value.visible = false;
		state.value.resolve = null;
	}

	function handleCancel() {
		state.value.resolve?.(false);
		state.value.visible = false;
		state.value.resolve = null;
	}

	return { confirmState: state, confirm, handleConfirm, handleCancel };
}
