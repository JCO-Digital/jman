<script setup lang="ts">
import { ref, computed } from "vue";
import { RouterLink } from "vue-router";
import { useTaskStore } from "../stores/tasks";
import { useAuthStore } from "../stores/auth";
import { useUserStore } from "../stores/user";
import { useDataStore } from "../stores/data";
import AppIcon from "./AppIcon.vue";
import type { Task, TaskStatus } from "../types";
import { useConfirm } from "../composables/useConfirm";

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
const userStore = useUserStore();
const dataStore = useDataStore();
const { confirm } = useConfirm();

// Get the enriched site corresponding to this task's site_id
const site = computed(() => {
	if (!props.task.site_id) return null;
	return (
		dataStore.enrichedSites.find((s) => s.id === props.task.site_id) || null
	);
});

// Parse task metadata to get original vulnerability UUIDs
const metadata = computed(() => {
	if (!props.task.metadata) return null;
	try {
		return JSON.parse(props.task.metadata);
	} catch {
		return null;
	}
});

const originalVulnUuids = computed<string[]>(() => {
	return metadata.value?.vuln_uuids || [];
});

// A task is a vulnerability-derived task if it has a site_id AND has vuln_uuids in metadata,
// OR if the title starts with "Security Vulnerabilities".
const isVulnerabilityTask = computed(() => {
	return (
		props.task.site_id !== null &&
		(originalVulnUuids.value.length > 0 ||
			props.task.title
				.toLowerCase()
				.startsWith("security vulnerabilities"))
	);
});

// Total active (unsuppressed) vulnerabilities currently on the site
const activeVulnerabilitiesCount = computed(() => {
	if (!site.value) return 0;
	return site.value.vulnerabilities.filter((v) => !v.suppressed).length;
});

// Count how many of the originally reported vulnerabilities remain active (unsuppressed) on the site
const remainingOriginalVulnsCount = computed(() => {
	if (!site.value) return 0;
	const activeVulns = site.value.vulnerabilities.filter((v) => !v.suppressed);
	if (originalVulnUuids.value.length === 0) {
		// Fallback if we don't have metadata but it's a vulnerability task:
		// count all current active vulnerabilities for the site as the remaining
		return activeVulns.length;
	}
	return activeVulns.filter((v) => originalVulnUuids.value.includes(v.uuid))
		.length;
});

// Flag to indicate if all vulnerabilities (or at least all original ones) are resolved
const isVulnerabilityReadyToClose = computed(() => {
	if (!isVulnerabilityTask.value) return false;

	if (originalVulnUuids.value.length > 0) {
		// If we have the specific UUIDs, the task is ready to close when those are all resolved
		return remainingOriginalVulnsCount.value === 0;
	} else {
		// Fallback: ready to close when the site has 0 active vulnerabilities
		return activeVulnerabilitiesCount.value === 0;
	}
});

const isActioning = ref(false);
const actionError = ref<string | null>(null);

