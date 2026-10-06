import { useAuthStore } from "../stores/auth";

export const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "/api";

/**
 * fetch() for authenticated API calls. Attaches a fresh access token,
 * refreshing it first if it is about to expire, and retries once with a
 * new token if the request is rejected with 401.
 */
export async function apiFetch(
	input: string,
	init: RequestInit = {},
): Promise<Response> {
	const authStore = useAuthStore();

	const send = (token: string | null) => {
		const headers = new Headers(init.headers);
		if (token) headers.set("Authorization", `Bearer ${token}`);
		return fetch(input, { ...init, headers });
	};

	const token = await authStore.getValidToken();
	const res = await send(token);
	if (res.status !== 401 || !token) return res;

	// The access token was rejected (e.g. the API restarted with a new
	// signing key); a refresh either recovers or ends the session.
	if (!(await authStore.refreshToken()) || authStore.token === token) {
		return res;
	}
	return send(authStore.token);
}

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
