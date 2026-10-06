import { ref } from "vue";
import { defineStore } from "pinia";
import type { Note, NoteParentType } from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

export const useNotesStore = defineStore("notes", () => {
	const notes = ref<Note[]>([]);
	const isLoading = ref(false);
	const error = ref<string | null>(null);

	async function fetchNotes(
		parentType: NoteParentType,
		parentId: string | number,
	) {
		isLoading.value = true;
		error.value = null;
		try {
			const res = await apiFetch(
				`${BASE_URL}/notes?type=${parentType}&id=${encodeURIComponent(parentId.toString())}`,
			);
			if (!res.ok) await handleErrorResponse(res);
			notes.value = await res.json();
		} catch (e: any) {
			error.value = e.message;
			console.error(e);
		} finally {
			isLoading.value = false;
		}
	}

	async function createNote(
		parentType: NoteParentType,
		parentId: string | number,
		content: string,
	) {
		const res = await apiFetch(`${BASE_URL}/notes`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({
				parent_type: parentType,
				parent_id: parentId.toString(),
				content,
			}),
		});
		if (!res.ok) await handleErrorResponse(res);
		const newNote = await res.json();
		notes.value.unshift(newNote);
		return newNote;
	}

	async function updateNote(id: number, content: string) {
		const res = await apiFetch(`${BASE_URL}/notes/${id}`, {
			method: "PATCH",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ content }),
		});
		if (!res.ok) await handleErrorResponse(res);
		const updatedNote = await res.json();
		const idx = notes.value.findIndex((n) => n.id === id);
		if (idx !== -1) {
			notes.value[idx] = updatedNote;
		}
		return updatedNote;
	}

	async function deleteNote(id: number) {
		const res = await apiFetch(`${BASE_URL}/notes/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
		notes.value = notes.value.filter((n) => n.id !== id);
	}

	return {
		notes,
		isLoading,
		error,
		fetchNotes,
		createNote,
		updateNote,
		deleteNote,
	};
});
