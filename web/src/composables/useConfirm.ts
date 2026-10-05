import { ref } from "vue";

interface ConfirmOptions {
	confirmLabel?: string;
	danger?: boolean;
}

interface ConfirmWithOptionOptions extends ConfirmOptions {
	/** Label of a checkbox shown under the message, unchecked by default. */
	optionLabel: string;
	/** Shown under the checkbox while it's checked. */
	optionWarning?: string;
}

interface ConfirmState {
	visible: boolean;
	message: string;
	confirmLabel: string;
	danger: boolean;
	optionLabel: string | null;
	optionWarning: string | null;
	optionChecked: boolean;
	resolve: ((confirmed: boolean, checked: boolean) => void) | null;
}

// Module-level singleton — one confirm dialog for the whole app.
const state = ref<ConfirmState>({
	visible: false,
	message: "",
	confirmLabel: "Confirm",
	danger: false,
	optionLabel: null,
	optionWarning: null,
	optionChecked: false,
	resolve: null,
});

export function useConfirm() {
	function open(
		message: string,
		options: ConfirmOptions | undefined,
		option: { label: string; warning?: string } | null,
		resolve: (confirmed: boolean, checked: boolean) => void,
	) {
		state.value = {
			visible: true,
			message,
			confirmLabel: options?.confirmLabel ?? "Confirm",
			danger: options?.danger ?? false,
			optionLabel: option?.label ?? null,
			optionWarning: option?.warning ?? null,
			optionChecked: false,
			resolve,
		};
	}

	function confirm(
		message: string,
		options?: ConfirmOptions,
	): Promise<boolean> {
		return new Promise((resolve) => {
			open(message, options, null, (confirmed) => resolve(confirmed));
		});
	}

	/** Like confirm, with a checkbox whose state is returned too. */
	function confirmWithOption(
		message: string,
		options: ConfirmWithOptionOptions,
	): Promise<{ confirmed: boolean; checked: boolean }> {
		return new Promise((resolve) => {
			open(
				message,
				options,
				{ label: options.optionLabel, warning: options.optionWarning },
				(confirmed, checked) => resolve({ confirmed, checked }),
			);
		});
	}

	function close(confirmed: boolean) {
		state.value.resolve?.(
			confirmed,
			confirmed && state.value.optionChecked,
		);
		state.value.visible = false;
		state.value.resolve = null;
	}

	function handleConfirm() {
		close(true);
	}

	function handleCancel() {
		close(false);
	}

	return {
		confirmState: state,
		confirm,
		confirmWithOption,
		handleConfirm,
		handleCancel,
	};
}
