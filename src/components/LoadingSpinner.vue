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
/* Scoped styles removed in favor of global .loading-container and .spinner classes in components.css */
</style>
