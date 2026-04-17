<script setup lang="ts">
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useDataStore } from "../stores/data";
import ViewHeader from "../components/ViewHeader.vue";
import LoadingSpinner from "../components/LoadingSpinner.vue";
import InfoCard from "../components/InfoCard.vue";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const dataStore = useDataStore();

const siteId = parseInt(props.id, 10);
const site = computed(() => dataStore.getSiteById(siteId));
const server = computed(() =>
	site.value ? dataStore.getServerById(site.value.server_id) : null,
);
const sitePlugins = computed(() => {
	const siteVulns = dataStore.vulnerabilitiesBySiteId.get(siteId) || [];
	return dataStore.getPluginsBySiteId(siteId).map((plugin) => {
		const vulns = siteVulns.filter((v) => v.slug === plugin.name);
		return {
			...plugin,
			vulnerabilities: vulns,
		};
	});
});

const siteInfoItems = computed(() => {
	if (!site.value) return [];
	return [
		{ label: "Site ID", value: site.value.id },
		{
			label: "Domain",
			value: site.value.domain,
			copyable: true,
			isLink: true,
			href: site.value.domain.startsWith("http")
				? site.value.domain
				: `https://${site.value.domain}`,
		},
		{ label: "PHP Version", value: site.value.php_version },
		{ label: "Status", value: site.value.status },
	];
});

const serverInfoItems = computed(() => {
	if (!server.value) return [];
	return [
		{ label: "Server Name", value: server.value.name, copyable: true },
		{ label: "IP Address", value: server.value.ip_address, copyable: true },
	];
});

const goBack = () => {
	router.push({ name: "sites" });
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
		<ViewHeader
			title="Site Details"
			:back-button="{ text: 'Back to Sites', onClick: goBack }"
		/>

		<main class="content" v-if="site">
			<InfoCard title="Site Information" :items="siteInfoItems" />

			<InfoCard
				v-if="server"
				title="Server Information"
				:items="serverInfoItems"
			/>

			<section class="card">
				<h2>Installed Plugins ({{ sitePlugins.length }})</h2>
				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Plugin Name</th>
								<th>Version</th>
								<th>Status</th>
								<th>Vulns</th>
							</tr>
						</thead>
						<tbody>
							<tr v-if="sitePlugins.length === 0">
								<td colspan="4" class="empty-state">No plugins found.</td>
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
								<td>
									<span
										v-if="plugin.vulnerabilities.length > 0"
										class="status-badge error"
										:title="`${plugin.vulnerabilities.length} vulnerabilities detected`"
									>
										{{ plugin.vulnerabilities.length }}
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
				<LoadingSpinner
					v-if="dataStore.isLoading"
					message="Loading site details..."
				/>
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
/* Specific styles can remain if needed, but standard table styles are in style.css */
</style>
