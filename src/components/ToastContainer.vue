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
				<button
					class="toast-dismiss"
					@click="toastStore.removeToast(toast.id)"
				>
					&times;
				</button>
			</div>
		</TransitionGroup>
	</div>
</template>
