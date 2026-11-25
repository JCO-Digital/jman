<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { RouterLink } from "vue-router";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import { useDataStore } from "../stores/data";
import { useOrganizationStore } from "../stores/organization";
import { useToastStore } from "../stores/toast";
import { useConfirm } from "../composables/useConfirm";
import { useTaskModal } from "../composables/useTaskModal";
import { getVulnerabilityStatus } from "../utils/taskVulnerability";
import AppIcon from "./AppIcon.vue";
import NotesWidget from "./NotesWidget.vue";
import SearchableSelect from "./SearchableSelect.vue";
import TaskChecklist, { type ChecklistRow } from "./TaskChecklist.vue";
import type {
	NewTaskChecklistItem,
	Task,
	TaskChecklistItem,
	TaskStatus,
	UpdateTaskPayload,
} from "../types";

// A task, edited in place: each change is saved as it's made (jman-api
// groups the assignee's and watchers' Slack messages, so this doesn't send
// one per field).

const props = defineProps<{
	task: Task;
	/** The task as read on its own, with its checklist and watchers. */
	details: Task | null;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "updated", task: Task): void;
}>();

const taskStore = useTaskStore();
const authStore = useAuthStore();
const userStore = useUserStore();
const dataStore = useDataStore();
const orgStore = useOrganizationStore();
const toast = useToastStore();
const { confirm } = useConfirm();
const { openTask } = useTaskModal();

const canEdit = computed(() => authStore.canEdit);

onMounted(() => {
	if (!dataStore.isLoaded) dataStore.initData();
	if (orgStore.organizations.length === 0) orgStore.fetchOrganizations();
	userStore.ensureUsers();
	window.addEventListener("keydown", onKeydown);
});
onUnmounted(() => window.removeEventListener("keydown", onKeydown));

function onKeydown(e: KeyboardEvent) {
	if (e.key === "Escape" && !e.defaultPrevented) emit("close");
}

// --- Checklist and watchers, kept here as they change ---

const checklist = ref<TaskChecklistItem[]>([]);
const watchers = ref<string[]>([]);
// Taken from the details once per task: after that the checklist calls
// answer with the whole list, so a later copy of the details may be older.
const loadedFor = ref<number | null>(null);
watch(
	() => props.details,
	(d) => {
		if (!d || d.id !== props.task.id || loadedFor.value === d.id) return;
		loadedFor.value = d.id;
		checklist.value = d.checklist ?? [];
		watchers.value = d.watchers ?? [];
	},
	{ immediate: true },
);

const openItems = computed(() => checklist.value.filter((i) => !i.done).length);

async function checklistCall(fn: () => Promise<TaskChecklistItem[]>) {
	try {
		checklist.value = await fn();
	} catch (e: any) {
		toast.addToast(`Checklist not saved: ${e.message}`, "error");
	}
}

function onToggle(item: ChecklistRow) {
	// item is the listed object itself: read it before ticking it locally.
	const done = !item.done;
	const local = checklist.value.find((i) => i.id === item.id);
	if (local) local.done = done;
	checklistCall(() =>
		taskStore.updateChecklistItem(props.task.id, item.id, { done }),
	);
}
const onRename = (item: ChecklistRow, text: string) =>
	checklistCall(() =>
		taskStore.updateChecklistItem(props.task.id, item.id, { text }),
	);
const onRemove = (item: ChecklistRow) =>
	checklistCall(() => taskStore.deleteChecklistItem(props.task.id, item.id));
const onAdd = (items: NewTaskChecklistItem[]) =>
	checklistCall(() => taskStore.addChecklistItems(props.task.id, items));
function onReorder(ids: number[]) {
	const byId = new Map(checklist.value.map((i) => [i.id, i]));
	checklist.value = ids.map((id) => byId.get(id)!).filter(Boolean);
	checklistCall(() => taskStore.reorderChecklist(props.task.id, ids));
}

