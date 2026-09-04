<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from "vue";
import AppIcon from "./AppIcon.vue";

interface Option {
	value: string | number;
	label: string;
}

const props = defineProps<{
	options: Option[];
	modelValue: string | number | null;
	placeholder?: string;
	required?: boolean;
	disabled?: boolean;
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: string | number | null): void;
}>();

const isOpen = ref(false);
const searchQuery = ref("");
const containerRef = ref<HTMLElement | null>(null);
const searchInput = ref<HTMLInputElement | null>(null);

const filteredOptions = computed(() => {
	if (!searchQuery.value) return props.options;
	const q = searchQuery.value.toLowerCase();
	return props.options.filter((o) => o.label.toLowerCase().includes(q));
});

const selectedLabel = computed(() => {
	const option = props.options.find((o) => o.value === props.modelValue);
	return option ? option.label : "";
});

watch(isOpen, (newVal) => {
	if (newVal) {
		nextTick(() => {
			searchInput.value?.focus();
		});
	}
});

const selectOption = (option: Option) => {
	emit("update:modelValue", option.value);
	isOpen.value = false;
	searchQuery.value = "";
};

const toggleDropdown = () => {
	if (props.disabled) return;
	isOpen.value = !isOpen.value;
	if (isOpen.value) {
		searchQuery.value = "";
	}
};

const handleClickOutside = (event: MouseEvent) => {
	if (
		containerRef.value &&
		!containerRef.value.contains(event.target as Node)
	) {
		isOpen.value = false;
	}
};

onMounted(() => {
	document.addEventListener("mousedown", handleClickOutside);
});

onUnmounted(() => {
	document.removeEventListener("mousedown", handleClickOutside);
});
</script>

<template>
	<div
		ref="containerRef"
		class="searchable-select"
		:class="{ 'is-open': isOpen, 'is-disabled': disabled }"
	>
		<div class="select-display" @click="toggleDropdown">
			<div v-if="selectedLabel" class="selected-text">
				{{ selectedLabel }}
			</div>
			<div v-else class="placeholder">
				{{ placeholder || "Select option..." }}
			</div>
			<AppIcon name="chevron-right" size="14" class="arrow-icon" />
		</div>

		<div v-if="isOpen" class="dropdown-menu">
			<div class="search-box">
				<input
					ref="searchInput"
					v-model="searchQuery"
					type="text"
					placeholder="Search..."
					@click.stop
				/>
			</div>
			<ul class="options-list">
				<li v-if="filteredOptions.length === 0" class="no-results">
					No matches found
				</li>
				<li
					v-for="option in filteredOptions"
					:key="option.value"
					:class="{ 'is-selected': option.value === modelValue }"
					@click="selectOption(option)"
				>
					{{ option.label }}
				</li>
			</ul>
		</div>
	</div>
</template>

<style scoped>
.searchable-select {
	position: relative;
	width: 100%;
}

.select-display {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 8px 12px;
	background-color: var(--bg-card);
	border: 1px solid var(--border-input);
	border-radius: 4px;
	cursor: pointer;
	min-height: 38px;
	font-size: 14px;
}

.is-disabled .select-display {
	background-color: var(--bg-disabled);
	cursor: not-allowed;
	color: var(--text-disabled);
}

.select-display:hover:not(.is-disabled) {
	border-color: var(--primary);
}

.arrow-icon {
	transition: transform 0.2s;
	transform: rotate(90deg);
}

.is-open .arrow-icon {
	transform: rotate(-90deg);
}

.placeholder {
	color: var(--text-placeholder);
}

.dropdown-menu {
	position: absolute;
	top: 100%;
	left: 0;
	right: 0;
	z-index: 100;
	background-color: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 4px;
	margin-top: 4px;
	box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
	display: flex;
	flex-direction: column;
	max-height: 300px;
}

.search-box {
	padding: 8px;
	border-bottom: 1px solid var(--border-color);
}

.search-box input {
	width: 100%;
	padding: 6px 10px;
}

.options-list {
	list-style: none;
	padding: 0;
	margin: 0;
	overflow-y: auto;
}

.options-list li {
	padding: 8px 12px;
	cursor: pointer;
	font-size: 14px;
	color: var(--text-main);
}

.options-list li:hover {
	background-color: var(--bg-hover);
}

.options-list li.is-selected {
	background-color: var(--primary);
	color: var(--primary-text);
}

.no-results {
	padding: 12px;
	text-align: center;
	color: var(--text-muted);
	font-size: 14px;
}
</style>
