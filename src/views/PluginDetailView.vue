<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useData } from "../composables/useData";

const props = defineProps<{
	name: string;
}>();

const router = useRouter();
const { plugins, sites, isLoading } = useData();

const pluginName = computed(() => decodeURIComponent(props.name));

const sitesWithPlugin = computed(() => {
	const name = pluginName.value;
	return plugins.value
		.filter((p) => p.name === name)
		.map((p) => {
			const site = sites.value.find((s) => s.id === p.site_id);
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
	<div class="plugin-detail-view">
		<header class="header">
			<div class="title-area">
				<button class="back-btn" @click="goBack">&larr; Back to Plugins</button>
				<h1>Plugin Details</h1>
			</div>
		</header>

		<main class="content" v-if="sitesWithPlugin.length > 0">
			<section class="card">
				<h2>Plugin Information</h2>
				<div class="info-grid">
					<div class="info-item">
						<span class="label">Plugin Name:</span>
						<span class="value">{{ pluginName }}</span>
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
					<table class="install-table">
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
				<div v-if="isLoading" class="empty-state">
					<div class="spinner" style="margin-bottom: 12px"></div>
					<div>Loading plugin details...</div>
				</div>
				<div v-else class="empty-state">
					<p>Plugin details not found or plugin is not installed on any sites.</p>
					<button class="back-btn" @click="goBack" style="margin-top: 16px">
						Go back to plugins
					</button>
				</div>
			</div>
		</main>
	</div>
</template>

<style scoped>
.plugin-detail-view {
	max-width: 1000px;
	width: 95%;
	margin: 0 auto;
	padding: 20px;
}

.header {
	margin-bottom: 24px;
}

.title-area {
	display: flex;
	align-items: center;
	gap: 16px;
}

.title-area h1 {
	margin: 0;
	font-size: 24px;
}

.back-btn {
	padding: 8px 12px;
	background-color: transparent;
	border: 1px solid var(--border-input);
	border-radius: 4px;
	cursor: pointer;
	color: var(--text-main);
	font-weight: 500;
}

.back-btn:hover {
	background-color: var(--bg-hover);
}

.content {
	display: flex;
	flex-direction: column;
	gap: 24px;
}

.card {
	background: var(--bg-card);
	border: 1px solid var(--border-color);
	border-radius: 8px;
	padding: 20px;
}

.card h2 {
	margin-top: 0;
	margin-bottom: 16px;
	font-size: 18px;
	color: var(--text-heading);
	border-bottom: 1px solid var(--border-color);
	padding-bottom: 8px;
}

.info-grid {
	display: grid;
	grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
	gap: 16px;
}

.info-item {
	display: flex;
	flex-direction: column;
	gap: 4px;
}

.label {
	font-size: 12px;
	text-transform: uppercase;
	color: var(--text-muted);
	font-weight: 600;
	letter-spacing: 0.05em;
}

.value {
	font-size: 16px;
	color: var(--text-heading);
}

.table-container {
	border: 1px solid var(--border-color);
	border-radius: 6px;
	overflow: hidden;
}

.install-table {
	width: 100%;
	border-collapse: collapse;
	text-align: left;
}

.install-table th,
.install-table td {
	padding: 10px 16px;
	border-bottom: 1px solid var(--border-color);
}

.install-table th {
	background-color: var(--bg-table-header);
	font-weight: 600;
	color: var(--text-table-header);
}

.clickable-row {
	cursor: pointer;
}

.clickable-row:hover td {
	background-color: var(--bg-hover);
}

.status-badge {
	display: inline-block;
	padding: 2px 8px;
	border-radius: 9999px;
	font-size: 12px;
	font-weight: 500;
	text-transform: capitalize;
	background-color: var(--badge-default-bg);
	color: var(--badge-default-text);
}

.status-badge.active {
	background-color: var(--badge-active-bg);
	color: var(--badge-active-text);
}

.status-badge.inactive,
.status-badge.disabled {
	background-color: var(--badge-inactive-bg);
	color: var(--badge-inactive-text);
}

.empty-state {
	text-align: center;
	padding: 24px;
	color: var(--text-muted);
}
</style>