async function complete() {
	isActioning.value = true;
	actionError.value = null;
	try {
		const updated = await taskStore.completeTask(props.task.id);
		emit("updated", updated);
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
	if (
		!(await confirm(`Delete task "${props.task.title}"?`, { danger: true }))
	)
		return;
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

function formatDate(d: string | null, includeTime = false) {
	if (!d) return "—";
	const options: Intl.DateTimeFormatOptions = { dateStyle: "medium" };
	if (includeTime) {
		options.timeStyle = "short";
	}
	return new Date(d).toLocaleString("de-DE", options);
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
			<div class="modal-content card">
				<header class="modal-header">
					<div class="title-area">
						<div class="flex-row gap-2 mb-4">
							<span
								:class="[
									'status-badge',
									'badge-sm',
									statusClass[task.status],
								]"
							>
								{{ task.status.replace("_", " ") }}
							</span>
							<span
								:class="[
									'status-badge',
									'badge-sm',
									priorityClass[task.priority],
								]"
							>
								{{ task.priority }}
							</span>
							<span class="status-badge badge-sm info">{{
								task.type
							}}</span>
						</div>
						<h2>{{ task.title }}</h2>
					</div>
					<button class="modal-close" @click="emit('close')">
						<AppIcon name="x" size="20" />
					</button>
				</header>

				<div class="content">
					<p v-if="task.description" class="description">
						{{ task.description }}
					</p>

					<div class="info-grid">
						<div class="info-item">
							<span class="label">Assigned to</span>
							<span class="value">{{
								userStore.resolveDisplayName(task.assigned_to)
							}}</span>
						</div>
						<div class="info-item">
							<span class="label">Due date</span>
							<span
								class="value"
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
						<div class="info-item">
							<span class="label">Reminder date</span>
							<span class="value">{{
								formatDate(task.reminder_date)
							}}</span>
						</div>
						<div v-if="task.interval" class="info-item">
							<span class="label">Interval</span>
							<span class="value">{{ task.interval }}</span>
						</div>
					</div>

					<div class="section-divider"></div>

					<div class="grid-2-cols gap-4">
						<div class="info-item">
							<span class="label">Created by</span>
							<span class="value">{{ task.created_by }}</span>
						</div>
						<div class="info-item">
							<span class="label">Created at</span>
							<span class="value">{{
								formatDate(task.created_at)
							}}</span>
						</div>
					</div>

					<div
						v-if="task.status === 'completed'"
						class="grid-2-cols gap-4 mt-4"
					>
						<div class="info-item">
							<span class="label">Completed by</span>
							<span class="value">{{
								userStore.resolveDisplayName(task.completed_by)
							}}</span>
						</div>
						<div class="info-item">
							<span class="label">Completed at</span>
							<span class="value">{{
								formatDate(task.completed_at, true)
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
						class="section-divider"
					>
						<h3 class="sub-text font-medium mb-4">Linked to</h3>
						<div class="info-grid">
							<div v-if="task.site_id" class="info-item">
								<span class="label">Site</span>
								<RouterLink
									:to="`/site/${task.site_id}`"
									class="font-sm font-medium"
									@click="emit('close')"
								>
									{{
										site
											? site.domain
											: `Site #${task.site_id}`
									}}
								</RouterLink>
							</div>
							<div v-if="task.server_id" class="info-item">
								<span class="label">Server</span>
								<span class="value"
									>Server #{{ task.server_id }}</span
								>
							</div>
							<div v-if="task.organization_id" class="info-item">
								<span class="label">Organization</span>
								<RouterLink
									:to="`/organization/${task.organization_id}`"
									class="font-sm font-medium"
									@click="emit('close')"
								>
									Organization #{{ task.organization_id }}
								</RouterLink>
							</div>
							<div v-if="task.plugin_slug" class="info-item">
								<span class="label">Plugin</span>
								<RouterLink
									:to="`/plugin/${task.plugin_slug}`"
									class="font-sm font-medium"
									@click="emit('close')"
								>
									{{ task.plugin_slug }}
								</RouterLink>
							</div>
						</div>
					</div>

					<!-- Vulnerability Status Indicator -->
					<div v-if="isVulnerabilityTask" class="section-divider">
						<h3 class="sub-text font-medium mb-4">
							Vulnerability Status Check
						</h3>

						<div
							v-if="isVulnerabilityReadyToClose"
							class="status-banner success"
						>
							<div class="banner-icon">
								<AppIcon name="check" size="20" />
							</div>
							<div class="banner-text">
								<strong>Ready to Close</strong>
								<p v-if="originalVulnUuids.length > 0">
									All {{ originalVulnUuids.length }} linked
									vulnerabilities for this site have been
									resolved! You can now complete this task.
								</p>
								<p v-else>
									No active vulnerabilities remain on this
									site.
								</p>
							</div>
						</div>

						<div v-else class="status-banner warning">
							<div class="banner-icon">
								<AppIcon name="vulnerability" size="20" />
							</div>
							<div class="banner-text">
								<strong>Action Required</strong>
								<p v-if="originalVulnUuids.length > 0">
									{{ remainingOriginalVulnsCount }} of
									{{ originalVulnUuids.length }} originally
									reported vulnerabilities are still active.
								</p>
								<p v-else>
									{{ activeVulnerabilitiesCount }} active
									vulnerabilities currently remain on this
									site.
								</p>
								<div class="site-vulnerability-link">
									<RouterLink
										:to="`/site/${task.site_id}`"
										class="btn btn-outline btn-sm font-xs"
										@click="emit('close')"
									>
										View active vulnerabilities on site
									</RouterLink>
								</div>
							</div>
						</div>
					</div>

					<!-- Status actions -->
					<div
						v-if="!isTerminal(task.status)"
						class="section-divider"
					>
						<h3 class="sub-text font-medium mb-4">Quick Actions</h3>
						<div class="flex-row gap-3">
							<button
								v-if="canComplete(task.status)"
								class="btn btn-primary"
								:disabled="isActioning"
								@click="complete"
							>
								Complete Task
							</button>
							<button
								v-if="task.status === 'pending'"
								class="btn btn-outline"
								:disabled="isActioning"
								@click="changeStatus('in_progress')"
							>
								Start Task
							</button>
							<button
								v-if="
									task.status === 'pending' ||
									task.status === 'in_progress' ||
									task.status === 'overdue'
								"
								class="btn btn-outline"
								:disabled="isActioning"
								@click="changeStatus('skipped')"
							>
								Skip Task
							</button>
						</div>
					</div>

					<p v-if="actionError" class="error-banner">
						{{ actionError }}
					</p>
				</div>

				<footer class="form-actions flex-between mt-4">
					<button
						v-if="authStore.canEdit"
						class="btn btn-text danger"
						:disabled="isDeleting"
						@click="handleDelete"
					>
						{{ isDeleting ? "Deleting…" : "Delete Task" }}
					</button>
					<div class="flex-row gap-3">
						<button class="btn btn-outline" @click="emit('close')">
							Close
						</button>
						<button
							v-if="authStore.canEdit"
							class="btn btn-primary"
							@click="emit('edit', task)"
						>
							Edit Task
						</button>
					</div>
				</footer>
			</div>
		</div>
	</Teleport>
</template>

<style scoped>
.status-banner {
	display: flex;
	gap: 12px;
	padding: 12px 16px;
	border-radius: 6px;
	margin-top: 8px;
	font-size: 14px;
	line-height: 1.4;
	border-left: 4px solid;
}

.status-banner.success {
	background-color: var(--badge-active-bg);
	border-color: var(--badge-active-text);
	color: var(--text-main);

	& .banner-icon {
		color: var(--badge-active-text);
	}
}

.status-banner.warning {
	background-color: var(--warning-bg);
	border-color: var(--warning-border);
	color: var(--text-main);

	& .banner-icon {
		color: var(--warning-text);
	}
}

.banner-icon {
	display: flex;
	align-items: flex-start;
	padding-top: 2px;
}

.banner-text strong {
	display: block;
	font-weight: 600;
	margin-bottom: 2px;
}

.banner-text p {
	margin: 0;
	color: var(--text-muted);
	font-size: 13px;
}

.site-vulnerability-link {
	margin-top: 10px;
}
</style>
