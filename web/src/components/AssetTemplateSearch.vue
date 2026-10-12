<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick } from "vue";
import type { Asset } from "../types";
import { useAssetStore } from "../stores/assetStore";

const props = defineProps<{
	// Name of the currently selected template; restored when a search is cancelled.
	selectedName: string;
	disabled?: boolean;
	inputId?: string;
}>();

const emit = defineEmits<{
	(e: "select", template: Asset): void;
}>();

const assetStore = useAssetStore();
const query = ref(props.selectedName);
const results = ref<Asset[]>([]);
const isOpen = ref(false);
const isLoading = ref(false);
const activeIndex = ref(-1);
const containerRef = ref<HTMLElement | null>(null);
const listRef = ref<HTMLElement | null>(null);
const listId = `${props.inputId || "asset-template"}-listbox`;

let debounceTimer: ReturnType<typeof setTimeout> | undefined;
let abortController: AbortController | null = null;

watch(
	() => props.selectedName,
	(name) => {
		query.value = name;
	},
);

const runSearch = async (q: string) => {
	abortController?.abort();
	abortController = new AbortController();
	isLoading.value = true;
	try {
		results.value = await assetStore.searchAssetTemplates(
			q,
			abortController.signal,
		);
		activeIndex.value = results.value.length > 0 ? 0 : -1;
	} catch (e) {
		if ((e as Error).name !== "AbortError") {
			console.error("Asset template search failed", e);
			results.value = [];
		}
	} finally {
		isLoading.value = false;
	}
};

const onInput = () => {
	clearTimeout(debounceTimer);
	if (query.value.length < 2) {
		abortController?.abort();
		results.value = [];
		isOpen.value = false;
		return;
	}
	isOpen.value = true;
	debounceTimer = setTimeout(() => runSearch(query.value), 200);
};

const cancel = () => {
	clearTimeout(debounceTimer);
	abortController?.abort();
	isOpen.value = false;
	results.value = [];
	query.value = props.selectedName;
};

const select = (template: Asset) => {
	isOpen.value = false;
	results.value = [];
	query.value = template.name;
	emit("select", template);
};

const moveActive = (delta: number) => {
	if (!isOpen.value || results.value.length === 0) return;
	const n = results.value.length;
	activeIndex.value = (activeIndex.value + delta + n) % n;
	nextTick(() => {
		listRef.value
			?.querySelector<HTMLElement>(`[data-index="${activeIndex.value}"]`)
			?.scrollIntoView({ block: "nearest" });
	});
};

const onKeydown = (e: KeyboardEvent) => {
	switch (e.key) {
		case "ArrowDown":
			e.preventDefault();
			moveActive(1);
			break;
		case "ArrowUp":
			e.preventDefault();
			moveActive(-1);
			break;
		case "Enter": {
			const t = results.value[activeIndex.value];
			if (isOpen.value && t) {
				e.preventDefault();
				select(t);
			}
			break;
		}
		case "Escape":
			if (isOpen.value) {
				e.preventDefault();
				e.stopPropagation();
				cancel();
			}
			break;
		case "Tab":
			if (isOpen.value) cancel();
			break;
	}
};

const handleClickOutside = (event: MouseEvent) => {
	if (
		isOpen.value &&
		containerRef.value &&
		!containerRef.value.contains(event.target as Node)
	) {
		cancel();
	}
};

onMounted(() => {
	document.addEventListener("mousedown", handleClickOutside);
});

onUnmounted(() => {
	document.removeEventListener("mousedown", handleClickOutside);
	clearTimeout(debounceTimer);
	abortController?.abort();
});
</script>

<template>
	<div ref="containerRef" class="asset-template-search">
		<!-- autocomplete/name/data-* attributes keep browser and password-manager autofill away -->
		<input
			:id="inputId"
			v-model="query"
			type="search"
			name="asset-template-query"
			autocomplete="off"
			autocorrect="off"
			autocapitalize="off"
			spellcheck="false"
			data-1p-ignore
			data-lpignore="true"
			data-bwignore
			data-form-type="other"
			role="combobox"
			aria-autocomplete="list"
			:aria-expanded="isOpen"
			:aria-controls="listId"
			:aria-activedescendant="
				isOpen && activeIndex >= 0
					? `${listId}-${activeIndex}`
					: undefined
			"
			placeholder="Start typing to search templates..."
			:disabled="disabled"
			@input="onInput"
			@keydown="onKeydown"
		/>
		<ul
			v-if="isOpen"
			:id="listId"
			ref="listRef"
			class="ats-dropdown"
			role="listbox"
		>
			<li v-if="isLoading && results.length === 0" class="ats-status">
				Searching...
			</li>
			<li v-else-if="results.length === 0" class="ats-status">
				No matching templates
			</li>
			<li
				v-for="(t, i) in results"
				:id="`${listId}-${i}`"
				:key="t.id"
				:data-index="i"
				role="option"
				:aria-selected="i === activeIndex"
				class="ats-option"
				:class="{ 'is-active': i === activeIndex }"
				@mousedown.prevent
				@mouseenter="activeIndex = i"
				@click="select(t)"
			>
				<div class="res-name">{{ t.name }}</div>
				<div class="sub-text">
					{{ t.type }} • {{ (t.default_price || 0) / 100 }}€ /
					{{ t.default_freq }}
				</div>
			</li>
		</ul>
	</div>
</template>

<style scoped>
.asset-template-search {
	position: relative;
}

.asset-template-search input {
	width: 100%;
}

.ats-dropdown {
	position: absolute;
	top: calc(100% + 4px);
	left: 0;
	right: 0;
	z-index: 100;
	margin: 0;
	padding: 4px 0;
	list-style: none;
	background: var(--bg-card);
	border: 1px solid var(--border-input);
	border-radius: 4px;
	box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
	max-height: 240px;
	overflow-y: auto;
}

.ats-option {
	padding: 0.5rem 0.75rem;
	cursor: pointer;
}

.ats-option.is-active {
	background-color: var(--bg-hover);
}

.ats-status {
	padding: 0.75rem;
	text-align: center;
	color: var(--text-muted);
	font-size: 14px;
}
</style>
