<script setup lang="ts">
defineProps<{
	message?: string;
	small?: boolean;
}>();
</script>

<template>
	<div :class="['loading-container', { 'inline-spinner': small }]">
		<div :class="['spinner', { 'spinner-small': small }]"></div>
		<div v-if="message" class="loading-message">{{ message }}</div>
	</div>
</template>

<style scoped>
.loading-container {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	padding: 20px;
}

.inline-spinner {
	display: inline-flex;
	flex-direction: row;
	padding: 0;
	gap: 8px;
}

.loading-message {
	margin-top: 12px;
	color: var(--text-muted);
}

.inline-spinner .loading-message {
	margin-top: 0;
}
</style>
