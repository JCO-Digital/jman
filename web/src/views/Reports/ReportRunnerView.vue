<script setup lang="ts">
import { ref, computed, onMounted, watch } from "vue";
import { useRouter } from "vue-router";
import { useReportsStore } from "../../stores/reports";
import type { ReportResult } from "../../types";
import { exportReportToCsv } from "../../utils/csvExport";
import ViewHeader from "../../components/ViewHeader.vue";
import LoadingSpinner from "../../components/LoadingSpinner.vue";
import AppIcon from "../../components/AppIcon.vue";
import SortableTh from "../../components/SortableTh.vue";
import Pagination from "../../components/Pagination.vue";
import { useListState } from "../../composables/useListState";
import { useClientList } from "../../composables/useClientList";
import { usePageTitle } from "../../composables/usePageTitle";

const props = defineProps<{
	id: string;
}>();

const router = useRouter();
const reportsStore = useReportsStore();

const formatDate = (date: Date) => date.toISOString().split("T")[0] as string;

const start = ref("");
const end = ref("");
const result = ref<ReportResult | null>(null);
const isRunning = ref(false);
const error = ref<string | null>(null);

const report = computed(() => reportsStore.getReport(props.id));
usePageTitle(() => report.value?.name);
const hasDateRange = computed(
	() => report.value?.params.some((p) => p.type === "daterange") ?? false,
);
const endDateParam = computed(() =>
	report.value?.params.find((p) => p.type === "enddate"),
);
const selectParams = computed(
	() => report.value?.params.filter((p) => p.type === "select") ?? [],
);
// Values of the report's select params, keyed by param key.
const selectValues = ref<Record<string, string>>({});
const missingRequired = computed(() =>
	selectParams.value.some((p) => p.required && !selectValues.value[p.key]),
);

// Defaults depend on which param type the report declares, so they can only
// be set once its metadata has loaded — mirrors the backend's own defaults
// (trailing 30 days for daterange, one month out for enddate) for a sane
// initial view before the user picks their own dates.
watch(
	report,
	(r) => {
		if (!r || start.value || end.value) return;
		if (r.params.some((p) => p.type === "daterange")) {
			const today = new Date();
			end.value = formatDate(today);
			start.value = formatDate(
				new Date(today.getTime() - 30 * 24 * 60 * 60 * 1000),
			);
		} else if (endDateParam.value) {
			const oneMonthOut = new Date();
			oneMonthOut.setMonth(oneMonthOut.getMonth() + 1);
			end.value = formatDate(oneMonthOut);
		}
	},
	{ immediate: true },
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
			...selectValues.value,
		});
		// A new result starts unsorted, on the first page.
		router.replace(list.location({ sort: "", page: 1 }));
	} catch (e: any) {
		error.value = e.message;
		result.value = null;
	} finally {
		isRunning.value = false;
	}
};

// The sort and page are in the URL (?sort=&dir=&page=&per=), like other
// lists. The sortable columns are the result's; unsorted keeps the rows in
// the report's own order.
const list = useListState<string>({
	sortKeys: () => result.value?.columns.map((c) => c.key) ?? [],
	defaultSort: "",
});

const rows = useClientList(() => result.value?.rows ?? [], list, {
	sortValue: (row, key) => row[key],
});

const handleExport = () => {
	if (!result.value) return;
	const suffix = [
		...selectParams.value.map((p) => selectValues.value[p.key]),
		start.value,
		end.value,
	]
		.filter(Boolean)
		.join("-");
	exportReportToCsv(
		result.value.columns,
		result.value.rows,
		suffix ? `${props.id}-${suffix}.csv` : `${props.id}.csv`,
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
</script>

<template>
	<div class="view-container">
		<ViewHeader
			:title="report?.name || 'Report'"
			:back="{ label: 'Reports', to: { name: 'reports' } }"
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
				<div
					v-for="param in selectParams"
					:key="param.key"
					class="form-group"
				>
					<label :for="`report-param-${param.key}`">{{
						param.label
					}}</label>
					<select
						:id="`report-param-${param.key}`"
						v-model="selectValues[param.key]"
					>
						<option value="" disabled>
							Select {{ param.label.toLowerCase() }}…
						</option>
						<option
							v-for="option in param.options ?? []"
							:key="option.value"
							:value="option.value"
						>
							{{ option.label }}
						</option>
					</select>
				</div>
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
				<div v-else-if="endDateParam" class="form-group">
					<label for="report-end-only">{{
						endDateParam.label
					}}</label>
					<input id="report-end-only" v-model="end" type="date" />
				</div>
				<button
					class="btn btn-primary"
					:disabled="isRunning || missingRequired"
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
				<table class="data-table">
					<thead>
						<tr>
							<SortableTh
								v-for="col in result.columns"
								:key="col.key"
								:list="list"
								:sort-key="col.key"
							>
								{{ col.label }}
							</SortableTh>
						</tr>
					</thead>
					<tbody>
						<tr v-if="result.rows.length === 0">
							<td
								:colspan="result.columns.length"
								class="empty-state"
							>
								{{
									selectParams.length > 0
										? "No matching results."
										: "No data for the selected range."
								}}
							</td>
						</tr>
						<tr
							v-for="(row, index) in rows.pageItems.value"
							:key="index"
						>
							<td v-for="col in result.columns" :key="col.key">
								{{ formatCell(row[col.key], col.type) }}
							</td>
						</tr>
					</tbody>
				</table>
				<Pagination
					v-if="result.rows.length > 0"
					:list="list"
					:page="rows.page.value"
					:total="rows.total.value"
				/>
			</main>
		</template>
	</div>
</template>

<style scoped></style>
