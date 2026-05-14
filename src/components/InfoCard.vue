<script setup lang="ts">
import { ref } from "vue";

export interface InfoItem {
	label: string;
	value: string | number | undefined | null;
	isLink?: boolean;
	href?: string;
	copyable?: boolean;
}

defineProps<{
	title: string;
	items: InfoItem[];
}>();

const copiedIndex = ref<number | null>(null);

const copyToClipboard = async (
	value: string | number | undefined | null,
	index: number,
) => {
	if (value === undefined || value === null) return;

	try {
		await navigator.clipboard.writeText(value.toString());
		copiedIndex.value = index;
		setTimeout(() => {
			copiedIndex.value = null;
		}, 2000);
	} catch (err) {
		console.error("Failed to copy: ", err);
	}
};
</script>

<template>
	<section class="card">
		<h2>{{ title }}</h2>
		<div class="info-grid mt-4">
			<div v-for="(item, index) in items" :key="index" class="info-item">
				<span class="label">{{ item.label }}</span>
				<div class="value-container">
					<span
						class="value"
						:class="{ copyable: item.copyable }"
						:title="item.copyable ? 'Click to copy' : ''"
						@click="
							item.copyable
								? copyToClipboard(item.value, index)
								: null
						"
					>
						{{
							item.value !== undefined && item.value !== null
								? item.value
								: "—"
						}}
						<span v-if="copiedIndex === index" class="copy-feedback"
							>Copied!</span
						>
					</span>

					<a
						v-if="item.isLink && item.href"
						:href="item.href"
						target="_blank"
						rel="noopener noreferrer"
						class="external-link"
						title="Open link"
					>
						<svg
							xmlns="http://www.w3.org/2000/svg"
							width="14"
							height="14"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
						>
							<path
								d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"
							></path>
							<polyline points="15 3 21 3 21 9"></polyline>
							<line x1="10" y1="14" x2="21" y2="3"></line>
						</svg>
					</a>
				</div>
			</div>
		</div>
	</section>
</template>
