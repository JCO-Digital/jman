<script setup lang="ts">
import { useToastStore } from "../stores/toast";

const toastStore = useToastStore();
</script>

<template>
	<div class="toast-container">
		<TransitionGroup name="toast">
			<div
				v-for="toast in toastStore.toasts"
				:key="toast.id"
				:class="['toast', `toast--${toast.type}`]"
			>
				<span class="toast-message">{{ toast.message }}</span>
				<button class="toast-dismiss" @click="toastStore.removeToast(toast.id)">
					&times;
				</button>
			</div>
		</TransitionGroup>
	</div>
</template>

<style scoped>
.toast-container {
	position: fixed;
	bottom: 20px;
	right: 20px;
	z-index: 9999;
	display: flex;
	flex-direction: column;
	gap: 8px;
	max-width: 380px;
}

.toast {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 12px 16px;
	border-radius: 6px;
	border: 1px solid;
	box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	font-size: 0.9rem;
}

.toast--error {
	background: var(--error-bg);
	border-color: var(--error-border);
	color: var(--error-text);
}

.toast--success {
	background: var(--badge-active-bg);
	border-color: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.toast--info {
	background: var(--bg-card);
	border-color: var(--border-color);
	color: var(--text-main);
}

.toast-message {
	flex: 1;
	margin-right: 12px;
}

.toast-dismiss {
	background: none;
	border: none;
	color: inherit;
	font-size: 1.2rem;
	cursor: pointer;
	padding: 0 4px;
	line-height: 1;
	opacity: 0.7;
}

.toast-dismiss:hover {
	opacity: 1;
}

.toast-enter-active,
.toast-leave-active {
	transition:
		opacity 0.3s ease,
		transform 0.3s ease;
}

.toast-enter-from,
.toast-leave-to {
	opacity: 0;
	transform: translateX(20px);
}
</style>
