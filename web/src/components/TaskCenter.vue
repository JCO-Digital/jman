<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { RouterLink } from "vue-router";
import {
	useNotificationStore,
	type Notification,
} from "../stores/notifications";
import { useAuthStore } from "../stores/auth";
import { useDataStore } from "../stores/data";
import { useUpdateJobsStore } from "../stores/updateJobs";
import { useNow } from "../composables/useNow";
import { formatElapsed, formatRelativeTime } from "../utils/format";
import AppIcon from "./AppIcon.vue";
import UpdateOutputModal from "./UpdateOutputModal.vue";

const store = useNotificationStore();
const authStore = useAuthStore();
const dataStore = useDataStore();
const updateJobsStore = useUpdateJobsStore();

const root = ref<HTMLElement | null>(null);
const open = computed(() => store.panelOpen);
const runningCount = computed(() => store.running.length);
// Elapsed and relative times only need to tick while they're visible.
const now = useNow(open);

function toggle() {
	store.setPanelOpen(!open.value);
}

function author(e: Notification): string {
	const by = e.task?.createdBy;
	if (!by) return "";
	return by === authStore.user?.username ? "you" : by;
}

// The job whose wp-cli output is open, if any.
const outputJobId = ref<number | null>(null);

function showOutput(jobId: number) {
	store.setPanelOpen(false);
	outputJobId.value = jobId;
}

function siteDomain(e: Notification): string | undefined {
	return e.task ? dataStore.getSiteById(e.task.siteId)?.domain : undefined;
}

/** "Queued", with the job's place in the queue when known. */
function queuedLabel(e: Notification): string {
	const pos = e.task?.position;
	return pos ? `Queued · #${pos} in line` : "Queued";
}

/** A label for a priority other than normal. */
function priorityLabel(e: Notification): string {
	switch (e.task?.priority) {
		case "urgent":
			return "Urgent";
		case "high":
			return "High priority";
		case "low":
			return "Low priority";
		default:
			return "";
	}
}

/** Queued normal and low jobs can be moved ahead by execute users. */
function canPrioritize(e: Notification): boolean {
	const p = e.task?.priority;
	return (
		authStore.canExecute &&
		e.task?.status === "queued" &&
		(p === "normal" || p === "low")
	);
}

const prioritizing = ref<Set<number>>(new Set());

async function prioritize(e: Notification) {
	if (!e.task) return;
	const id = e.task.jobId;
	prioritizing.value.add(id);
	try {
		await updateJobsStore.setPriority(id, "high");
	} catch (err) {
		store.notify(
			`Failed to prioritize the job: ${err instanceof Error ? err.message : String(err)}`,
		);
	} finally {
		prioritizing.value.delete(id);
	}
}

const icons: Record<string, string> = {
	success: "check",
	error: "alert",
	info: "bell",
};

function onPointerDown(ev: PointerEvent) {
	if (open.value && root.value && !root.value.contains(ev.target as Node)) {
		store.setPanelOpen(false);
	}
}

function onKeydown(ev: KeyboardEvent) {
	if (ev.key === "Escape" && open.value) store.setPanelOpen(false);
}

onMounted(() => {
	document.addEventListener("pointerdown", onPointerDown);
	document.addEventListener("keydown", onKeydown);
});
onUnmounted(() => {
	document.removeEventListener("pointerdown", onPointerDown);
	document.removeEventListener("keydown", onKeydown);
});
</script>