const me = computed(() => authStore.user?.username ?? "");
const watching = computed(() => watchers.value.includes(me.value));

async function toggleWatch() {
	try {
		watchers.value = await taskStore.setWatching(
			props.task.id,
			!watching.value,
		);
	} catch (e: any) {
		toast.addToast(`Failed to update watching: ${e.message}`, "error");
	}
}

function onNoteAdded(_note: unknown, watched: boolean) {
	if (watched && !watching.value)
		watchers.value = [...watchers.value, me.value];
}

// --- Saving fields ---

// Bumped when a save fails, to put the controls back to the saved values.
const revision = ref(0);

async function patch(fields: UpdateTaskPayload) {
	try {
		const updated = await taskStore.updateTask(props.task.id, fields);
		emit("updated", updated);
	} catch (e: any) {
		toast.addToast(`Failed to save: ${e.message}`, "error");
		revision.value++;
	}
}

function toDateInput(iso: string | null): string {
	if (!iso) return "";
	const d = new Date(iso);
	const pad = (n: number) => String(n).padStart(2, "0");
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// Due dates are the end of the day, reminders its start (local time).
function fromDateInput(value: string, endOfDay: boolean): string | null {
	if (!value) return null;
	const [y, m, d] = value.split("-").map(Number);
	return endOfDay
		? new Date(y!, m! - 1, d!, 23, 59, 59).toISOString()
		: new Date(y!, m! - 1, d!, 0, 0, 0).toISOString();
}

const inputValue = (e: Event) =>
	(e.target as HTMLInputElement | HTMLSelectElement).value;

// Title
const editingTitle = ref(false);
const titleDraft = ref("");
const titleInput = ref<HTMLInputElement | null>(null);
function startTitle() {
	if (!canEdit.value) return;
	titleDraft.value = props.task.title;
	editingTitle.value = true;
	nextTick(() => titleInput.value?.select());
}
function saveTitle() {
	if (!editingTitle.value) return;
	editingTitle.value = false;
	const title = titleDraft.value.trim();
	if (title && title !== props.task.title) patch({ title });
}

// Description
const editingDesc = ref(false);
const descDraft = ref("");
const descInput = ref<HTMLTextAreaElement | null>(null);
function startDesc() {
	if (!canEdit.value) return;
	descDraft.value = props.task.description ?? "";
	editingDesc.value = true;
	nextTick(() => descInput.value?.focus());
}
function saveDesc() {
	editingDesc.value = false;
	const description = descDraft.value.trim();
	if (description !== (props.task.description ?? "")) patch({ description });
}

// Interval
function saveInterval(e: Event) {
	const interval = inputValue(e).trim();
	if (interval === (props.task.interval ?? "")) return;
	if (!interval) {
		toast.addToast("Repeating tasks need an interval.", "error");
		revision.value++;
		return;
	}
	patch({ interval });
}

function setType(e: Event) {
	const type = inputValue(e) as Task["type"];
	if (type !== "one-time" && !props.task.interval) {
		// Needs an interval first; the field shows up for it.
		pendingType.value = type;
		return;
	}
	pendingType.value = null;
	patch({ type, ...(type === "one-time" ? { interval: null } : {}) });
}
// A repeating type picked before its interval was given.
const pendingType = ref<Task["type"] | null>(null);
const shownType = computed(() => pendingType.value ?? props.task.type);
function saveFirstInterval(e: Event) {
	const interval = inputValue(e).trim();
	if (!interval || !pendingType.value) return;
	const type = pendingType.value;
	pendingType.value = null;
	patch({ type, interval });
}
watch(
	() => props.task.id,
	() => {
		pendingType.value = null;
		editingTitle.value = editingDesc.value = editingLinks.value = false;
		confirmingComplete.value = false;
	},
);

// --- Status ---

const isTerminal = (s: TaskStatus) => s === "completed" || s === "skipped";
const isPaused = (s: TaskStatus) => s === "on_hold" || s === "blocked";
const confirmingComplete = ref(false);

async function complete(force = false) {
	if (!force && openItems.value > 0) {
		confirmingComplete.value = true;
		return;
	}
	confirmingComplete.value = false;
	try {
		const updated = await taskStore.completeTask(props.task.id);
		emit("updated", updated);
		if (updated.type !== "one-time") {
			toast.addToast(
				"Completed — the next one is on the list.",
				"success",
			);
		}
	} catch (e: any) {
		toast.addToast(`Failed to complete task: ${e.message}`, "error");
	}
}

function setStatus(e: Event) {
	const status = inputValue(e) as TaskStatus;
	if (status === "completed") {
		// Completing also starts the next round of a repeating task.
		revision.value++;
		complete();
		return;
	}
	patch({ status });
}

const statusLabels: Record<TaskStatus, string> = {
	pending: "Pending",
	in_progress: "In progress",
	on_hold: "On hold",
	blocked: "Blocked",
	completed: "Completed",
	skipped: "Skipped",
	overdue: "Overdue",
};
// "overdue" is shown from the due date, not picked.
const statusOptions: TaskStatus[] = [
	"pending",
	"in_progress",
	"on_hold",
	"blocked",
	"completed",
	"skipped",
];
const statusClass: Record<TaskStatus, string> = {
	pending: "default",
	in_progress: "active",
	completed: "success",
	skipped: "default",
	on_hold: "warning",
	blocked: "error",
	overdue: "error",
};
const isOverdue = computed(
	() =>
		!!props.task.due_date &&
		new Date(props.task.due_date) < new Date() &&
		!isTerminal(props.task.status) &&
		!isPaused(props.task.status),
);

// --- Links ---

const editingLinks = ref(false);
const site = computed(() =>
	props.task.site_id
		? dataStore.enrichedSites.find((s) => s.id === props.task.site_id)
		: null,
);
const server = computed(() => dataStore.getServerById(props.task.server_id));
const org = computed(() =>
	orgStore.organizations.find((o) => o.id === props.task.organization_id),
);
const hasLinks = computed(
	() =>
		!!props.task.site_id ||
		!!props.task.server_id ||
		props.task.organization_id != null ||
		!!props.task.plugin_slug,
);
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

function savePlugin(e: Event) {
	const slug = inputValue(e).trim();
	if (slug !== (props.task.plugin_slug ?? ""))
		patch({ plugin_slug: slug || null });
}

// --- Vulnerability (hand-made vulnerability tasks) ---

const vuln = computed(() =>
	getVulnerabilityStatus(props.task, dataStore.enrichedSites),
);

// --- Menu ---

const menuOpen = ref(false);
function onMenuBlur(e: FocusEvent) {
	const menu = (e.currentTarget as HTMLElement) ?? null;
	if (!menu?.contains(e.relatedTarget as Node)) menuOpen.value = false;
}

async function copyLink() {
	menuOpen.value = false;
	const url = new URL(window.location.href);
	url.pathname = "/tasks";
	url.search = `?task=${props.task.id}`;
	await navigator.clipboard.writeText(url.toString());
	toast.addToast("Link copied.", "success");
}

async function handleDelete() {
	menuOpen.value = false;
	// The confirm dialog would open behind this modal, so close it first.
	emit("close");
	if (
		!(await confirm(`Delete task "${props.task.title}"?`, { danger: true }))
	)
		return;
	try {
		await taskStore.deleteTask(props.task.id);
	} catch (e: any) {
		toast.addToast(`Failed to delete task: ${e.message}`, "error");
	}
}

function formatDate(d: string | null | undefined, withTime = false) {
	if (!d) return "—";
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "medium",
		...(withTime ? { timeStyle: "short" as const } : {}),
	});
}
</script>

