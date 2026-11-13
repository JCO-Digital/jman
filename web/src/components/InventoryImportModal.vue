<script setup lang="ts">
import { computed, ref } from "vue";
import AppIcon from "./AppIcon.vue";
import { useInventoryStore } from "../stores/inventory";
import { displayDomain } from "../utils/format";
import type {
	InventoryImportAction,
	InventoryImportOptions,
	InventoryImportResult,
	InventoryImportRow,
} from "../types";

const emit = defineEmits<{
	(e: "imported", result: InventoryImportResult): void;
	(e: "close"): void;
}>();

const inventoryStore = useInventoryStore();

const fileName = ref("");
const csvText = ref("");
const result = ref<InventoryImportResult | null>(null);
const loading = ref(false);
const error = ref<string | null>(null);
/** The options the shown preview was made with, as JSON. */
const previewedOptions = ref("");

const domainColumn = ref("");
const overwrite = ref(false);
/** Per column header: the details key to store it under, or skipped. */
const columns = ref<Record<string, { key: string; skip: boolean }>>({});

const options = computed<InventoryImportOptions>(() => ({
	domain_column: domainColumn.value || undefined,
	overwrite: overwrite.value,
	columns: Object.fromEntries(
		Object.entries(columns.value).map(([header, c]) => [
			header,
			c.skip ? "" : c.key.trim(),
		]),
	),
}));

// True when the options changed since the preview was made: Import then
// waits for an updated preview, so what is imported is what was shown.
const stale = computed(
	() =>
		!!result.value &&
		JSON.stringify(options.value) !== previewedOptions.value,
);

// decodeFile reads a CSV as UTF-8, falling back to Windows-1252, which is
// what Excel saves "CSV" as on many systems.
const decodeFile = async (file: File): Promise<string> => {
	const bytes = await file.arrayBuffer();
	try {
		return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
	} catch {
		return new TextDecoder("windows-1252").decode(bytes);
	}
};

const onFile = async (event: Event) => {
	const file = (event.target as HTMLInputElement).files?.[0];
	if (!file) return;
	fileName.value = file.name;
	csvText.value = await decodeFile(file);
	result.value = null;
	domainColumn.value = "";
	columns.value = {};
	await preview();
};

const preview = async () => {
	if (!csvText.value) return;
	loading.value = true;
	error.value = null;
	try {
		const res = await inventoryStore.importDomains(
			csvText.value,
			options.value,
			true,
		);
		// The first preview of a file sets up the column options.
		if (!domainColumn.value) domainColumn.value = res.domain_column;
		for (const header of res.headers) {
			if (!columns.value[header]) {
				columns.value[header] = { key: header, skip: false };
			}
		}
		result.value = res;
		previewedOptions.value = JSON.stringify(options.value);
	} catch (e: any) {
		error.value = e.message;
		result.value = null;
	} finally {
		loading.value = false;
	}
};

const runImport = async () => {
	if (!result.value || stale.value) return;
	loading.value = true;
	error.value = null;
	try {
		const res = await inventoryStore.importDomains(
			csvText.value,
			options.value,
			false,
		);
		emit("imported", res);
	} catch (e: any) {
		error.value = e.message;
	} finally {
		loading.value = false;
	}
};

// The columns other than the domain column, with their options.
const otherColumns = computed(() =>
	(result.value?.headers ?? []).flatMap((header) => {
		const col = columns.value[header];
		return header !== domainColumn.value && col ? [{ header, col }] : [];
	}),
);

type Filter = "all" | InventoryImportAction | "conflicts";
const filter = ref<Filter>("all");

const filters = computed(() => {
	const s = result.value?.summary;
	return [
		{ value: "all" as Filter, label: "All" },
		{ value: "create" as Filter, label: `New (${s?.create ?? 0})` },
		{ value: "update" as Filter, label: `Updated (${s?.update ?? 0})` },
		{
			value: "unchanged" as Filter,
			label: `Unchanged (${s?.unchanged ?? 0})`,
		},
		{
			value: "conflicts" as Filter,
			label: `Conflicts (${s?.conflicts ?? 0})`,
		},
		{ value: "skip" as Filter, label: `Skipped (${s?.skip ?? 0})` },
	];
});

const visibleRows = computed(() =>
	(result.value?.rows ?? []).filter((r) => {
		if (filter.value === "all") return true;
		if (filter.value === "conflicts") return (r.conflicts ?? []).length > 0;
		return r.action === filter.value;
	}),
);

const actionLabel: Record<InventoryImportAction, string> = {
	create: "new",
	update: "update",
	unchanged: "unchanged",
	skip: "skipped",
};
const actionClass: Record<InventoryImportAction, string> = {
	create: "active",
	update: "info",
	unchanged: "inactive",
	skip: "warning",
};

const canImport = computed(
	() =>
		!!result.value &&
		!stale.value &&
		!loading.value &&
		result.value.summary.create + result.value.summary.update > 0,
);

