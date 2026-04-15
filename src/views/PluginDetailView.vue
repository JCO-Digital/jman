<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const dataStore = useDataStore();

const info = computed(() => {
	return dataStore.enrichedPlugins.find((i) => i.slug === props.name);
});

const sitesWithPlugin = computed(() => {
	const vulnerableSites = new Set(
		info.value?.vulnerabilities.flatMap((v) => v.sites.map((s) => s.site_id)) ||
			[],
	);
	return dataStore.plugins
		.filter((p) => p.name === props.name)
		.map((p) => {
			const site = dataStore.sites.find((s) => s.id === p.site_id);
			return {
				...p,
				site_domain: site ? site.domain : "Unknown Site",
				site_id: p.site_id,
				isVulnerable: vulnerableSites.has(p.site_id),
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
						<span class="value">{{ info?.name }}</span>
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

			<section class="card" v-if="info?.vulnerabilities && info.vulnerabilities.length > 0">
				<h2 style="color: #d32f2f;">Vulnerabilities Detected</h2>
				<div class="vulnerabilities-list">
					<div
						v-for="item in info.vulnerabilities"
						:key="item.vulnerability.uuid"
						class="vuln-item"
						style="margin-bottom: 24px; padding-bottom: 20px; border-bottom: 1px solid var(--border-color);"
					>
						<div style="display: flex; justify-content: space-between; align-items: start; gap: 10px;">
							<h3 style="margin: 0; font-size: 1.1em;">{{ item.vulnerability.name }}</h3>
							<span v-if="item.vulnerability.impact.cvss" class="status-badge error" style="white-space: nowrap;">
								{{ item.vulnerability.impact.cvss.severity }} ({{ item.vulnerability.impact.cvss.score }})
							</span>
						</div>

						<div v-for="source in item.vulnerability.source" :key="source.id" style="margin-top: 12px;">
							<p v-if="source.description" style="margin: 8px 0; line-height: 1.4; color: var(--text-main); font-size: 0.95em;">
								{{ source.description }}
							</p>
							<div class="info-grid" style="grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); margin-top: 8px; font-size: 0.85em; background: var(--bg-body); padding: 12px; border-radius: 4px;">
								<div class="info-item">
									<span class="label">Source:</span>
									<span class="value">{{ source.name }}</span>
								</div>
								<div class="info-item" v-if="item.vulnerability.operator.max_version">
									<span class="label">Max Version:</span>
									<span class="value">{{ item.vulnerability.operator.max_operator }} {{ item.vulnerability.operator.max_version }}</span>
								</div>
								<div class="info-item" v-if="source.date && source.date !== '0000-00-00'">
									<span class="label">Date:</span>
									<span class="value">{{ source.date }}</span>
								</div>
								<div class="info-item">
									<span class="label">Link:</span>
									<span class="value">
										<a :href="source.link" target="_blank" rel="noopener noreferrer" class="link">Reference</a>
									</span>
								</div>
							</div>
						</div>
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
								<th>Vuln</th>
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
								<td>
									<span v-if="item.isVulnerable" class="status-badge error">
										Yes
									</span>
									<span v-else style="color: #999">—</span>
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