<template>
	<Teleport to="body">
		<div class="modal-overlay" @click.self="emit('close')">
			<div
				class="modal-content card task-modal"
				role="dialog"
				aria-modal="true"
				:aria-label="task.title"
			>
				<header class="task-head">
					<div class="task-head-main">
						<div class="task-kicker">
							<span>Task #{{ task.id }}</span>
							<span
								:class="[
									'status-badge',
									'badge-sm',
									statusClass[task.status],
								]"
								>{{ statusLabels[task.status] }}</span
							>
							<span
								v-if="isOverdue"
								class="status-badge badge-sm error"
								>Overdue</span
							>
						</div>
						<input
							v-if="editingTitle"
							ref="titleInput"
							v-model="titleDraft"
							class="title-input"
							maxlength="300"
							@keydown.enter.prevent="saveTitle"
							@keydown.esc.stop.prevent="editingTitle = false"
							@blur="saveTitle"
						/>
						<h2
							v-else
							class="task-title"
							:class="{ editable: canEdit }"
							:title="canEdit ? 'Click to edit' : undefined"
							@click="startTitle"
						>
							{{ task.title }}
						</h2>
					</div>

					<div class="task-head-actions">
						<button
							class="btn btn-outline btn-sm"
							:class="{ watching }"
							:title="
								watching
									? 'You get Slack messages about this task. Click to stop.'
									: 'Get Slack messages when this task changes or gets a note.'
							"
							@click="toggleWatch"
						>
							<AppIcon name="bell" size="14" />
							{{ watching ? "Watching" : "Watch" }}
						</button>
						<button
							v-if="canEdit && !isTerminal(task.status)"
							class="btn btn-primary btn-sm"
							@click="complete()"
						>
							<AppIcon name="check" size="14" />
							Complete
						</button>
						<button
							v-else-if="canEdit"
							class="btn btn-outline btn-sm"
							@click="patch({ status: 'pending' })"
						>
							Reopen
						</button>
						<div class="menu-wrap" @focusout="onMenuBlur">
							<button
								class="icon-btn"
								title="More"
								aria-haspopup="menu"
								:aria-expanded="menuOpen"
								@click="menuOpen = !menuOpen"
							>
								<AppIcon name="menu" size="18" />
							</button>
							<div v-if="menuOpen" class="task-menu" role="menu">
								<button role="menuitem" @click="copyLink">
									<AppIcon name="copy" size="14" /> Copy link
								</button>
								<button
									v-if="canEdit"
									role="menuitem"
									class="danger"
									@click="handleDelete"
								>
									<AppIcon name="trash" size="14" /> Delete
									task
								</button>
							</div>
						</div>
						<button
							class="modal-close"
							title="Close"
							@click="emit('close')"
						>
							<AppIcon name="x" size="20" />
						</button>
					</div>
				</header>

				<div v-if="confirmingComplete" class="complete-confirm">
					<AppIcon name="alert" size="16" />
					<span>
						{{ openItems }} of {{ checklist.length }} checklist
						{{ checklist.length === 1 ? "item is" : "items are" }}
						still open.
					</span>
					<button
						class="btn btn-primary btn-sm"
						@click="complete(true)"
					>
						Complete anyway
					</button>
					<button
						class="btn btn-outline btn-sm"
						@click="confirmingComplete = false"
					>
						Cancel
					</button>
				</div>

				<div class="task-body">
					<main class="task-main">
						<!-- Description -->
						<section class="task-desc">
							<template v-if="editingDesc">
								<textarea
									ref="descInput"
									v-model="descDraft"
									rows="5"
									placeholder="Describe the task"
									@keydown.esc.stop.prevent="
										editingDesc = false
									"
									@keydown.ctrl.enter="saveDesc"
									@keydown.meta.enter="saveDesc"
								></textarea>
								<div class="flex-row justify-end gap-2 mt-2">
									<button
										class="btn btn-outline btn-sm"
										@click="editingDesc = false"
									>
										Cancel
									</button>
									<button
										class="btn btn-primary btn-sm"
										@click="saveDesc"
									>
										Save
									</button>
								</div>
							</template>
							<p
								v-else-if="task.description"
								class="desc-text"
								:class="{ editable: canEdit }"
								:title="canEdit ? 'Click to edit' : undefined"
								@click="startDesc"
							>
								{{ task.description }}
							</p>
							<button
								v-else-if="canEdit"
								class="desc-empty"
								@click="startDesc"
							>
								Add a description…
							</button>
						</section>

						<!-- Hand-made vulnerability tasks -->
						<div
							v-if="vuln.isVuln"
							:class="[
								'vuln-banner',
								vuln.remaining === 0 ? 'ok' : 'open',
							]"
						>
							<AppIcon
								:name="
									vuln.remaining === 0
										? 'check'
										: 'vulnerability'
								"
								size="16"
							/>
							<span v-if="vuln.remaining === 0">
								The linked vulnerabilities are resolved — ready
								to complete.
							</span>
							<span v-else-if="vuln.type === 'specific'">
								{{ vuln.remaining }} of
								{{ vuln.total }} reported vulnerabilities are
								still active.
							</span>
							<span v-else>
								{{ vuln.totalActiveOnSite }} active
								vulnerabilities on the site.
							</span>
							<RouterLink
								v-if="vuln.remaining > 0 && task.site_id"
								:to="{
									name: 'site-detail',
									params: { id: task.site_id },
								}"
								class="vuln-link"
								@click="emit('close')"
								>View site</RouterLink
							>
						</div>

						<TaskChecklist
							v-if="loadedFor === task.id"
							:items="checklist"
							:can-edit="canEdit"
							:can-tick="canEdit"
							@toggle="onToggle"
							@rename="onRename"
							@remove="onRemove"
							@add="onAdd"
							@reorder="onReorder"
						/>

						<NotesWidget
							parent-type="Task"
							:parent-id="task.id"
							flat
							watch-option
							class="task-notes"
							@added="onNoteAdded"
						/>
					</main>

					<aside :key="revision" class="task-side">
						<dl class="props">
							<dt><label for="tp-status">Status</label></dt>
							<dd>
								<select
									v-if="canEdit"
									id="tp-status"
									:value="task.status"
									@change="setStatus"
								>
									<option
										v-for="value in statusOptions"
										:key="value"
										:value="value"
									>
										{{ statusLabels[value] }}
									</option>
								</select>
								<span v-else>{{
									statusLabels[task.status]
								}}</span>
							</dd>

							<dt><label for="tp-priority">Priority</label></dt>
							<dd>
								<select
									v-if="canEdit"
									id="tp-priority"
									:value="task.priority"
									:class="`prio-${task.priority}`"
									@change="
										patch({
											priority: inputValue(
												$event,
											) as Task['priority'],
										})
									"
								>
									<option value="low">Low</option>
									<option value="medium">Medium</option>
									<option value="high">High</option>
								</select>
								<span v-else>{{ task.priority }}</span>
							</dd>

							<dt><label for="tp-assignee">Assignee</label></dt>
							<dd>
								<select
									v-if="canEdit"
									id="tp-assignee"
									:value="task.assigned_to ?? ''"
									@change="
										patch({
											assigned_to:
												inputValue($event) || null,
										})
									"
								>
									<option value="">Unassigned</option>
									<option
										v-for="u in userStore.users"
										:key="u.username"
										:value="u.username"
									>
										{{ u.displayName }}
									</option>
								</select>
								<span v-else>{{
									userStore.resolveDisplayName(
										task.assigned_to,
									)
								}}</span>
							</dd>

							<dt><label for="tp-due">Due</label></dt>
							<dd>
								<input
									v-if="canEdit"
									id="tp-due"
									type="date"
									:value="toDateInput(task.due_date)"
									:class="{ overdue: isOverdue }"
									@change="
										patch({
											due_date: fromDateInput(
												inputValue($event),
												true,
											),
										})
									"
								/>
								<span v-else :class="{ overdue: isOverdue }">{{
									formatDate(task.due_date)
								}}</span>
							</dd>

							<dt><label for="tp-reminder">Reminder</label></dt>
							<dd>
								<input
									v-if="canEdit"
									id="tp-reminder"
									type="date"
									:value="toDateInput(task.reminder_date)"
									@change="
										patch({
											reminder_date: fromDateInput(
												inputValue($event),
												false,
											),
										})
									"
								/>
								<span v-else>{{
									formatDate(task.reminder_date)
								}}</span>
							</dd>

							<dt><label for="tp-type">Repeats</label></dt>
							<dd>
								<select
									v-if="canEdit"
									id="tp-type"
									:value="shownType"
									@change="setType"
								>
									<option value="one-time">Never</option>
									<option value="repeating">
										From the due date
									</option>
									<option value="dynamic">
										From completion
									</option>
								</select>
								<span v-else>{{ task.type }}</span>
							</dd>

							<template v-if="shownType !== 'one-time'">
								<dt><label for="tp-interval">Every</label></dt>
								<dd>
									<input
										v-if="canEdit && pendingType"
										id="tp-interval"
										type="text"
										placeholder="e.g. 30d, 1w, 1m, 1y"
										@change="saveFirstInterval"
									/>
									<input
										v-else-if="canEdit"
										id="tp-interval"
										type="text"
										:value="task.interval ?? ''"
										placeholder="e.g. 30d, 1w, 1m, 1y"
										@change="saveInterval"
									/>
									<span v-else>{{ task.interval }}</span>
								</dd>
							</template>
						</dl>

						<section class="side-section">
							<header>
								<h3>Linked to</h3>
								<button
									v-if="canEdit"
									class="link-btn"
									@click="editingLinks = !editingLinks"
								>
									{{ editingLinks ? "Done" : "Edit" }}
								</button>
							</header>
							<div v-if="editingLinks" class="link-editor">
								<label>Site</label>
								<SearchableSelect
									:options="siteOptions"
									:model-value="task.site_id ?? ''"
									placeholder="— None —"
									@update:model-value="
										patch({
											site_id: ($event as string) || null,
										})
									"
								/>
								<label>Server</label>
								<SearchableSelect
									:options="serverOptions"
									:model-value="task.server_id ?? ''"
									placeholder="— None —"
									@update:model-value="
										patch({
											server_id:
												($event as string) || null,
										})
									"
								/>
								<label>Organization</label>
								<SearchableSelect
									:options="orgOptions"
									:model-value="task.organization_id ?? ''"
									placeholder="— None —"
									@update:model-value="
										patch({
											organization_id:
												$event === ''
													? null
													: Number($event),
										})
									"
								/>
								<label for="tp-plugin">Plugin slug</label>
								<input
									id="tp-plugin"
									type="text"
									:value="task.plugin_slug ?? ''"
									placeholder="plugin-slug"
									@change="savePlugin"
								/>
							</div>
							<ul v-else-if="hasLinks" class="links">
								<li v-if="task.site_id">
									<AppIcon name="site" size="14" />
									<RouterLink
										:to="{
											name: 'site-detail',
											params: { id: task.site_id },
										}"
										@click="emit('close')"
										>{{
											site?.domain ?? "Unknown site"
										}}</RouterLink
									>
								</li>
								<li v-if="task.server_id">
									<AppIcon name="box" size="14" />
									<span>{{
										server?.name ?? "Unknown server"
									}}</span>
								</li>
								<li v-if="task.organization_id != null">
									<AppIcon name="organization" size="14" />
									<RouterLink
										:to="`/organization/${task.organization_id}`"
										@click="emit('close')"
										>{{
											org?.name ??
											`Organization #${task.organization_id}`
										}}</RouterLink
									>
								</li>
								<li v-if="task.plugin_slug">
									<AppIcon name="plugin" size="14" />
									<RouterLink
										:to="`/plugin/${task.plugin_slug}`"
										@click="emit('close')"
										>{{ task.plugin_slug }}</RouterLink
									>
								</li>
							</ul>
							<p v-else class="muted">Nothing</p>
						</section>

						<section class="side-section">
							<header><h3>Watching</h3></header>
							<p v-if="watchers.length" class="watchers">
								{{
									watchers
										.map((w) =>
											userStore.resolveDisplayName(w),
										)
										.join(", ")
								}}
							</p>
							<p v-else class="muted">No one</p>
						</section>

						<section class="side-section meta">
							<p>
								Created by
								{{
									userStore.resolveDisplayName(
										task.created_by,
									)
								}},
								{{ formatDate(task.created_at) }}
							</p>
							<p v-if="task.completed_at">
								Completed by
								{{
									userStore.resolveDisplayName(
										task.completed_by,
									)
								}},
								{{ formatDate(task.completed_at, true) }}
							</p>
							<p v-if="task.previous_task_id">
								<a
									href="#"
									@click.prevent="
										openTask(task.previous_task_id)
									"
									>Previous round: #{{
										task.previous_task_id
									}}</a
								>
							</p>
						</section>
					</aside>
				</div>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.modal-content.task-modal {
	max-width: 1120px;
	padding: 0;
	display: flex;
	flex-direction: column;
	overflow: hidden;
}

