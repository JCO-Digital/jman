<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useData } from "../composables/useData";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const { getSiteById, getServerById, getPluginsBySiteId, isLoading } = useData();

const siteId = parseInt(props.id, 10);
const site = getSiteById(siteId);
const server = computed(() =>
	site.value ? getServerById(site.value.server_id).value : null,
);
const sitePlugins = getPluginsBySiteId(siteId);

const goBack = () => {
	router.push({ name: "home" });
};

const goToPlugin = (name: string) => {
	router.push({
		name: "plugin-detail",
		params: { name },
	});
};
</script>

<template>
	<div class="view-container">
		<header class="header">
			<div class="title-area">
				<button class="back-btn" @click="goBack">&larr; Back to Sites</button>
				<h1>Site Details</h1>
			</div>
		</header>

		<main class="content" v-if="site">
			<section class="card">
				<h2>Site Information</h2>
				<div class="info-grid">
					<div class="info-item">
						<span class="label">Site ID:</span>
						<span class="value">{{ site.id }}</span>
					</div>
					<div class="info-item">
						<span class="label">Domain:</span>
						<span class="value">{{ site.domain }}</span>
					</div>
					<div class="info-item">
						<span class="label">PHP Version:</span>
						<span class="value">{{ site.php_version }}</span>
					</div>
					<div class="info-item">
						<span class="label">Status:</span>
						<span class="value">{{ site.status }}</span>
					</div>
				</div>
			</section>

			<section class="card" v-if="server">
				<h2>Server Information</h2>
				<div class="info-grid">
					<div class="info-item">
						<span class="label">Server Name:</span>
						<span class="value">{{ server.name }}</span>
					</div>
					<div class="info-item">
						<span class="label">IP Address:</span>
						<span class="value">{{ server.ip_address }}</span>
					</div>
				</div>
			</section>

			<section class="card">
				<h2>Installed Plugins ({{ sitePlugins.length }})</h2>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Plugin Name</th>
								<th>Version</th>
								<th>Status</th>
							</tr>
						</thead>
						<tbody>
							<tr v-if="sitePlugins.length === 0">
								<td colspan="3" class="empty-state">No plugins found.</td>
							</tr>
							<tr
								v-for="plugin in sitePlugins"
								:key="plugin.name"
								class="clickable-row"
								@click="goToPlugin(plugin.name)"
							>
								<td>{{ plugin.name }}</td>
								<td>{{ plugin.version }}</td>
								<td>
									<span :class="['status-badge', plugin.status.toLowerCase()]">
										{{ plugin.status }}
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
					<div>Loading site details...</div>
				</div>
				<div v-else class="empty-state">
					<p>Site not found.</p>
					<button class="back-btn" @click="goBack" style="margin-top: 16px">
						Go back to sites
					</button>
				</div>
			</div>
		</main>
	</div>
</template>

<style scoped>
/* All generic styles moved to style.css */
</style>
