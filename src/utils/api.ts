import { useAuthStore } from "../stores/auth";

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