.task-head {
	display: flex;
	align-items: flex-start;
	gap: 16px;
	padding: 18px 20px 16px 24px;
	border-bottom: 1px solid var(--border-color);
}

.task-head-main {
	flex: 1;
	min-width: 0;
}

.task-kicker {
	display: flex;
	align-items: center;
	gap: 8px;
	font-size: 12px;
	color: var(--text-muted);
	margin-bottom: 6px;
}

.modal-content .task-title {
	margin: 0;
	padding: 2px 6px;
	margin-left: -6px;
	font-size: 1.35rem;
	line-height: 1.3;
	border-radius: 4px;
	overflow-wrap: anywhere;

	&.editable:hover {
		background: var(--bg-hover);
		cursor: text;
	}
}

.title-input {
	width: 100%;
	font-size: 1.35rem;
	font-weight: 600;
	padding: 2px 6px;
	margin-left: -6px;
}

.task-head-actions {
	display: flex;
	align-items: center;
	gap: 8px;
	flex-shrink: 0;

	& .watching {
		color: var(--primary);
		border-color: var(--primary);
	}
}

.menu-wrap {
	position: relative;
}

.task-menu {
	position: absolute;
	right: 0;
	top: calc(100% + 4px);
	min-width: 170px;
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 6px;
	box-shadow: var(--shadow-md);
	padding: 4px;
	z-index: 2;

	& button {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		padding: 7px 10px;
		background: none;
		border: none;
		border-radius: 4px;
		font: inherit;
		font-size: 14px;
		color: var(--text-main);
		cursor: pointer;
		text-align: left;

		&:hover {
			background: var(--bg-hover);
		}
		&.danger {
			color: var(--error-text);
		}
	}
}

