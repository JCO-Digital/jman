<script setup lang="ts">
import { ref } from "vue";
import AppIcon from "./AppIcon.vue";

export interface InfoItem {
	label: string;
	value: string | number | undefined | null;
	isLink?: boolean;
	href?: string;
	copyable?: boolean;
	secondaryCopyValue?: string;
	secondaryCopyTitle?: string;
}

defineProps<{
	title: string;
	items: InfoItem[];
}>();

const copiedIndex = ref<number | null>(null);
const copiedSecondaryIndex = ref<number | null>(null);

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

const copySecondary = async (value: string, index: number) => {
	try {
		await navigator.clipboard.writeText(value);
		copiedSecondaryIndex.value = index;
		setTimeout(() => {
			copiedSecondaryIndex.value = null;
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

					<button
						v-if="item.secondaryCopyValue"
						type="button"
						class="icon-btn icon-btn-sm"
						:title="
							item.secondaryCopyTitle || 'Copy alternative value'
						"
						@click.stop="
							copySecondary(item.secondaryCopyValue, index)
						"
						style="
							display: inline-flex;
							align-items: center;
							border: none;
							background: transparent;
							cursor: pointer;
							color: var(--text-muted);
							padding: 2px 4px;
							margin-left: 4px;
						"
					>
						<AppIcon
							:name="
								copiedSecondaryIndex === index
									? 'check'
									: 'copy'
							"
							size="14"
						/>
					</button>

					<a
						v-if="item.isLink && item.href"
						:href="item.href"
						target="_blank"
						rel="noopener noreferrer"
						class="external-link"
						title="Open link"
					>
						<AppIcon name="external-link" size="14" />
					</a>
				</div>
			</div>
		</div>
	</section>
</template>
