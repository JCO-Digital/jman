<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useUserStore } from "../stores/user";
import type { Task, TaskType, TaskPriority } from "../types";

const props = defineProps<{
	task?: Task | null;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "saved", task: Task): void;
}>();

const taskStore = useTaskStore();
const dataStore = useDataStore();
const orgStore = useOrganizationStore();
const userStore = useUserStore();

const isEditing = computed(() => !!props.task);
const isSaving = ref(false);
const saveError = ref<string | null>(null);

const form = reactive({
	title: "",
	description: "",
	type: "one-time" as TaskType,
	priority: "medium" as TaskPriority,
	assigned_to: "",
	site_id: "" as number | "",
	server_id: "" as number | "",
	organization_id: "" as number | "",
	plugin_slug: "",
	interval: "",
	due_date: "",
	reminder_date: "",
});

function toDateInput(iso: string | null): string {
	if (!iso) return "";
	const d = new Date(iso);
	const pad = (n: number) => String(n).padStart(2, "0");
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function resetForm() {
	if (props.task) {
		form.title = props.task.title;
		form.description = props.task.description ?? "";
		form.type = props.task.type;
		form.priority = props.task.priority;
		form.assigned_to = props.task.assigned_to ?? "";
		form.site_id = props.task.site_id ?? "";
		form.server_id = props.task.server_id ?? "";
		form.organization_id = props.task.organization_id ?? "";
		form.plugin_slug = props.task.plugin_slug ?? "";
		form.interval = props.task.interval ?? "";
		form.due_date = toDateInput(props.task.due_date);
		form.reminder_date = toDateInput(props.task.reminder_date);
	} else {
		form.title = "";
		form.description = "";
		form.type = "one-time";
		form.priority = "medium";
		form.assigned_to = "";
		form.site_id = "";
		form.server_id = "";
		form.organization_id = "";
		form.plugin_slug = "";
		form.interval = "";
		form.due_date = "";
		form.reminder_date = "";
	}
}

watch(() => props.task, resetForm, { immediate: true });

onMounted(() => {
	if (!dataStore.isLoaded) dataStore.initData();
	if (orgStore.organizations.length === 0) orgStore.fetchOrganizations();
	userStore.ensureUsers();
});

function toIso(
	value: string,
	hours: number,
	minutes: number,
	seconds: number,
): string | null {
	if (!value) return null;
	const parts = value.split("-");
	const year = Number(parts[0]);
	const month = Number(parts[1]);
	const day = Number(parts[2]);
	return new Date(year, month - 1, day, hours, minutes, seconds, 0).toISOString();
}

async function save() {
	if (!form.title.trim()) {
		saveError.value = "Title is required.";
		return;
	}
	if (
		(form.type === "repeating" || form.type === "dynamic") &&
		!form.interval.trim()
	) {
		saveError.value = "Interval is required for repeating/dynamic tasks.";
		return;
	}

	isSaving.value = true;
	saveError.value = null;

	const payload = {
		title: form.title.trim(),
		description: form.description.trim() || "",
		type: form.type,
		priority: form.priority,
		assigned_to: form.assigned_to.trim() || null,
		site_id: form.site_id !== "" ? Number(form.site_id) : null,
		server_id: form.server_id !== "" ? Number(form.server_id) : null,
		organization_id:
			form.organization_id !== "" ? Number(form.organization_id) : null,
		plugin_slug: form.plugin_slug.trim() || null,
		interval:
			form.type !== "one-time" ? form.interval.trim() || null : null,
		due_date: toIso(form.due_date, 23, 59, 59),
		reminder_date: toIso(form.reminder_date, 0, 0, 0),
	};

	try {
		let result: Task;
		if (isEditing.value && props.task) {
			result = await taskStore.updateTask(props.task.id, payload);
		} else {
			result = await taskStore.createTask(payload);
		}
		emit("saved", result);
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
			<div class="modal-card">
				<header class="modal-header">
					<h3>{{ isEditing ? "Edit Task" : "New Task" }}</h3>
				</header>

				<form class="modal-body" @submit.prevent="save">
					<div class="form-group">
						<label for="task-title"
							>Title <span class="required">*</span></label
						>
						<input
							id="task-title"
							v-model="form.title"
							type="text"
							class="form-input"
							placeholder="Task title"
							required
						/>
					</div>

					<div class="form-group">
						<label for="task-desc">Description</label>
						<textarea
							id="task-desc"
							v-model="form.description"
							class="form-input form-textarea"
							placeholder="Optional description"
							rows="3"
						/>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="task-type">Type</label>
							<select
								id="task-type"
								v-model="form.type"
								class="form-input"
							>
								<option value="one-time">One-time</option>
								<option value="repeating">Repeating</option>
								<option value="dynamic">Dynamic</option>
							</select>
						</div>

						<div class="form-group">
							<label for="task-priority">Priority</label>
							<select
								id="task-priority"
								v-model="form.priority"
								class="form-input"
							>
								<option value="low">Low</option>
								<option value="medium">Medium</option>
								<option value="high">High</option>
							</select>
						</div>
					</div>

					<div
						v-if="
							form.type === 'repeating' || form.type === 'dynamic'
						"
						class="form-group"
					>
						<label for="task-interval">
							Interval <span class="required">*</span>
						</label>
						<input
							id="task-interval"
							v-model="form.interval"
							type="text"
							class="form-input"
							placeholder="e.g. 30d, 1w, 1m, 1y"
						/>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="task-due">Due Date</label>
							<input
								id="task-due"
								v-model="form.due_date"
								type="date"
								class="form-input"
							/>
						</div>

						<div class="form-group">
							<label for="task-reminder">Reminder Date</label>
							<input
								id="task-reminder"
								v-model="form.reminder_date"
								type="date"
								class="form-input"
							/>
						</div>
					</div>

					<div class="form-group">
						<label for="task-assigned">Assigned To</label>
						<select
							id="task-assigned"
							v-model="form.assigned_to"
							class="form-input"
						>
							<option value="">— Unassigned —</option>
							<option
								v-for="user in userStore.users"
								:key="user.username"
								:value="user.username"
							>
								{{ user.displayName }}
							</option>
						</select>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="task-site">Site</label>
							<select
								id="task-site"
								v-model="form.site_id"
								class="form-input"
							>
								<option value="">— None —</option>
								<option
									v-for="site in dataStore.sites"
									:key="site.id"
									:value="site.id"
								>
									{{ site.domain }}
								</option>
							</select>
						</div>

						<div class="form-group">
							<label for="task-server">Server</label>
							<select
								id="task-server"
								v-model="form.server_id"
								class="form-input"
							>
								<option value="">— None —</option>
								<option
									v-for="server in dataStore.servers"
									:key="server.id"
									:value="server.id"
								>
									{{ server.name }}
								</option>
							</select>
						</div>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="task-org">Organization</label>
							<select
								id="task-org"
								v-model="form.organization_id"
								class="form-input"
							>
								<option value="">— None —</option>
								<option
									v-for="org in orgStore.organizations"
									:key="org.id"
									:value="org.id"
								>
									{{ org.name }}
								</option>
							</select>
						</div>

						<div class="form-group">
							<label for="task-plugin">Plugin Slug</label>
							<input
								id="task-plugin"
								v-model="form.plugin_slug"
								type="text"
								class="form-input"
								placeholder="plugin-slug"
							/>
						</div>
					</div>

					<p v-if="saveError" class="save-error">{{ saveError }}</p>
				</form>

				<footer class="modal-footer">
					<button
						type="button"
						class="btn btn-cancel"
						@click="emit('close')"
					>
						Cancel
					</button>
					<button
						type="button"
						class="btn btn-primary"
						:disabled="isSaving"
						@click="save"
					>
						{{
							isSaving
								? "Saving…"
								: isEditing
									? "Save Changes"
									: "Create Task"
						}}
					</button>
				</footer>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.modal-overlay {
	position: fixed;
	inset: 0;
	background: rgba(0, 0, 0, 0.5);
	display: flex;
	align-items: center;
	justify-content: center;
	z-index: 1000;
	padding: 16px;
}

.modal-card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 8px;
	width: 100%;
	max-width: 640px;
	max-height: 90vh;
	overflow-y: auto;
	display: flex;
	flex-direction: column;
}

.modal-header {
	padding: 20px 24px 16px;
	border-bottom: 1px solid var(--border-color);
}

.modal-header h3 {
	margin: 0;
	font-size: 18px;
	color: var(--text-heading);
}

.modal-body {
	padding: 20px 24px;
	flex: 1;
	display: flex;
	flex-direction: column;
	gap: 16px;
}

.form-row {
	display: grid;
	grid-template-columns: 1fr 1fr;
	gap: 16px;
}

@media (max-width: 480px) {
	.form-row {
		grid-template-columns: 1fr;
	}
}

.form-group {
	display: flex;
	flex-direction: column;
	gap: 6px;
}

.form-group label {
	font-size: 13px;
	font-weight: 500;
	color: var(--text-muted);
}

.required {
	color: var(--error-text);
}

.form-input {
	padding: 8px 10px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	background: var(--bg-input);
	color: var(--text-main);
	font-size: 14px;
	width: 100%;
	box-sizing: border-box;
}

.form-input:focus {
	outline: none;
	border-color: var(--primary);
}

.form-textarea {
	resize: vertical;
	font-family: inherit;
}

.save-error {
	color: var(--error-text);
	font-size: 13px;
	margin: 0;
}

.modal-footer {
	padding: 16px 24px 20px;
	border-top: 1px solid var(--border-color);
	display: flex;
	justify-content: flex-end;
	gap: 12px;
}

.btn-cancel {
	padding: 8px 16px;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	background: var(--bg-card);
	color: var(--text-main);
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	transition: background-color 0.2s;
}

.btn-cancel:hover {
	background-color: var(--bg-hover);
}
</style>
