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
const visible = computed(() => (store.panelOpen ? [] : store.popups));
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
		</TransitionGroup>
	</div>
</template>
