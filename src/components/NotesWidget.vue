<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import type { NoteParentType, Note } from "../types";
import { useNotesStore } from "../stores/notes";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import { useToastStore } from "../stores/toast";
import { useConfirm } from "../composables/useConfirm";
import LoadingSpinner from "./LoadingSpinner.vue";
import AppIcon from "./AppIcon.vue";

const props = defineProps<{
	parentType: NoteParentType;
	parentId: number;
}>();

const notesStore = useNotesStore();
const authStore = useAuthStore();
const userStore = useUserStore();
const toastStore = useToastStore();
const { confirm } = useConfirm();

const newNoteContent = ref("");
const isAdding = ref(false);
const editingNoteId = ref<number | null>(null);
const editingContent = ref("");
const isSaving = ref(false);

const isFormExpanded = ref(false);
const isListExpanded = ref(false);

const visibleNotes = computed(() => {
	if (isListExpanded.value) {
		return notesStore.notes;
	}
	return notesStore.notes.slice(0, 3);
});

const remainingNotesCount = computed(() => {
	return Math.max(0, notesStore.notes.length - 3);
});

const loadNotes = () => {
	notesStore.fetchNotes(props.parentType, props.parentId);
};

onMounted(() => {
	loadNotes();
});

watch(
	() => [props.parentType, props.parentId],
	() => {
		loadNotes();
		isFormExpanded.value = false;
		isListExpanded.value = false;
	},
);

const handleAddNote = async () => {
	if (!newNoteContent.value.trim()) return;
	isAdding.value = true;
	try {
		await notesStore.createNote(
			props.parentType,
			props.parentId,
			newNoteContent.value,
		);
		newNoteContent.value = "";
		isFormExpanded.value = false;
		toastStore.addToast("Note added successfully.", "success");
	} catch (e: any) {
		toastStore.addToast("Failed to add note: " + e.message, "error");
	} finally {
		isAdding.value = false;
	}
};

const startEdit = (note: Note) => {
	editingNoteId.value = note.id;
	editingContent.value = note.content;
};

const cancelEdit = () => {
	editingNoteId.value = null;
	editingContent.value = "";
};

const handleUpdateNote = async (id: number) => {
	if (!editingContent.value.trim()) return;
	isSaving.value = true;
	try {
		await notesStore.updateNote(id, editingContent.value);
		editingNoteId.value = null;
		editingContent.value = "";
		toastStore.addToast("Note updated successfully.", "success");
	} catch (e: any) {
		toastStore.addToast("Failed to update note: " + e.message, "error");
	} finally {
		isSaving.value = false;
	}
};

const handleDeleteNote = async (id: number) => {
	if (
		!(await confirm("Are you sure you want to delete this note?", {
			confirmLabel: "Delete",
			danger: true,
		}))
	) {
		return;
	}
	try {
		await notesStore.deleteNote(id);
		toastStore.addToast("Note deleted successfully.", "success");
	} catch (e: any) {
		toastStore.addToast("Failed to delete note: " + e.message, "error");
	}
};

