<script setup lang="ts">
import LoadingSpinner from "./LoadingSpinner.vue";

defineProps<{
	title: string;
	value: string | number;
	label: string;
	loading?: boolean;
	valueStyle?: Record<string, string | number>;
}>();
</script>

<template>
	<div class="card stat-card">
		<h3>{{ title }}</h3>
		<div class="stat-value" :style="valueStyle">
			<LoadingSpinner v-if="loading" small />
			<template v-else>{{ value }}</template>
		</div>
		<p class="stat-label">{{ label }}</p>
	</div>
</template>

<style scoped>
.stat-value {
	font-size: 3rem;
	font-weight: 700;
	color: var(--primary-color);
	margin: 12px 0;
	display: flex;
	align-items: center;
	min-height: 1.2em;
}

.stat-label {
	color: var(--text-muted);
	font-size: 0.9rem;
	margin: 0;
}

.stat-card h3 {
	margin: 0;
	font-size: 1.1rem;
}
</style>
