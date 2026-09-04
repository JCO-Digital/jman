<script setup lang="ts">
defineProps<{
	currentPage: number;
	totalPages: number;
	rowsPerPage: number;
}>();

const emit = defineEmits<{
	(e: "update:rowsPerPage", value: number): void;
	(e: "prev"): void;
	(e: "next"): void;
}>();

const handleRowsPerPageChange = (event: Event) => {
	const target = event.target as HTMLSelectElement;
	emit("update:rowsPerPage", parseInt(target.value, 10));
};
</script>

<template>
	<div class="pagination">
		<div class="rows-per-page">
			<label for="per-page">Rows per page:</label>
			<select
				id="per-page"
				:value="rowsPerPage"
				@change="handleRowsPerPageChange"
			>
				<option value="50">50</option>
				<option value="100">100</option>
				<option value="150">150</option>
				<option value="200">200</option>
				<option value="250">250</option>
			</select>
		</div>
		<div class="page-controls">
			<button :disabled="currentPage === 1" @click="emit('prev')">
				&laquo; Prev
			</button>
			<span>Page {{ currentPage }} of {{ totalPages }}</span>
			<button
				:disabled="currentPage === totalPages"
				@click="emit('next')"
			>
				Next &raquo;
			</button>
		</div>
	</div>
</template>

<style scoped>
/* Scoped styles if needed, though most are in style.css */
</style>