const formatNoteDate = (dateString: string) => {
	return new Date(dateString).toLocaleString("de-DE", {
		day: "2-digit",
		month: "2-digit",
		year: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
};
</script>

<template>
	<section class="card notes-section">
		<div class="card-header border-none mb-2">
			<div class="flex-row items-center gap-2">
				<AppIcon name="note" size="18" />
				<h2>Notes</h2>
			</div>
			<button
				v-if="authStore.canEdit && !isFormExpanded"
				class="btn btn-outline btn-sm"
				@click="isFormExpanded = true"
			>
				<AppIcon name="plus-circle" size="14" />
				<span class="ml-1">Add Note</span>
			</button>
		</div>

		<!-- Add note form -->
		<div
			v-if="authStore.canEdit && isFormExpanded"
			class="add-note-form mb-4"
		>
			<textarea
				v-model="newNoteContent"
				placeholder="Add a new note..."
				rows="3"
				class="note-textarea"
				:disabled="isAdding"
			></textarea>
			<div class="flex-row justify-end gap-2 mt-2">
				<button
					class="btn btn-outline btn-sm"
					:disabled="isAdding"
					@click="isFormExpanded = false"
				>
					Cancel
				</button>
				<button
					class="btn btn-primary btn-sm"
					:disabled="!newNoteContent.trim() || isAdding"
					@click="handleAddNote"
				>
					{{ isAdding ? "Adding..." : "Add Note" }}
				</button>
			</div>
		</div>

		<!-- Notes list -->
		<div v-if="notesStore.isLoading" class="loading-container py-4">
			<LoadingSpinner message="Loading notes..." />
		</div>
		<div v-else-if="notesStore.notes.length === 0" class="empty-state py-4">
			No notes available.
		</div>
		<div v-else class="notes-list">
			<div v-for="note in visibleNotes" :key="note.id" class="note-item">
				<!-- Edit mode -->
				<div v-if="editingNoteId === note.id" class="edit-note-form">
					<textarea
						v-model="editingContent"
						rows="3"
						class="note-textarea"
						:disabled="isSaving"
					></textarea>
					<div class="flex-row justify-end gap-2 mt-2">
						<button
							class="btn btn-outline btn-sm"
							:disabled="isSaving"
							@click="cancelEdit"
						>
							Cancel
						</button>
						<button
							class="btn btn-primary btn-sm"
							:disabled="!editingContent.trim() || isSaving"
							@click="handleUpdateNote(note.id)"
						>
							{{ isSaving ? "Saving..." : "Save" }}
						</button>
					</div>
				</div>

				<!-- View mode -->
				<div v-else>
					<div class="note-content pre-wrap">{{ note.content }}</div>
					<div
						class="note-meta flex-row justify-between items-center mt-3 pt-2"
					>
						<span class="sub-text text-sm text-muted">
							By
							{{ userStore.resolveDisplayName(note.created_by) }}
							on
							{{ formatNoteDate(note.created_at) }}
							<template
								v-if="note.updated_at !== note.created_at"
							>
								(edited by
								{{
									userStore.resolveDisplayName(
										note.updated_by,
									)
								}}
								on {{ formatNoteDate(note.updated_at) }})
							</template>
						</span>
						<div v-if="authStore.canEdit" class="flex-row gap-2">
							<button
								class="icon-btn icon-btn-sm"
								title="Edit Note"
								@click="startEdit(note)"
							>
								<AppIcon name="edit" size="14" />
							</button>
							<button
								class="icon-btn icon-btn-sm danger"
								title="Delete Note"
								@click="handleDeleteNote(note.id)"
							>
								<AppIcon name="trash" size="14" />
							</button>
						</div>
					</div>
				</div>
			</div>

			<div
				v-if="remainingNotesCount > 0 && !isListExpanded"
				class="flex-row justify-center mt-3"
			>
				<button
					class="btn btn-outline btn-sm"
					@click="isListExpanded = true"
				>
					Show {{ remainingNotesCount }} more...
				</button>
			</div>
		</div>
	</section>
</template>

<style scoped>
.notes-section {
	margin-top: 1rem;
}
.border-none {
	border: none;
}
.note-textarea {
	width: 100%;
	padding: 0.75rem;
	border: 1px solid var(--border-color);
	border-radius: 6px;
	resize: vertical;
	background-color: var(--bg-card);
	color: var(--text-main);
	font-family: inherit;
	font-size: 0.95rem;
	line-height: 1.5;
}
.note-textarea:focus {
	outline: none;
	border-color: var(--primary);
}
.notes-list {
	display: flex;
	flex-direction: column;
	gap: 1rem;
}
.note-item {
	padding: 1rem;
	background-color: var(--bg-body);
	border: 1px solid var(--border-color);
	border-radius: 6px;
}
.note-content {
	color: var(--text-main);
	font-size: 0.95rem;
	line-height: 1.5;
}
.pre-wrap {
	white-space: pre-wrap;
}
.note-meta {
	border-top: 1px solid var(--border-color);
}
</style>
