<script setup lang="ts">
export interface InfoItem {
	label: string;
	value: string | number | undefined | null;
	isLink?: boolean;
	href?: string;
}

defineProps<{
	title: string;
	items: InfoItem[];
}>();
</script>

<template>
	<section class="card">
		<h2>{{ title }}</h2>
		<div class="info-grid">
			<div v-for="(item, index) in items" :key="index" class="info-item">
				<span class="label">{{ item.label }}:</span>
				<span class="value">
					<template v-if="item.isLink && item.href">
						<a
							:href="item.href"
							target="_blank"
							rel="noopener noreferrer"
							class="link"
						>
							{{ item.value || "-" }}
						</a>
					</template>
					<template v-else>
						{{ item.value !== undefined && item.value !== null ? item.value : "-" }}
					</template>
				</span>
			</div>
		</div>
	</section>
</template>

<style scoped>
.info-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
	gap: 16px;
	margin-top: 16px;
}

.info-item {
	display: flex;
	flex-direction: column;
	gap: 4px;
}

.label {
	font-size: 0.85em;
	color: var(--text-muted);
	font-weight: 500;
}

.value {
	font-weight: 500;
	word-break: break-word;
}

.link {
	color: var(--primary);
	text-decoration: none;
}

.link:hover {
	text-decoration: underline;
}
</style>
