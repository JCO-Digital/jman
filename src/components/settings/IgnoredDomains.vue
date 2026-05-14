<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useMonitorStore } from "../../stores/monitor";
import { useAuthStore } from "../../stores/auth";
import { useToastStore } from "../../stores/toast";
import LoadingSpinner from "../LoadingSpinner.vue";

const monitorStore = useMonitorStore();
const authStore = useAuthStore();
const toast = useToastStore();

const newDomain = ref("");
const newReason = ref("");
const isSubmitting = ref(false);
const error = ref<string | null>(null);

onMounted(() => {
	monitorStore.fetchIgnored();
});

const handleAddIgnored = async () => {
	if (!newDomain.value) return;

	isSubmitting.value = true;
	error.value = null;
	try {
		await monitorStore.addIgnored(newDomain.value, newReason.value);
		newDomain.value = "";
		newReason.value = "";
	} catch (e: any) {
		error.value = e.message || "Failed to add domain to ignore list";
	} finally {
		isSubmitting.value = false;
	}
};

const handleRemoveIgnored = async (domain: string) => {
	if (!confirm(`Are you sure you want to stop ignoring ${domain}?`)) return;

	try {
		await monitorStore.removeIgnored(domain);
	} catch (e: any) {
		toast.addToast(e.message || "Failed to remove domain", "error");
	}
};

const formatDate = (dateStr: string) => {
	return new Date(dateStr).toLocaleString(undefined, {
		year: "numeric",
		month: "short",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	});
};
</script>

<template>
	<section class="card">
		<div class="card-header">
			<h2>Ignored Domains</h2>
		</div>
		<p class="sub-text mb-4">
			Sites in this list are excluded from uptime monitoring.
		</p>

		<!-- Add form -->
		<form
			v-if="authStore.canEdit"
			class="card-muted mb-4"
			@submit.prevent="handleAddIgnored"
		>
			<div class="flex-row gap-3">
				<input
					v-model="newDomain"
					type="text"
					placeholder="domain.com"
					required
					style="flex: 1"
				/>
				<input
					v-model="newReason"
					type="text"
					placeholder="Reason (optional)"
					style="flex: 1"
				/>
				<button
					type="submit"
					class="btn btn-primary"
					:disabled="isSubmitting"
				>
					{{ isSubmitting ? "Adding..." : "Add to list" }}
				</button>
			</div>
			<p v-if="error" class="text-error font-xs mt-2">{{ error }}</p>
		</form>

		<div v-if="monitorStore.isLoadingIgnored" class="loading-state">
			<LoadingSpinner message="Loading ignored domains..." />
		</div>

		<div
			v-else-if="monitorStore.ignoredDomains.length > 0"
			class="table-container"
		>
			<table class="data-table">
				<thead>
					<tr>
						<th>Domain</th>
						<th class="hide-mobile">Reason</th>
						<th class="hide-mobile">Added At</th>
						<th v-if="authStore.canEdit" class="text-right">
							Actions
						</th>
					</tr>
				</thead>
				<tbody>
					<tr
						v-for="site in monitorStore.ignoredDomains"
						:key="site.domain"
					>
						<td class="font-medium text-main">{{ site.domain }}</td>
						<td class="hide-mobile">{{ site.reason || "—" }}</td>
						<td class="hide-mobile">
							<span class="sub-text">{{
								formatDate(site.created_at)
							}}</span>
						</td>
						<td v-if="authStore.canEdit" class="text-right">
							<button
								class="btn btn-text danger"
								@click="handleRemoveIgnored(site.domain)"
							>
								Remove
							</button>
						</td>
					</tr>
				</tbody>
			</table>
		</div>

		<div v-else class="loading-state">
			<p class="text-muted">No domains are currently ignored.</p>
		</div>
	</section>
</template>

<style scoped>
/* Scoped styles removed in favor of global utility classes and component styles */
</style>
