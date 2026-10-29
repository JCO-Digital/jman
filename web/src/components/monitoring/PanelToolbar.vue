<script setup lang="ts">
/**
 * The toolbar at the top of a Monitoring section: search and filters in the
 * default slot on the left, the section's actions in #actions on the right.
 */
</script>

<template>
	<div class="panel-toolbar">
		<slot />
		<span class="spacer"></span>
		<slot name="actions" />
	</div>
</template>

<style scoped>
.panel-toolbar {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 8px;
	margin-bottom: 16px;

	:deep(.input-field) {
		max-width: 240px;
	}
}

.spacer {
	flex: 1;
}
</style>
