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
			<button v-if="backButton" class="back-btn" @click="backButton.onClick">
				&larr; {{ backButton.text }}
			</button>
			<h1>{{ title }}</h1>
		</div>
		<div class="header-actions">
			<slot name="actions"></slot>
		</div>
	</header>
</template>

<style scoped>
.header {
	display: flex;
	justify-content: space-between;
	align-items: center;
	margin-bottom: 24px;
	gap: 16px;
	flex-wrap: wrap;
}

.title-area {
	display: flex;
	flex-direction: column;
	gap: 8px;
	align-items: flex-start;
}

.back-btn {
	background: none;
	border: none;
	color: var(--primary);
	cursor: pointer;
	padding: 0;
	font-size: 0.9em;
	font-weight: 500;
}

.back-btn:hover {
	text-decoration: underline;
}

h1 {
	margin: 0;
	font-size: 1.8em;
	font-weight: 700;
}

@media (max-width: 600px) {
	.header {
		flex-direction: column;
		align-items: flex-start;
	}
}
</style>
