<script setup lang="ts">
import { onMounted } from "vue";
import { useVulnBackportStore } from "../../stores/vulnBackports";
import { useToastStore } from "../../stores/toast";
import { useConfirm } from "../../composables/useConfirm";
import AppIcon from "../AppIcon.vue";
import LoadingSpinner from "../LoadingSpinner.vue";

const backportStore = useVulnBackportStore();
const toast = useToastStore();
const { confirm } = useConfirm();

onMounted(async () => {
	try {
		await backportStore.fetchRules();
	} catch (e: any) {
		toast.addToast(e.message || "Failed to load backport rules", "error");
	}
});

const handleRemove = async (id: number) => {
	if (
		!(await confirm(
			"Remove this backport rule? Matching sites will be reported as affected again.",
			{ danger: true },
		))
	)
		return;

	try {
		await backportStore.deleteRule(id);
		toast.addToast("Backport rule removed", "success");
	} catch (e: any) {
		toast.addToast(e.message || "Failed to remove rule", "error");
	}
};
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Backport Rules</h2>
		</div>
		<p class="sub-text mb-4">
			Vulnerabilities fixed by backport releases the vulnerability
			database doesn't list. Matching versions aren't reported as
			affected. Add rules with "Mark patched…" on an affected version.
		</p>

		<div
			v-if="backportStore.isLoading && backportStore.rules.length === 0"
			class="loading-state"
		>
			<LoadingSpinner message="Loading backport rules..." />
		</div>

		<div v-else-if="backportStore.rules.length > 0" class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th>Component</th>
						<th>Vulnerability</th>
						<th>Patched in</th>
						<th>Sites</th>
						<th class="hide-mobile">Reason</th>
						<th class="text-right">Actions</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="rule in backportStore.rules" :key="rule.id">
						<td class="font-medium text-main">
							<span v-if="rule.type === 'core'"
								>WordPress core</span
							>
							<router-link
								v-else
								:to="{
									name: 'plugin-detail',
									params: { name: rule.slug },
								}"
							>
								{{ rule.slug }}
							</router-link>
						</td>
						<td>
							<span class="font-sm" :title="rule.vuln_uuid">{{
								rule.vuln_uuid.length > 12
									? rule.vuln_uuid.substring(0, 12) + "..."
									: rule.vuln_uuid
							}}</span>
						</td>
						<td>{{ rule.branch }}.x from {{ rule.min_version }}</td>
						<td>
							<span
								class="status-badge badge-sm"
								:class="rule.matched_sites > 0 ? 'active' : ''"
								:title="
									rule.matched_sites > 0
										? 'Sites whose installed version this rule matches'
										: 'No site currently runs a matching version'
								"
								>{{ rule.matched_sites }}</span
							>
						</td>
						<td class="text-muted font-sm hide-mobile">
							{{ rule.reason }}
							<div class="font-xs">— {{ rule.created_by }}</div>
						</td>
						<td class="text-right">
							<button
								class="icon-btn danger"
								title="Remove Rule"
								@click="handleRemove(rule.id)"
							>
								<AppIcon name="trash" size="18" />
							</button>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">No backport rules.</p>
		</div>
	</section>
</template>
