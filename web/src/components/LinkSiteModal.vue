<script setup lang="ts">
import { ref, computed } from "vue";
import type { Site } from "../types";
import { useOrganizationStore } from "../stores/organization";
import { useDataStore } from "../stores/data";
import { useToastStore } from "../stores/toast";

const props = defineProps<{
	modelValue: boolean;
	organizationId: number;
	linkedSites: Site[];
}>();

const emit = defineEmits<{
	(e: "update:modelValue", value: boolean): void;
	(e: "linked"): void;
}>();

const organizationStore = useOrganizationStore();
const dataStore = useDataStore();
const toast = useToastStore();

const siteSearchQuery = ref("");

const availableSites = computed(() => {
	const query = siteSearchQuery.value.toLowerCase();
	return dataStore.enrichedSites.filter((site) => {
		const isNotLinked = !props.linkedSites.some((s) => s.id === site.id);
		const matchesQuery = site.domain.toLowerCase().includes(query);
		return isNotLinked && matchesQuery;
	});
});

const handleLinkSite = async (siteId: number) => {
	try {
		await organizationStore.linkSiteToOrganization(
			siteId,
			props.organizationId,
		);
		dataStore.setSiteOrganizationLink(siteId, props.organizationId);
		await dataStore.refreshData();
		emit("linked");
		close();
	} catch (e: any) {
		toast.addToast("Failed to link site: " + e.message, "error");
	}
};

const close = () => {
	siteSearchQuery.value = "";
	emit("update:modelValue", false);
};
</script>

<template>
	<div v-if="modelValue" class="modal-overlay" @click.self="close">
		<div class="modal-content card">
			<h2>Link Site to Organization</h2>
			<div class="form-layout">
				<div class="form-group">
					<label for="s-search">Search Site (by domain)</label>
					<input
						id="s-search"
						v-model="siteSearchQuery"
						type="text"
						placeholder="Start typing domain..."
						autofocus
					/>
				</div>

				<div v-if="availableSites.length > 0" class="search-results">
					<div
						v-for="site in availableSites"
						:key="site.id"
						class="search-result-item clickable-row"
						@click="handleLinkSite(site.id)"
					>
						<div class="res-name">{{ site.domain }}</div>
					</div>
				</div>
				<div v-else-if="siteSearchQuery.length > 1" class="empty-state">
					No unlinked sites found matching "{{ siteSearchQuery }}"
				</div>

				<div class="form-actions">
					<button class="btn btn-outline" @click="close">
						Cancel
					</button>
				</div>
			</div>
		</div>
	</div>
</template>

<style scoped>
.search-results {
	max-height: 300px;
	overflow-y: auto;
	border: 1px solid var(--border-color);
	border-radius: var(--radius-md);
	margin-bottom: 1rem;
}

.search-result-item {
	padding: 0.75rem 1rem;
	border-bottom: 1px solid var(--border-color);
	cursor: pointer;
	transition: background-color 0.2s;

	&:last-child {
		border-bottom: none;
	}

	&:hover {
		background-color: var(--bg-hover);
	}
}

.res-name {
	font-weight: 500;
}
</style>
