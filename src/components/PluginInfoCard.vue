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
		<div class="info-grid mt-4">
			<div class="info-item">
				<span class="label">Plugin Name</span>
				<span class="value">{{ info?.name }}</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Slug</span>
				<span class="value">{{ info.slug }}</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Author</span>
				<span class="value">
					<a
						v-if="info.author_profile"
						:href="info.author_profile"
						target="_blank"
						rel="noopener noreferrer"
					>
						{{ info.author }}
					</a>
					<span v-else>{{ info.author }}</span>
				</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Version</span>
				<span class="value">{{ info.version }}</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Requires</span>
				<span class="value">WP {{ info.requires }}</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Tested up to</span>
				<span class="value">WP {{ info.tested }}</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Last Updated</span>
				<span class="value">{{ info.last_updated }}</span>
			</div>
			<div v-if="info" class="info-item">
				<span class="label">Homepage</span>
				<span class="value">
					<a
						v-if="info.homepage"
						:href="info.homepage"
						target="_blank"
						rel="noopener noreferrer"
					>
						View Plugin Page
					</a>
					<span v-else>—</span>
				</span>
			</div>
			<div class="info-item">
				<span class="label">Total Installations</span>
				<span class="value">{{ installationCount }}</span>
			</div>
		</div>
	</section>
</template>

<style scoped>
/* Scoped styles removed in favor of global .info-grid and .info-item classes in components.css */
</style>
