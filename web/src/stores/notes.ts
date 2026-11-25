import { ref } from "vue";
import { defineStore } from "pinia";
import type { Note, NoteParentType } from "../types";
import { handleErrorResponse, BASE_URL, apiFetch } from "../utils/api";

function noteKey(parentType: NoteParentType, parentId: string | number) {
	return `${parentType}:${parentId}`;
}

/**
 * Notes, kept per parent: a task's notes can be open in the task modal over
 * a site page showing the site's notes, so one shared list would mix them.
 */
export const useNotesStore = defineStore("notes", () => {
	const byParent = ref<Record<string, Note[]>>({});
	const loading = ref<Record<string, boolean>>({});

	function notesFor(parentType: NoteParentType, parentId: string | number) {
		return byParent.value[noteKey(parentType, parentId)] ?? [];
	}

	function isLoadingFor(
		parentType: NoteParentType,
		parentId: string | number,
	) {
		return !!loading.value[noteKey(parentType, parentId)];
	}

	async function fetchNotes(
		parentType: NoteParentType,
		parentId: string | number,
	) {
		const key = noteKey(parentType, parentId);
		loading.value[key] = true;
		try {
			const res = await apiFetch(
				`${BASE_URL}/notes?type=${parentType}&id=${encodeURIComponent(parentId.toString())}`,
			);
			if (!res.ok) await handleErrorResponse(res);
			byParent.value[key] = await res.json();
		} catch (e: any) {
			console.error(e);
		} finally {
			loading.value[key] = false;
		}
	}

	/**
	 * Adds a note. For a task, watch says whether the author starts
	 * watching it (the server's default is yes).
	 */
	async function createNote(
		parentType: NoteParentType,
		parentId: string | number,
		content: string,
		options: { watch?: boolean } = {},
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
				...(options.watch === undefined
					? {}
					: { watch: options.watch }),
			}),
		});
		if (!res.ok) await handleErrorResponse(res);
		const newNote: Note = await res.json();
		const key = noteKey(parentType, parentId);
		byParent.value[key] = [newNote, ...(byParent.value[key] ?? [])];
		return newNote;
	}

	function replaceNote(id: number, fn: (list: Note[]) => Note[]) {
		for (const key of Object.keys(byParent.value)) {
			const list = byParent.value[key]!;
			if (list.some((n) => n.id === id)) {
				byParent.value[key] = fn(list);
			}
		}
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
		const updatedNote: Note = await res.json();
		replaceNote(id, (list) =>
			list.map((n) => (n.id === id ? updatedNote : n)),
		);
		return updatedNote;
	}

	async function deleteNote(id: number) {
		const res = await apiFetch(`${BASE_URL}/notes/${id}`, {
			method: "DELETE",
		});
		if (!res.ok) await handleErrorResponse(res);
		replaceNote(id, (list) => list.filter((n) => n.id !== id));
	}

	return {
		notesFor,
		isLoadingFor,
		fetchNotes,
		createNote,
		updateNote,
		deleteNote,
	};
});
