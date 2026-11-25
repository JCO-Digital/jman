<script setup lang="ts">
import { ref, reactive, computed, onMounted, nextTick } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useUserStore } from "../stores/user";
import AppIcon from "./AppIcon.vue";
import SearchableSelect from "./SearchableSelect.vue";
import TaskChecklist, { type ChecklistRow } from "./TaskChecklist.vue";
import type {
	NewTaskChecklistItem,
	Task,
	TaskType,
	TaskPriority,
} from "../types";

// The new-task form. A task is edited in place once it exists
// (TaskDetailModal).

const props = defineProps<{
	/** Prefilled fields. */
	defaults?: Partial<
		Pick<
			Task,
			"title" | "description" | "priority" | "site_id" | "plugin_slug"
		>
	>;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved", task: Task): void;
}>();

const taskStore = useTaskStore();
const dataStore = useDataStore();
const orgStore = useOrganizationStore();
const userStore = useUserStore();

const isSaving = ref(false);
const saveError = ref<string | null>(null);
const titleInput = ref<HTMLInputElement | null>(null);

const d = props.defaults ?? {};
const form = reactive({
	title: d.title ?? "",
	description: d.description ?? "",
	type: "one-time" as TaskType,
	priority: (d.priority ?? "medium") as TaskPriority,
	assigned_to: "",
	// Site/server ids are UUID strings; "" means none.
	site_id: d.site_id ?? "",
	server_id: "",
	organization_id: "" as number | "",
	plugin_slug: d.plugin_slug ?? "",
	interval: "",
	due_date: "",
	reminder_date: "",
});

// The checklist to create the task with; ids are only for the list.
const checklist = ref<ChecklistRow[]>([]);
let nextDraftId = 1;
function addItems(items: NewTaskChecklistItem[]) {
	for (const item of items) {
		checklist.value.push({ id: nextDraftId++, done: false, ...item });
	}
}
function renameItem(item: ChecklistRow, text: string) {
	const it = checklist.value.find((i) => i.id === item.id);
	if (it) it.text = text;
}
function removeItem(item: ChecklistRow) {
	checklist.value = checklist.value.filter((i) => i.id !== item.id);
}
function reorderItems(ids: number[]) {
	const byId = new Map(checklist.value.map((i) => [i.id, i]));
	checklist.value = ids.map((id) => byId.get(id)!).filter(Boolean);
}

onMounted(() => {
	if (!dataStore.isLoaded) dataStore.initData();
	if (orgStore.organizations.length === 0) orgStore.fetchOrganizations();
	userStore.ensureUsers();
	nextTick(() => titleInput.value?.focus());
});

const siteOptions = computed(() => [
	{ value: "", label: "— None —" },
	...dataStore.sites.map((s) => ({ value: s.id, label: s.domain })),
]);
const serverOptions = computed(() => [
	{ value: "", label: "— None —" },
	...dataStore.servers.map((s) => ({ value: s.id, label: s.name })),
]);
const orgOptions = computed(() => [
	{ value: "", label: "— None —" },
	...orgStore.organizations.map((o) => ({ value: o.id, label: o.name })),
]);

function toIso(value: string, endOfDay: boolean): string | null {
	if (!value) return null;
	const [y, m, day] = value.split("-").map(Number);
	return endOfDay
		? new Date(y!, m! - 1, day!, 23, 59, 59).toISOString()
		: new Date(y!, m! - 1, day!, 0, 0, 0).toISOString();
}

async function save() {
	if (!form.title.trim()) {
		saveError.value = "Title is required.";
		return;
	}
	if (form.type !== "one-time" && !form.interval.trim()) {
		saveError.value = "Repeating tasks need an interval.";
		return;
	}

	isSaving.value = true;
	saveError.value = null;
	try {
		const task = await taskStore.createTask({
			title: form.title.trim(),
			description: form.description.trim(),
			type: form.type,
			priority: form.priority,
			assigned_to: form.assigned_to || null,
			site_id: form.site_id || null,
			server_id: form.server_id || null,
			organization_id:
				form.organization_id !== ""
					? Number(form.organization_id)
					: null,
			plugin_slug: form.plugin_slug.trim() || null,
			interval: form.type !== "one-time" ? form.interval.trim() : null,
			due_date: toIso(form.due_date, true),
			reminder_date: toIso(form.reminder_date, false),
			checklist: checklist.value.map((i) => ({
				text: i.text,
				ref_type: i.ref_type,
				ref_id: i.ref_id,
			})),
		});
		emit("saved", task);
	} catch (e: any) {
		saveError.value = e.message || "Failed to save task.";
	} finally {
		isSaving.value = false;
	}
}
</script>

