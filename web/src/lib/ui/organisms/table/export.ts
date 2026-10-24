import { t } from '$lib/copy/t';
import type { ColumnDef, UiError } from '$lib/ui/types';

export type Result<T> = { ok: true; value: T } | { ok: false; error: UiError };

type CsvColumn<T> = ColumnDef<T> & { csv: (row: T) => string };

const CSV_NEEDS_QUOTES = /[",\r\n]/;
const CSV_FORMULA_START = /^[=+\-@\t\r]/;
const REVOKE_DELAY_MS = 1000;

const hasCsv = <T>(column: ColumnDef<T>): column is CsvColumn<T> => 'function' === typeof column.csv;

const exportColumns = <T>(columns: ColumnDef<T>[]): CsvColumn<T>[] => columns.filter(hasCsv);

const csvField = (raw: string): string => {
	const value = CSV_FORMULA_START.test(raw) ? `'${raw}` : raw;
	return CSV_NEEDS_QUOTES.test(value) ? `"${value.replaceAll('"', '""')}"` : value;
};

export const toCSV = <T>(rows: T[], columns: ColumnDef<T>[]): string => {
	const exported = exportColumns(columns);
	const header = exported.map((column) => csvField(column.header)).join(',');
	const body = rows.map((row) => exported.map((column) => csvField(column.csv(row))).join(','));
	return [header, ...body].join('\r\n');
};

export const toJSON = <T>(rows: T[], columns: ColumnDef<T>[]): string => {
	const exported = exportColumns(columns);
	const records = rows.map((row) =>
		Object.fromEntries(exported.map((column) => [column.id, column.csv(row)]))
	);
	return JSON.stringify(records, null, 2);
};

export const download = (name: string, blob: Blob): Result<void> => {
	try {
		const url = URL.createObjectURL(blob);
		const anchor = document.createElement('a');
		anchor.href = url;
		anchor.download = name;
		document.body.append(anchor);
		anchor.click();
		anchor.remove();
		window.setTimeout(() => URL.revokeObjectURL(url), REVOKE_DELAY_MS);
		return { ok: true, value: undefined };
	} catch {
		return {
			ok: false,
			error: {
				title: t('u091datatable.download_failed_title'),
				body: t('u091datatable.download_failed_body'),
				retryable: true
			}
		};
	}
};