.complete-confirm {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: 10px;
	padding: 10px 24px;
	background: var(--warning-bg);
	color: var(--warning-text);
	font-size: 14px;

	& span {
		margin-right: auto;
	}
}

.task-body {
	display: grid;
	grid-template-columns: minmax(0, 1fr) 320px;
	min-height: 0;
	flex: 1;
}

.task-main {
	padding: 20px 24px 28px;
	overflow-y: auto;
	display: flex;
	flex-direction: column;
	gap: 24px;
}

.task-side {
	padding: 16px 20px 24px;
	overflow-y: auto;
	border-left: 1px solid var(--border-color);
	background: var(--bg-body);
	display: flex;
	flex-direction: column;
	gap: 20px;
}

.desc-text {
	margin: 0;
	white-space: pre-wrap;
	line-height: 1.55;
	padding: 6px 8px;
	margin: -6px -8px;
	border-radius: 6px;
	overflow-wrap: anywhere;

	&.editable:hover {
		background: var(--bg-hover);
		cursor: text;
	}
}

.task-desc textarea {
	width: 100%;
	resize: vertical;
}

.desc-empty {
	background: none;
	border: 1px dashed var(--border-input);
	border-radius: 6px;
	padding: 10px 12px;
	width: 100%;
	text-align: left;
	font: inherit;
	font-size: 14px;
	color: var(--text-muted);
	cursor: text;
}

