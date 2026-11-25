<script setup lang="ts">
import { computed } from "vue";
import { useNotificationStore } from "../stores/notifications";
import { useAuthStore } from "../stores/auth";
import { useNow } from "../composables/useNow";
import { formatElapsed } from "../utils/format";
import AppIcon from "./AppIcon.vue";

const store = useNotificationStore();
const authStore = useAuthStore();

// The open task center lists the same entries, so toasts step aside.
const shown = computed(() => (store.panelOpen ? [] : store.popups));

const isQueued = (t: { type: string; task?: { status: string } }) =>
	t.type === "running" && t.task?.status === "queued";

// Starting many updates queues them all at once: past a few, their toasts
// fold into one so they don't cover the side of the screen.
const MAX_QUEUED_TOASTS = 2;
const queued = computed(() => shown.value.filter(isQueued));
const foldQueued = computed(() => queued.value.length > MAX_QUEUED_TOASTS);
const visible = computed(() =>
	foldQueued.value ? shown.value.filter((t) => !isQueued(t)) : shown.value,
);

const pauseQueued = () => queued.value.forEach((t) => store.pauseTimer(t.id));
const resumeQueued = () => queued.value.forEach((t) => store.resumeTimer(t.id));
const dismissQueued = () => queued.value.forEach((t) => store.dismiss(t.id));
const now = useNow(
	computed(() => visible.value.some((t) => t.type === "running")),
);
</script>

<template>
	<div
		:class="[
			'toast-container',
			{ 'toast-container--above-launcher': authStore.isAuthenticated },
		]"
		aria-live="polite"
	>
		<TransitionGroup name="toast">
			<div
				v-for="toast in visible"
				:key="toast.id"
				:class="['toast', `toast--${toast.type}`]"
				@mouseenter="store.pauseTimer(toast.id)"
				@mouseleave="store.resumeTimer(toast.id)"
			>
				<span
					v-if="toast.type === 'running'"
					class="spinner spinner-small toast-spinner"
				></span>
				<div class="toast-body">
					<div v-if="toast.title" class="toast-title">
						{{ toast.title }}
					</div>
					<span class="toast-message">{{ toast.message }}</span>
					<span v-if="toast.type === 'running'" class="toast-meta">
						{{
							toast.task?.status === "queued"
								? "Queued"
								: `Running ${formatElapsed(now - toast.createdAt)}`
						}}
					</span>
				</div>
				<button
					v-if="toast.type === 'running'"
					class="toast-dismiss"
					title="Hide (the task keeps running)"
					aria-label="Hide task"
					@click="store.setTaskHidden(toast.id, true)"
				>
					<AppIcon name="eye-off" size="18" />
				</button>
				<button
					v-else
					class="toast-dismiss"
					aria-label="Dismiss"
					@click="store.dismiss(toast.id)"
				>
					<AppIcon name="x" size="18" />
				</button>
			</div>
			<div
				v-if="foldQueued"
				key="queued-summary"
				class="toast toast--running"
				@mouseenter="pauseQueued"
				@mouseleave="resumeQueued"
			>
				<span class="spinner spinner-small toast-spinner"></span>
				<div class="toast-body">
					<div class="toast-title">
						{{ queued.length }} jobs queued
					</div>
					<span class="toast-message">
						Each shows here when it starts; the task center lists
						them all.
					</span>
				</div>
				<button
					class="toast-dismiss"
					aria-label="Dismiss"
					@click="dismissQueued"
				>
					<AppIcon name="x" size="18" />
				</button>
			</div>
		</TransitionGroup>
	</div>
</template>
