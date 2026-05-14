<script setup lang="ts">
import LoadingSpinner from "./LoadingSpinner.vue";

defineProps<{
	title: string;
	value: string | number;
	label: string;
	loading?: boolean;
}>();
</script>

<template>
	<div class="card stat-card">
		<h3>{{ title }}</h3>
		<div class="stat-value">
			<LoadingSpinner v-if="loading" small />
			<template v-else>{{ value }}</template>
		</div>
		<p class="stat-label">{{ label }}</p>
	</div>
</template>

<style scoped>
.stat-card h3 {
	margin: 0;
	font-size: 1.1rem;
	color: var(--text-heading);
}

.stat-value {
	font-size: 3rem;
	font-weight: 700;
	color: var(--primary);
	margin: 12px 0;
	display: flex;
	align-items: center;
	min-height: 1.2em;
}

/* Allow utility classes applied to the component to override the value color */
.stat-card.error-text .stat-value,
.stat-card.text-error .stat-value {
	color: var(--error-text);
}

.stat-card.warning-text .stat-value,
.stat-card.text-warning .stat-value {
	color: var(--warning-text);
}

.stat-label {
	color: var(--text-muted);
	font-size: 0.875rem;
	margin: 0;
}
</style>