.vuln-banner {
	display: flex;
	align-items: center;
	gap: 10px;
	padding: 10px 14px;
	border-radius: 6px;
	font-size: 14px;
	border-left: 4px solid;

	&.ok {
		background: var(--badge-active-bg);
		border-color: var(--badge-active-text);
	}
	&.open {
		background: var(--warning-bg);
		border-color: var(--warning-border);
	}
}

.vuln-link {
	margin-left: auto;
	white-space: nowrap;
}

.task-notes :deep(.card-header) {
	margin-bottom: 8px;
}
.task-notes :deep(h2) {
	font-size: 14px;
}

/* Properties: label column and a control that looks like text until
   it's pointed at. */
.props {
	display: grid;
	grid-template-columns: 76px minmax(0, 1fr);
	align-items: center;
	gap: 4px 8px;
	margin: 0;

	& dt {
		font-size: 12px;
		color: var(--text-muted);

		& label {
			font-weight: 500;
		}
	}
	& dd {
		margin: 0;
		font-size: 14px;
		min-height: 32px;
		display: flex;
		align-items: center;
	}
	& select,
	& input {
		width: 100%;
		padding: 5px 8px;
		font-size: 14px;
		border: 1px solid transparent;
		background-color: transparent;
		border-radius: 4px;

		&:hover {
			border-color: var(--border-input);
		}
		&:focus {
			border-color: var(--primary);
			background-color: var(--bg-card);
		}
	}
	& .overdue {
		color: var(--error-text);
		font-weight: 600;
	}
	& .prio-high {
		color: var(--error-text);
		font-weight: 600;
	}
	& .prio-medium {
		color: var(--warning-text);
	}
}

