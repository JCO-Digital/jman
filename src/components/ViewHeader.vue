<script setup lang="ts">
defineProps<{
	title: string;
	backButton?: {
		text: string;
		onClick: () => void;
	};
}>();
</script>

<template>
	<header class="header">
		<div class="title-area">
			<button
				v-if="backButton"
				class="back-btn"
				@click="backButton.onClick"
			>
				&larr; {{ backButton.text }}
			</button>
			<h1>{{ title }}</h1>
		</div>
		<div class="header-actions">
			<slot name="actions"></slot>
		</div>
	</header>
</template>
