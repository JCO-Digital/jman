<script setup lang="ts">
import { ref } from "vue";
import { RouterLink } from "vue-router";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import type { Task, TaskStatus } from "../types";

const props = defineProps<{
	task: Task;
}>();

const emit = defineEmits<{
	(e: "close"): void;
	(e: "edit", task: Task): void;
	(e: "updated", task: Task): void;
	(e: "deleted"): void;
}>();

const taskStore = useTaskStore();
const authStore = useAuthStore();

const isActioning = ref(false);
const actionError = ref<string | null>(null);

async function complete() {
	isActioning.value = true;
	actionError.value = null;
	try {
		await taskStore.completeTask(props.task.id);
		emit("updated", { ...props.task, status: "completed" });
	} catch (e: any) {
		actionError.value = e.message;
	} finally {
		isActioning.value = false;
	}
}

async function changeStatus(status: TaskStatus) {
	isActioning.value = true;
	actionError.value = null;
	try {
		const updated = await taskStore.setStatus(props.task.id, status);
		emit("updated", updated);
	} catch (e: any) {
		actionError.value = e.message;
	} finally {
		isActioning.value = false;
	}
}

const isDeleting = ref(false);

async function handleDelete() {
	if (!confirm(`Delete task "${props.task.title}"?`)) return;
	isDeleting.value = true;
	try {
		await taskStore.deleteTask(props.task.id);
		emit("deleted");
	} catch (e: any) {
		actionError.value = e.message;
	} finally {
		isDeleting.value = false;
	}
}

function formatDate(d: string | null) {
	if (!d) return "—";
	return new Date(d).toLocaleString("de-DE", {
		dateStyle: "medium",
		timeStyle: "short",
	});
}

const priorityClass: Record<string, string> = {
	low: "default",
	medium: "warning",
	high: "error",
};

const statusClass: Record<string, string> = {
	pending: "default",
	in_progress: "active",
	completed: "success",
	skipped: "default",
	overdue: "error",
};

const isTerminal = (s: string) => s === "completed" || s === "skipped";
const canComplete = (s: string) =>
	s === "pending" || s === "in_progress" || s === "overdue";
</script>

<template>
	<Teleport to="body">
		<div class="modal-overlay" @click.self="emit('close')">
			<div class="modal-card">
				<header class="modal-header">
					<div class="header-badges">
						<span
							:class="['status-badge', statusClass[task.status]]"
						>
							{{ task.status.replace("_", " ") }}
						</span>
						<span
							:class="['status-badge', priorityClass[task.priority]]"
						>
							{{ task.priority }}
						</span>
						<span class="status-badge default">{{ task.type }}</span>
					</div>
					<h3>{{ task.title }}</h3>
				</header>

				<div class="modal-body">
					<p v-if="task.description" class="description">
						{{ task.description }}
					</p>

					<div class="info-grid">
						<div class="info-row">
							<span class="info-label">Assigned to</span>
							<span class="info-value">{{
								task.assigned_to ?? "—"
							}}</span>
						</div>
						<div class="info-row">
							<span class="info-label">Due date</span>
							<span
								class="info-value"
								:class="{
									overdue:
										task.due_date &&
										new Date(task.due_date) < new Date() &&
										!isTerminal(task.status),
								}"
							>
								{{ formatDate(task.due_date) }}
							</span>
						</div>
						<div class="info-row">
							<span class="info-label">Reminder date</span>
							<span class="info-value">{{
								formatDate(task.reminder_date)
							}}</span>
						</div>
						<div v-if="task.interval" class="info-row">
							<span class="info-label">Interval</span>
							<span class="info-value">{{ task.interval }}</span>
						</div>
						<div class="info-row">
							<span class="info-label">Created by</span>
							<span class="info-value">{{ task.created_by }}</span>
						</div>
						<div class="info-row">
							<span class="info-label">Created</span>
							<span class="info-value">{{
								formatDate(task.created_at)
							}}</span>
						</div>
					</div>

					<!-- Linked entities -->
					<div
						v-if="
							task.site_id ||
							task.server_id ||
							task.organization_id ||
							task.plugin_slug
						"
						class="linked-entities"
					>
						<h4>Linked to</h4>
						<div class="info-grid">
							<div v-if="task.site_id" class="info-row">
								<span class="info-label">Site</span>
								<RouterLink
									:to="`/site/${task.site_id}`"
									class="entity-link"
									@click="emit('close')"
								>
									Site #{{ task.site_id }}
								</RouterLink>
							</div>
							<div v-if="task.server_id" class="info-row">
								<span class="info-label">Server</span>
								<span class="info-value"
									>Server #{{ task.server_id }}</span
								>
							</div>
							<div v-if="task.organization_id" class="info-row">
								<span class="info-label">Organization</span>
								<RouterLink
									:to="`/organization/${task.organization_id}`"
									class="entity-link"
									@click="emit('close')"
								>
									Organization #{{ task.organization_id }}
								</RouterLink>
							</div>
							<div v-if="task.plugin_slug" class="info-row">
								<span class="info-label">Plugin</span>
								<RouterLink
									:to="`/plugin/${task.plugin_slug}`"
									class="entity-link"
									@click="emit('close')"
								>
									{{ task.plugin_slug }}
								</RouterLink>
							</div>
						</div>
					</div>

					<!-- Status actions -->
					<div v-if="!isTerminal(task.status)" class="status-actions">
						<h4>Actions</h4>
						<div class="action-buttons">
							<button
								v-if="canComplete(task.status)"
								class="btn btn-primary"
								:disabled="isActioning"
								@click="complete"
							>
								Complete
							</button>
							<button
								v-if="task.status === 'pending'"
								class="btn btn-secondary"
								:disabled="isActioning"
								@click="changeStatus('in_progress')"
							>
								Start
							</button>
							<button
								v-if="
									task.status === 'pending' ||
									task.status === 'in_progress' ||
									task.status === 'overdue'
								"
								class="btn btn-secondary"
								:disabled="isActioning"
								@click="changeStatus('skipped')"
							>
								Skip
							</button>
						</div>
						<p v-if="actionError" class="action-error">
							{{ actionError }}
						</p>
					</div>
				</div>

				<footer class="modal-footer">
					<div class="footer-left">
						<button
							v-if="authStore.canEdit"
							class="btn btn-danger"
							:disabled="isDeleting"
							@click="handleDelete"
						>
							{{ isDeleting ? "Deleting…" : "Delete" }}
						</button>
					</div>
					<div class="footer-right">
						<button class="btn btn-cancel" @click="emit('close')">
							Close
						</button>
						<button
							v-if="authStore.canEdit"
							class="btn btn-primary"
							@click="emit('edit', task)"
						>
							Edit
						</button>
					</div>
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
	max-width: 600px;
	max-height: 90vh;
	overflow-y: auto;
	display: flex;
	flex-direction: column;
}

