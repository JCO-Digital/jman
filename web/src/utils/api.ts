import { useAuthStore } from "../stores/auth";

export const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "/api";

export async function handleErrorResponse(res: Response): Promise<never> {
	const authStore = useAuthStore();
	if (res.status === 401) {
		authStore.logout();
		throw new Error("Unauthorized");
	}
	let message: string;
	try {
		const data = await res.json();
		message = data.error || `Request failed (${res.status})`;
	} catch {
		message = `Request failed (${res.status})`;
	}
	throw new Error(message);
}