const changeSummary = (row: InventoryImportRow) =>
	(row.changes ?? []).join(", ");
</script>

<template>
	<div class="modal-overlay" @click.self="emit('close')">
		<div class="modal-content card modal-lg">
			<div class="modal-header">
				<h2>Import domains</h2>
				<button
					class="modal-close"
					title="Close"
					@click="emit('close')"
				>
					<AppIcon name="x" size="18" />
				</button>
			</div>

			<p class="text-muted">
				Choose a CSV file with a header row. The domain is read from the
				start of its cell; any further text is kept as a note. Other
				columns become fields of the item's details. A domain already in
				the inventory gets the fields merged in. Nothing is saved until
				you press Import.
			</p>

			<div class="form-group">
				<label for="import-file">CSV file</label>
				<input
					id="import-file"
					type="file"
					accept=".csv,text/csv,text/plain"
					@change="onFile"
				/>
			</div>

			<p v-if="error" class="form-error">{{ error }}</p>

			<template v-if="result">
				<div class="form-group">
					<label for="import-domain-column">Domain column</label>
					<select id="import-domain-column" v-model="domainColumn">
						<option v-for="h in result.headers" :key="h" :value="h">
							{{ h }}
						</option>
					</select>
				</div>

				<div class="form-group">
					<label>Other columns</label>
					<div class="table-container">
						<table class="data-table">
							<thead>
								<tr>
									<th>Column</th>
									<th>Store as</th>
									<th>Skip</th>
								</tr>
							</thead>
							<tbody>
								<tr
									v-for="{ header, col } in otherColumns"
									:key="header"
								>
									<td>{{ header }}</td>
									<td>
										<input
											v-model="col.key"
											type="text"
											:disabled="col.skip"
											:aria-label="
												'Details key for ' + header
											"
										/>
									</td>
									<td>
										<input
											v-model="col.skip"
											type="checkbox"
											:aria-label="'Skip ' + header"
										/>
									</td>
								</tr>
							</tbody>
						</table>
					</div>
				</div>

				<label class="flex-row gap-2 items-center">
					<input v-model="overwrite" type="checkbox" />
					Overwrite existing values that differ (otherwise they are
					kept and listed as conflicts)
				</label>

				<div v-if="stale" class="flex-row gap-2 items-center mt-4">
					<span class="text-muted"
						>The options changed since the preview.</span
					>
					<button
						class="btn btn-outline btn-sm"
						:disabled="loading"
						@click="preview"
					>
						<AppIcon name="refresh" size="16" />
						Update preview
					</button>
				</div>

				<h3 class="mt-4">
					Preview of {{ fileName }}: {{ result.summary.create }} new,
					{{ result.summary.update }} updated,
					{{ result.summary.unchanged }} unchanged,
					{{ result.summary.skip }} skipped
				</h3>

				<div class="tabs">
					<button
						v-for="f in filters"
						:key="f.value"
						:class="['tab', { active: filter === f.value }]"
						@click="filter = f.value"
					>
						{{ f.label }}
					</button>
				</div>

				<div class="table-container">
					<table class="data-table">
						<thead>
							<tr>
								<th>Line</th>
								<th>Domain</th>
								<th>Action</th>
								<th>Details</th>
							</tr>
						</thead>
						<tbody>
							<tr
								v-for="row in visibleRows"
								:key="row.lines.join(',')"
							>
								<td>{{ row.lines.join(", ") }}</td>
								<td>
									<template v-if="row.domain">
										{{ displayDomain(row.domain) }}
									</template>
									<div class="text-muted">{{ row.raw }}</div>
								</td>
								<td>
									<span
										:class="[
											'status-badge',
											'badge-sm',
											actionClass[row.action],
										]"
									>
										{{ actionLabel[row.action] }}
									</span>
								</td>
								<td>
									<div v-if="row.reason">
										{{ row.reason }}
									</div>
									<div v-if="(row.changes ?? []).length">
										Sets: {{ changeSummary(row) }}
									</div>
									<div
										v-for="c in row.conflicts ?? []"
										:key="c.key + c.imported"
										class="form-error"
									>
										{{ c.key }}:
										<template v-if="c.in_file">
											the file also has "{{ c.imported }}"
											on a later row; keeping "{{
												c.existing
											}}"
										</template>
										<template v-else>
											keeping "{{ c.existing }}", not "{{
												c.imported
											}}"
										</template>
									</div>
								</td>
							</tr>
						</tbody>
					</table>
				</div>
			</template>

			<div class="form-actions">
				<button
					type="button"
					class="btn btn-outline"
					@click="emit('close')"
				>
					Cancel
				</button>
				<button
					type="button"
					class="btn btn-primary"
					:disabled="!canImport"
					@click="runImport"
				>
					<AppIcon name="upload" size="18" />
					{{ loading ? "Working..." : "Import" }}
				</button>
			</div>
		</div>
	</div>
</template>
