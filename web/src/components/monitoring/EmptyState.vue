<script setup lang="ts">
/** "All clear" for a Monitoring section, with what is being watched. */
defineProps<{
	icon?: string;
	title: string;
}>();
</script>

<template>
	<div class="empty-state">
		<div v-if="icon" class="empty-icon">{{ icon }}</div>
		<h3>{{ title }}</h3>
		<p v-if="$slots.default" class="sub-text"><slot /></p>
	</div>
</template>

<style scoped>
.empty-state {
	text-align: center;
	padding: var(--space-8) var(--space-6);

	h3 {
		margin: 0 0 var(--space-2);
		color: var(--text-heading);
	}

	p {
		margin: 0;
	}
}

.empty-icon {
	font-size: 36px;
	margin-bottom: var(--space-3);
}
</style>
