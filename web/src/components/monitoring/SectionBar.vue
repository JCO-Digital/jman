<script setup lang="ts" generic="T extends string">
/**
 * The row of section buttons every Monitoring area starts with. The first
 * section is what needs attention now; extra controls for the current
 * section (filters, a loading spinner) go in the default slot.
 */
import type { Section } from "./sections";

defineProps<{ sections: Section<T>[] }>();
const model = defineModel<T>({ required: true });
</script>

<template>
	<div class="section-bar">
		<div class="segmented" role="tablist">
			<button
				v-for="s in sections"
				:key="s.key"
				class="btn btn-sm"
				role="tab"
				:aria-selected="model === s.key"
				:class="model === s.key ? 'btn-primary' : 'btn-outline'"
				@click="model = s.key"
			>
				{{ s.label }}
				<span v-if="s.count" class="count">{{ s.count }}</span>
			</button>
		</div>
		<slot />
	</div>
</template>

<style scoped>
.section-bar {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: var(--space-3);
	margin-bottom: var(--space-4);
}

.segmented {
	display: flex;
	flex-wrap: wrap;
	gap: var(--space-2);

	.count {
		margin-left: var(--space-1);
		font-weight: 700;
	}
}
</style>