.modal-header {
	padding: 20px 24px 16px;
	border-bottom: 1px solid var(--border-color);
}

.header-badges {
	display: flex;
	gap: 8px;
	margin-bottom: 10px;
	flex-wrap: wrap;
}

.modal-header h3 {
	margin: 0;
	font-size: 18px;
	color: var(--text-heading);
	line-height: 1.3;
}

.modal-body {
	padding: 20px 24px;
	flex: 1;
	display: flex;
	flex-direction: column;
	gap: 20px;
}

.description {
	margin: 0;
	color: var(--text-main);
	line-height: 1.6;
	white-space: pre-wrap;
}

.info-grid {
	display: flex;
	flex-direction: column;
	gap: 8px;
}

.info-row {
	display: flex;
	gap: 12px;
	align-items: baseline;
}

.info-label {
	font-size: 13px;
	color: var(--text-muted);
	min-width: 120px;
	flex-shrink: 0;
}

.info-value {
	font-size: 14px;
	color: var(--text-main);
}

.linked-entities h4,
.status-actions h4 {
	margin: 0 0 10px;
	font-size: 13px;
	font-weight: 600;
	color: var(--text-muted);
	text-transform: uppercase;
	letter-spacing: 0.05em;
}

.entity-link {
	color: var(--primary);
	text-decoration: none;
	font-size: 14px;
}

.entity-link:hover {
	text-decoration: underline;
}

.action-buttons {
	display: flex;
	gap: 10px;
	flex-wrap: wrap;
}

.action-error {
	margin: 8px 0 0;
	color: var(--error-text);
	font-size: 13px;
}

.modal-footer {
	padding: 16px 24px 20px;
	border-top: 1px solid var(--border-color);
	display: flex;
	justify-content: space-between;
	align-items: center;
	gap: 12px;
}

.footer-right {
	display: flex;
	gap: 10px;
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

.btn-secondary {
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

.btn-secondary:hover:not(:disabled) {
	background-color: var(--bg-hover);
}

.btn-danger {
	padding: 8px 16px;
	border: 1px solid var(--error-border);
	border-radius: 4px;
	background: transparent;
	color: var(--error-text);
	cursor: pointer;
	font-weight: 500;
	font-size: 14px;
	transition: background-color 0.2s;
}

.btn-danger:hover:not(:disabled) {
	background-color: var(--error-bg);
}

.btn-danger:disabled,
.btn-secondary:disabled {
	opacity: 0.5;
	cursor: not-allowed;
}

.overdue {
	color: var(--error-text);
	font-weight: 600;
}
</style>
