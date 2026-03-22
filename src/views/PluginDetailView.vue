<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const dataStore = useDataStore();

const pluginName = computed(() => props.name);

const info = computed(() => {
	return dataStore.pluginInfo.find((i) => i.name === props.name);
});

const sitesWithPlugin = computed(() => {
	const name = pluginName.value;
	return dataStore.plugins
		.filter((p) => p.name === name)
		.map((p) => {
			const site = dataStore.sites.find((s) => s.id === p.site_id);
			return {
				...p,
				site_domain: site ? site.domain : "Unknown Site",
				site_id: p.site_id,
			};
		})
		.sort((a, b) => a.site_domain.localeCompare(b.site_domain));
});

const goBack = () => {
	router.push({ name: "plugins" });
};

const goToSite = (siteId: number) => {
	router.push({ name: "site-detail", params: { id: siteId.toString() } });
};
</script>

<template>
	<div class="view-container">
		<header class="header">
			<div class="title-area">
				<button class="back-btn" @click="goBack">&larr; Back to Plugins</button>
				<h1>Plugin Details</h1>
			</div>
		</header>

		<main class="content" v-if="sitesWithPlugin.length > 0 || info">
			<section class="card">
				<h2>Plugin Information</h2>
				<div class="info-grid">
					<div class="info-item">
						<span class="label">Plugin Name:</span>
						<span class="value">{{ pluginName }}</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Slug:</span>
						<span class="value">{{ info.slug }}</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Author:</span>
						<span class="value">
							<a
								v-if="info.author_profile"
								:href="info.author_profile"
								target="_blank"
								rel="noopener noreferrer"
								class="link"
							>
								{{ info.author }}
							</a>
							<span v-else>{{ info.author }}</span>
						</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Version:</span>
						<span class="value">{{ info.version }}</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Requires:</span>
						<span class="value">WP {{ info.requires }}</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Tested up to:</span>
						<span class="value">WP {{ info.tested }}</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Last Updated:</span>
						<span class="value">{{ info.last_updated }}</span>
					</div>
					<div class="info-item" v-if="info">
						<span class="label">Homepage:</span>
						<span class="value">
							<a
								v-if="info.homepage"
								:href="info.homepage"
								target="_blank"
								rel="noopener noreferrer"
								class="link"
							>
								View Plugin Page
							</a>
							<span v-else>-</span>
						</span>
					</div>
					<div class="info-item">
						<span class="label">Total Installations:</span>
						<span class="value">{{ sitesWithPlugin.length }}</span>
					</div>
				</div>
			</section>

			<section class="card">
				<h2>Installed on Sites</h2>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Site Domain</th>
								<th>Version</th>
								<th>Status</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-for="item in sitesWithPlugin"
								:key="item.site_id"
								class="clickable-row"
								@click="goToSite(item.site_id)"
							>
								<td>{{ item.site_domain }}</td>
								<td>{{ item.version }}</td>
								<td>
									<span :class="['status-badge', item.status.toLowerCase()]">
										{{ item.status }}
									</span>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</section>
		</main>

		<main class="content" v-else>
			<div class="card">
				<div v-if="dataStore.isLoading" class="empty-state">
					<div class="spinner" style="margin-bottom: 12px"></div>
					<div>Loading plugin details...</div>
				</div>
				<div v-else class="empty-state">
					<p>Plugin details not found.</p>
					<button class="back-btn" @click="goBack" style="margin-top: 16px">
						Go back to plugins
					</button>
				</div>
			</div>
		</main>
	</div>
</template>

<style scoped>
/* All generic styles moved to style.css */
</style>