.side-section {
	border-top: 1px solid var(--border-color);
	padding-top: 14px;

	& header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 8px;
	}
	& h3 {
		margin: 0;
		font-size: 12px;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-muted);
	}
	& p {
		margin: 0;
		font-size: 14px;
	}
	& .muted {
		color: var(--text-muted);
	}
}

.link-btn {
	background: none;
	border: none;
	color: var(--primary);
	font: inherit;
	font-size: 13px;
	cursor: pointer;
	padding: 0;
}

.links {
	list-style: none;
	margin: 0;
	padding: 0;
	display: flex;
	flex-direction: column;
	gap: 6px;
	font-size: 14px;

	& li {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
		overflow-wrap: anywhere;
		color: var(--text-muted);
	}
	& a,
	& span {
		color: var(--text-main);
	}
	& a:hover {
		color: var(--primary);
	}
}

.link-editor {
	display: flex;
	flex-direction: column;
	gap: 4px;

	& label {
		font-size: 12px;
		color: var(--text-muted);
		margin-top: 6px;
	}
}

.meta p {
	font-size: 12px;
	color: var(--text-muted);
	margin-bottom: 4px;
}

@media (max-width: 860px) {
	.modal-content.task-modal {
		overflow-y: auto;
	}
	.task-body {
		grid-template-columns: minmax(0, 1fr);
	}
	.task-main,
	.task-side {
		overflow: visible;
	}
	.task-side {
		border-left: none;
		border-top: 1px solid var(--border-color);
	}
	.task-head {
		flex-wrap: wrap;
		padding: 14px 16px;
	}
	.task-head-actions {
		width: 100%;
		justify-content: flex-end;
	}
	.task-main {
		padding: 16px;
	}
}
</style>
