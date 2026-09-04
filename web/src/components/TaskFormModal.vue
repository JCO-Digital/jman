<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from "vue";
import { useTaskStore } from "../stores/tasks";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useUserStore } from "../stores/user";
import AppIcon from "./AppIcon.vue";
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
	return new Date(
		year,
		month - 1,
		day,
		hours,
		minutes,
		seconds,
		0,
	).toISOString();
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
			<div class="modal-content card">
				<header class="modal-header">
					<h2>{{ isEditing ? "Edit Task" : "New Task" }}</h2>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<form class="content" @submit.prevent="save">
					<div class="form-group">
						<label for="task-title">
							Title <span class="text-error">*</span>
						</label>
						<input
							id="task-title"
							v-model="form.title"
							type="text"
							placeholder="Task title"
							required
						/>
					</div>

					<div class="form-group">
						<label for="task-desc">Description</label>
						<textarea
							id="task-desc"
							v-model="form.description"
							placeholder="Optional description"
							rows="3"
						></textarea>
					</div>

					<div class="form-row">
						<div class="form-group">
							<label for="task-type">Type</label>
							<select id="task-type" v-model="form.type">
								<option value="one-time">One-time</option>
								<option value="repeating">Repeating</option>
								<option value="dynamic">Dynamic</option>
							</select>
						</div>

						<div class="form-group">
							<label for="task-priority">Priority</label>
							<select id="task-priority" v-model="form.priority">
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
							Interval <span class="text-error">*</span>
						</label>
						<input
							id="task-interval"
							v-model="form.interval"
							type="text"
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
							/>
						</div>

						<div class="form-group">
							<label for="task-reminder">Reminder Date</label>
							<input
								id="task-reminder"
								v-model="form.reminder_date"
								type="date"
							/>
						</div>
					</div>

					<div class="form-group">
						<label for="task-assigned">Assigned To</label>
						<select id="task-assigned" v-model="form.assigned_to">
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
							<select id="task-site" v-model="form.site_id">
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
							<select id="task-server" v-model="form.server_id">
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
								placeholder="plugin-slug"
							/>
						</div>
					</div>

					<div v-if="saveError" class="error-banner">
						<p>{{ saveError }}</p>
					</div>

					<footer class="form-actions">
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
							{{
								isSaving
									? "Saving…"
									: isEditing
										? "Save Changes"
										: "Create Task"
							}}
						</button>
					</footer>
				</form>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
/* Scoped styles removed in favor of global modal and form classes */
</style>
