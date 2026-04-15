<script setup lang="ts">
import { useDataStore } from "../stores/data";
import LoadingSpinner from "./LoadingSpinner.vue";

defineProps<{
	title: string;
	showRefresh?: boolean;
	backButton?: {
		text: string;
		onClick: () => void;
	};
}>();

const dataStore = useDataStore();
</script>

<template>
	<header class="header">
		<div class="title-area">
			<button v-if="backButton" class="back-btn" @click="backButton.onClick">
				&larr; {{ backButton.text }}
			</button>
			<h1>{{ title }}</h1>
		</div>
		<div v-if="showRefresh" class="actions">
			<button
				class="btn btn-primary"
				@click="dataStore.refreshData()"
				:disabled="dataStore.isLoading"
			>
				<LoadingSpinner
					v-if="dataStore.isLoading"
					small
					message="Refreshing..."
				/>
				<span v-else>Refresh Data</span>
			</button>
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

.actions {
	display: flex;
	align-items: center;
}

@media (max-width: 600px) {
	.header {
		flex-direction: column;
		align-items: flex-start;
	}

	.actions {
		width: 100%;
	}

	.btn {
		width: 100%;
		justify-content: center;
	}
}
</style>
