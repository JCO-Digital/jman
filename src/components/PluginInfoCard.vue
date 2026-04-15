<script setup lang="ts">
import type { EnrichedPlugin } from "../types";

defineProps<{
	info: EnrichedPlugin | undefined;
	installationCount: number;
}>();
</script>

<template>
	<section class="card">
		<h2>Plugin Information</h2>
		<div class="info-grid">
			<div class="info-item">
				<span class="label">Plugin Name:</span>
				<span class="value">{{ info?.name }}</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Slug:</span>
				<span class="value">{{ info.slug }}</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Author:</span>
				<span class="value">
					<a
						v-if="info.author_profile"
						:href="info.author_profile"
						target="_blank"
						rel="noopener noreferrer"
						class="link"
					>
						{{ info.author }}
					</a>
					<span v-else>{{ info.author }}</span>
				</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Version:</span>
				<span class="value">{{ info.version }}</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Requires:</span>
				<span class="value">WP {{ info.requires }}</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Tested up to:</span>
				<span class="value">WP {{ info.tested }}</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Last Updated:</span>
				<span class="value">{{ info.last_updated }}</span>
			</div>
			<div class="info-item" v-if="info">
				<span class="label">Homepage:</span>
				<span class="value">
					<a
						v-if="info.homepage"
						:href="info.homepage"
						target="_blank"
						rel="noopener noreferrer"
						class="link"
					>
						View Plugin Page
					</a>
					<span v-else>-</span>
				</span>
			</div>
			<div class="info-item">
				<span class="label">Total Installations:</span>
				<span class="value">{{ installationCount }}</span>
			</div>
		</div>
	</section>
</template>

<style scoped>
.info-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
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
