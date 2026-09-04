import type { ReportColumn } from "../types";

function escapeCsvValue(value: unknown): string {
	const s = value === null || value === undefined ? "" : String(value);
	if (/[",\n]/.test(s)) return `"${s.replace(/"/g, '""')}"`;
	return s;
}

export function exportReportToCsv(
	columns: ReportColumn[],
	rows: Record<string, unknown>[],
	filename: string,
) {
	const header = columns.map((c) => escapeCsvValue(c.label)).join(",");
	const lines = rows.map((row) =>
		columns.map((c) => escapeCsvValue(row[c.key])).join(","),
	);
	const csv = [header, ...lines].join("\r\n");

	const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
	const url = URL.createObjectURL(blob);
	const a = document.createElement("a");
	a.href = url;
	a.download = filename;
	document.body.appendChild(a);
	a.click();
	document.body.removeChild(a);
	URL.revokeObjectURL(url);
}