<template>
	<div ref="root" class="task-center">
		<Transition name="task-panel">
			<section
				v-if="open"
				class="task-panel"
				role="dialog"
				aria-label="Tasks and notifications"
			>
				<header class="task-panel-header">
					<h2>Tasks &amp; notifications</h2>
					<button
						v-if="store.recent.length"
						class="btn btn-outline btn-sm"
						@click="store.clearFinished()"
					>
						Clear
					</button>
					<button
						class="icon-btn icon-btn-sm"
						aria-label="Close"
						@click="store.setPanelOpen(false)"
					>
						<AppIcon name="x" size="18" />
					</button>
				</header>

				<div class="task-panel-body">
					<template v-if="store.running.length">
						<h3 class="task-section-title">Running</h3>
						<ul class="task-list">
							<li
								v-for="e in store.running"
								:key="e.id"
								class="task-item task-item--running"
							>
								<span
									class="spinner spinner-small task-item-icon"
								></span>
								<div class="task-item-body">
									<div class="task-item-title">
										{{ e.title }}
									</div>
									<div class="task-item-message">
										{{ e.message }}
									</div>
									<div class="task-item-meta">
										<span>{{
											e.task?.status === "queued"
												? queuedLabel(e)
												: formatElapsed(
														now - e.createdAt,
													)
										}}</span>
										<span
											v-if="priorityLabel(e)"
											:class="[
												'task-priority',
												`task-priority--${e.task?.priority}`,
											]"
											>{{ priorityLabel(e) }}</span
										>
										<span v-if="author(e)"
											>by {{ author(e) }}</span
										>
										<RouterLink
											v-if="e.task && siteDomain(e)"
											:to="{
												name: 'site-detail',
												params: { id: e.task.siteId },
											}"
											@click="store.setPanelOpen(false)"
										>
											Open site
										</RouterLink>
										<button
											v-if="canPrioritize(e)"
											type="button"
											class="task-link-btn"
											title="Move to high priority: runs before normal and low priority jobs"
											:disabled="
												prioritizing.has(e.task!.jobId)
											"
											@click="prioritize(e)"
										>
											Prioritize
										</button>
									</div>
								</div>
								<button
									v-if="e.task"
									class="icon-btn icon-btn-sm"
									:title="
										e.task.hidden
											? 'Show toast while running'
											: 'Hide toast while running'
									"
									:aria-label="
										e.task.hidden
											? 'Show toast'
											: 'Hide toast'
									"
									@click="
										store.setTaskHidden(
											e.id,
											!e.task.hidden,
										)
									"
								>
									<AppIcon
										:name="
											e.task.hidden ? 'eye' : 'eye-off'
										"
										size="16"
									/>
								</button>
							</li>
						</ul>
					</template>

					<h3 class="task-section-title">Recent</h3>
					<ul v-if="store.recent.length" class="task-list">
						<li
							v-for="e in store.recent"
							:key="e.id"
							:class="['task-item', `task-item--${e.type}`]"
						>
							<span class="task-item-icon">
								<AppIcon
									:name="icons[e.type] ?? 'bell'"
									size="16"
								/>
							</span>
							<div class="task-item-body">
								<div v-if="e.title" class="task-item-title">
									{{ e.title }}
								</div>
								<div class="task-item-message">
									{{ e.message }}
								</div>
								<div class="task-item-meta">
									<span>{{
										formatRelativeTime(e.updatedAt, now)
									}}</span>
									<span v-if="author(e)"
										>by {{ author(e) }}</span
									>
									<RouterLink
										v-if="e.task && siteDomain(e)"
										:to="{
											name: 'site-detail',
											params: { id: e.task.siteId },
										}"
										@click="store.setPanelOpen(false)"
									>
										Open site
									</RouterLink>
									<RouterLink
										v-if="e.task?.review"
										:to="{
											name: 'update-review',
											params: { jobId: e.task.jobId },
										}"
										@click="store.setPanelOpen(false)"
									>
										Review changes
									</RouterLink>
									<button
										v-if="e.task?.output"
										type="button"
										class="task-link"
										title="wp-cli's full output of the update"
										@click="showOutput(e.task.jobId)"
									>
										Show output
									</button>
								</div>
							</div>
						</li>
					</ul>
					<p v-else class="task-empty">No notifications yet.</p>
				</div>
			</section>
		</Transition>

		<button
			:class="[
				'task-launcher',
				{ 'task-launcher--active': runningCount },
			]"
			:aria-expanded="open"
			:aria-label="
				runningCount
					? `${runningCount} running task${runningCount === 1 ? '' : 's'}`
					: 'Tasks and notifications'
			"
			@click="toggle"
		>
			<AppIcon name="bell" size="20" />
			<span v-if="runningCount" class="task-launcher-badge">{{
				runningCount
			}}</span>
			<span
				v-else-if="store.unreadCount"
				class="task-launcher-dot"
				aria-hidden="true"
			></span>
		</button>
		<UpdateOutputModal :job-id="outputJobId" @close="outputJobId = null" />
	</div>
</template>
