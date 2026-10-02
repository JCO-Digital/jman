/**
 * Toasts are the pop-up view of the task center's notifications; see
 * ./notifications. useToastStore().addToast(message, type, duration) keeps
 * working for existing callers.
 */
export {
	useNotificationStore as useToastStore,
	type ToastType,
} from "./notifications";