<template>
	<Teleport to="body">
		<div class="modal-overlay" @click.self="emit('close')">
			<form
				class="modal-content card task-form"
				@submit.prevent="save"
				@keydown.ctrl.enter.prevent="save"
				@keydown.meta.enter.prevent="save"
			>
				<header class="task-form-head">
					<h2>New Task</h2>
					<button
						type="button"
						class="modal-close"
						title="Close"
						@click="emit('close')"
					>
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div class="task-form-body">
					<div class="task-form-main">
						<div class="form-group">
							<label for="task-title">
								Title <span class="text-error">*</span>
							</label>
							<input
								id="task-title"
								ref="titleInput"
								v-model="form.title"
								type="text"
								placeholder="What needs doing?"
								maxlength="300"
								required
							/>
						</div>

						<div class="form-group">
							<label for="task-desc">Description</label>
							<textarea
								id="task-desc"
								v-model="form.description"
								placeholder="Optional details"
								rows="4"
							></textarea>
						</div>

						<TaskChecklist
							:items="checklist"
							can-edit
							:can-tick="false"
							draft
							@add="addItems"
							@rename="renameItem"
							@remove="removeItem"
							@reorder="reorderItems"
						/>
					</div>

					<aside class="task-form-side">
						<div class="form-row">
							<div class="form-group">
								<label for="task-priority">Priority</label>
								<select
									id="task-priority"
									v-model="form.priority"
								>
									<option value="low">Low</option>
									<option value="medium">Medium</option>
									<option value="high">High</option>
								</select>
							</div>
							<div class="form-group">
								<label for="task-assigned">Assignee</label>
								<select
									id="task-assigned"
									v-model="form.assigned_to"
								>
									<option value="">Unassigned</option>
									<option
										v-for="user in userStore.users"
										:key="user.username"
										:value="user.username"
									>
										{{ user.displayName }}
									</option>
								</select>
							</div>
						</div>

						<div class="form-row">
							<div class="form-group">
								<label for="task-due">Due</label>
								<input
									id="task-due"
									v-model="form.due_date"
									type="date"
								/>
							</div>
							<div class="form-group">
								<label for="task-reminder">Reminder</label>
								<input
									id="task-reminder"
									v-model="form.reminder_date"
									type="date"
								/>
							</div>
						</div>

						<div class="form-row">
							<div class="form-group">
								<label for="task-type">Repeats</label>
								<select id="task-type" v-model="form.type">
									<option value="one-time">Never</option>
									<option value="repeating">
										From the due date
									</option>
									<option value="dynamic">
										From completion
									</option>
								</select>
							</div>
							<div
								v-if="form.type !== 'one-time'"
								class="form-group"
							>
								<label for="task-interval">
									Every <span class="text-error">*</span>
								</label>
								<input
									id="task-interval"
									v-model="form.interval"
									type="text"
									placeholder="30d, 1w, 1m, 1y"
								/>
							</div>
						</div>

						<h3 class="side-heading">Linked to</h3>
						<div class="form-group">
							<label>Site</label>
							<SearchableSelect
								:model-value="form.site_id"
								:options="siteOptions"
								placeholder="— None —"
								@update:model-value="
									form.site_id = String($event ?? '')
								"
							/>
						</div>
						<div class="form-group">
							<label>Server</label>
							<SearchableSelect
								:model-value="form.server_id"
								:options="serverOptions"
								placeholder="— None —"
								@update:model-value="
									form.server_id = String($event ?? '')
								"
							/>
						</div>
						<div class="form-group">
							<label>Organization</label>
							<SearchableSelect
								:model-value="form.organization_id"
								:options="orgOptions"
								placeholder="— None —"
								@update:model-value="
									form.organization_id =
										$event === '' || $event == null
											? ''
											: Number($event)
								"
							/>
						</div>
						<div class="form-group">
							<label for="task-plugin">Plugin slug</label>
							<input
								id="task-plugin"
								v-model="form.plugin_slug"
								type="text"
								placeholder="plugin-slug"
							/>
						</div>
					</aside>
				</div>

				<footer class="task-form-foot">
					<p v-if="saveError" class="form-error">{{ saveError }}</p>
					<button
						type="button"
						class="btn btn-outline"
						@click="emit('close')"
					>
						Cancel
					</button>
					<button
						type="submit"
						class="btn btn-primary"
						:disabled="isSaving"
					>
						{{ isSaving ? "Creating…" : "Create Task" }}
					</button>
				</footer>
			</form>
		</div>
	</Teleport>
</template>

<style scoped>
.modal-content.task-form {
	max-width: 1040px;
	padding: 0;
	display: flex;
	flex-direction: column;
	overflow: hidden;
}

.task-form-head {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 16px 20px 14px 24px;
	border-bottom: 1px solid var(--border-color);

	& h2 {
		margin: 0;
	}
}

.task-form-body {
	display: grid;
	grid-template-columns: minmax(0, 1fr) 360px;
	min-height: 0;
	flex: 1;
}

.task-form-main {
	padding: 20px 24px;
	overflow-y: auto;

	& textarea {
		resize: vertical;
	}
}

.task-form-side {
	padding: 20px;
	overflow-y: auto;
	border-left: 1px solid var(--border-color);
	background: var(--bg-body);

	& .form-row {
		gap: 12px;
	}
	& .form-group {
		margin-bottom: 12px;
	}
}

.side-heading {
	margin: 8px 0 10px;
	padding-top: 14px;
	border-top: 1px solid var(--border-color);
	font-size: 12px;
	font-weight: 600;
	text-transform: uppercase;
	letter-spacing: 0.04em;
	color: var(--text-muted);
}

.task-form-foot {
	display: flex;
	align-items: center;
	justify-content: flex-end;
	gap: 12px;
	padding: 14px 24px;
	border-top: 1px solid var(--border-color);

	& .form-error {
		margin: 0 auto 0 0;
	}
}

@media (max-width: 860px) {
	.modal-content.task-form {
		overflow-y: auto;
	}
	.task-form-body {
		grid-template-columns: minmax(0, 1fr);
	}
	.task-form-main,
	.task-form-side {
		overflow: visible;
	}
	.task-form-side {
		border-left: none;
		border-top: 1px solid var(--border-color);
	}
}
</style>
