<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useReportsStore } from "../../stores/reports";
import type { ReportResult } from "../../types";
import { exportReportToCsv } from "../../utils/csvExport";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import Pagination from "../../components/Pagination.vue";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const reportsStore = useReportsStore();

const today = new Date().toISOString().split("T")[0] as string;
const thirtyDaysAgo = new Date(Date.now() - 30 * 24 * 60 * 60 * 1000)
	.toISOString()
	.split("T")[0] as string;

const start = ref(thirtyDaysAgo);
const end = ref(today);
const result = ref<ReportResult | null>(null);
const isRunning = ref(false);
const error = ref<string | null>(null);

const sortKey = ref<string | null>(null);
const sortOrder = ref<"asc" | "desc">("asc");
const currentPage = ref(1);
const rowsPerPage = ref(50);

const report = computed(() => reportsStore.getReport(props.id));
const hasDateRange = computed(
	() => report.value?.params.some((p) => p.type === "daterange") ?? false,
);

onMounted(async () => {
	if (reportsStore.reports.length === 0) {
		await reportsStore.fetchReports();
	}
});

const runReport = async () => {
	isRunning.value = true;
	error.value = null;
	try {
		result.value = await reportsStore.runReport(props.id, {
			start: start.value,
			end: end.value,
		});
		sortKey.value = null;
		currentPage.value = 1;
	} catch (e: any) {
		error.value = e.message;
		result.value = null;
	} finally {
		isRunning.value = false;
	}
};

const handleSort = (key: string) => {
	if (sortKey.value === key) {
		sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
	} else {
		sortKey.value = key;
		sortOrder.value = "asc";
	}
	currentPage.value = 1;
};

const sortedRows = computed(() => {
	const rows = result.value?.rows ?? [];
	const key = sortKey.value;
	if (!key) return rows;

	return [...rows].sort((a, b) => {
		let valA: string | number = a[key] ?? "";
		let valB: string | number = b[key] ?? "";
		if (typeof valA === "string") valA = valA.toLowerCase();
		if (typeof valB === "string") valB = valB.toLowerCase();

		if (valA < valB) return sortOrder.value === "asc" ? -1 : 1;
		if (valA > valB) return sortOrder.value === "asc" ? 1 : -1;
		return 0;
	});
});

const totalPages = computed(
	() => Math.ceil(sortedRows.value.length / rowsPerPage.value) || 1,
);

const paginatedRows = computed(() => {
	const startIndex = (currentPage.value - 1) * rowsPerPage.value;
	return sortedRows.value.slice(startIndex, startIndex + rowsPerPage.value);
});

const prevPage = () => {
	if (currentPage.value > 1) currentPage.value--;
};

const nextPage = () => {
	if (currentPage.value < totalPages.value) currentPage.value++;
};

const handleRowsPerPageUpdate = (newRowsPerPage: number) => {
	rowsPerPage.value = newRowsPerPage;
	currentPage.value = 1;
};

const handleExport = () => {
	if (!result.value) return;
	exportReportToCsv(
		result.value.columns,
		result.value.rows,
		`${props.id}-${start.value}-${end.value}.csv`,
	);
};

const formatCell = (value: unknown, type: string) => {
	if (value === null || value === undefined || value === "") return "-";
	if (type === "currency") {
		return new Intl.NumberFormat("de-DE", {
			style: "currency",
			currency: "EUR",
		}).format(Number(value) / 100);
	}
	if (type === "date") {
		return new Date(String(value)).toLocaleDateString("de-DE");
	}
	return value;
};

const goBack = () => {
	router.push({ name: "reports" });
};
</script>

<template>
	<div class="view-container">
		<ViewHeader
			:title="report?.name || 'Report'"
			:back-button="{ text: 'Reports', onClick: goBack }"
		/>

		<div v-if="reportsStore.isLoading && !report" class="loading-container">
			<LoadingSpinner message="Loading report..." />
		</div>

		<div v-else-if="!report" class="empty-state">Report not found.</div>

		<template v-else>
			<p v-if="report.description" class="sub-text text-muted mb-4">
				{{ report.description }}
			</p>

			<div class="controls-row items-center">
				<template v-if="hasDateRange">
					<div class="form-group">
						<label for="report-start">Start date</label>
						<input id="report-start" v-model="start" type="date" />
					</div>
					<div class="form-group">
						<label for="report-end">End date</label>
						<input id="report-end" v-model="end" type="date" />
					</div>
				</template>
				<button
					class="btn btn-primary"
					:disabled="isRunning"
					@click="runReport"
				>
					Run Report
				</button>
				<button
					class="btn btn-secondary"
					:disabled="!result || result.rows.length === 0"
					@click="handleExport"
				>
					<AppIcon name="external-link" size="16" />
					<span>Export CSV</span>
				</button>
			</div>

			<div v-if="error" class="error-banner">{{ error }}</div>

			<div v-if="isRunning" class="loading-container">
				<LoadingSpinner message="Running report..." />
			</div>

			<main v-else-if="result" class="table-container">
				<table class="data-table sortable">
					<thead>
						<tr>
							<th
								v-for="col in result.columns"
								:key="col.key"
								@click="handleSort(col.key)"
							>
								{{ col.label }}
								<span v-if="sortKey === col.key">{{
									sortOrder === "asc" ? "↑" : "↓"
								}}</span>
							</th>
						</tr>
					</thead>
					<tbody>
						<tr v-if="result.rows.length === 0">
							<td
								:colspan="result.columns.length"
								class="empty-state"
							>
								No data for the selected range.
							</td>
						</tr>
						<tr v-for="(row, index) in paginatedRows" :key="index">
							<td v-for="col in result.columns" :key="col.key">
								{{ formatCell(row[col.key], col.type) }}
							</td>
						</tr>
					</tbody>
				</table>
				<Pagination
					v-if="result.rows.length > 0"
					:current-page="currentPage"
					:total-pages="totalPages"
					:rows-per-page="rowsPerPage"
					@update:rows-per-page="handleRowsPerPageUpdate"
					@prev="prevPage"
					@next="nextPage"
				/>
			</main>
		</template>
	</div>
</template>

<style scoped></style>
