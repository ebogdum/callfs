import type { ApiResponse } from '../http';
import { request } from '../http';
import type { ApiFailure } from '../errors';
import type { AuditEntry, Event, LogEntry, MetricFamily, Result, SeriesResult, Transfer } from '../types';

export type LogLevel = LogEntry['level'];
export type SeriesMeasure = SeriesResult['measure'];

export type MetricsCatalog = { generated_at: string; families: MetricFamily[] };

export type LogQuery = { level?: LogLevel; q?: string; request_id?: string; since?: string; limit?: number; cursor?: string };
export type LogPage = { items: LogEntry[]; next_cursor: string | null };

export type AuditQuery = {
	since?: string;
	until?: string;
	user?: string;
	op?: AuditEntry['op'];
	path?: string;
	status?: string;
	request_id?: string;
	limit?: number;
	cursor?: string;
};
export type AuditPage = { items: AuditEntry[]; next_cursor: string | null };

export type EventHistoryQuery = { types?: readonly string[]; since?: string };
export type EventHistory = { items: Event[] };

export type TransferList = { items: Transfer[] };

export type SeriesQuery = {
	name: string;
	measure?: SeriesMeasure;
	by?: readonly string[];
	filter?: readonly string[];
	from?: number;
	to?: number;
	stepS?: number;
};

export function joinList(values: readonly string[] | undefined): string | undefined {
	return undefined === values || 0 === values.length ? undefined : values.join(',');
}

export function dataOf<T>(result: Result<ApiResponse<T>, ApiFailure>): Result<T, ApiFailure> {
	return result.ok ? { ok: true, value: result.value.data } : result;
}

export async function metrics(names: readonly string[] = []): Promise<Result<MetricsCatalog, ApiFailure>> {
	return dataOf(await request<MetricsCatalog>('/v1/admin/metrics', { query: { names: joinList(names) } }));
}

export async function series(query: SeriesQuery): Promise<Result<SeriesResult, ApiFailure>> {
	const params = new URLSearchParams({ name: query.name });
	if (undefined !== query.measure) {
		params.set('measure', query.measure);
	}
	const by = joinList(query.by);
	if (undefined !== by) {
		params.set('by', by);
	}
	for (const expression of query.filter ?? []) {
		params.append('filter', expression);
	}
	if (undefined !== query.from) {
		params.set('from', String(query.from));
	}
	if (undefined !== query.to) {
		params.set('to', String(query.to));
	}
	if (undefined !== query.stepS) {
		params.set('step_s', String(query.stepS));
	}
	return dataOf(await request<SeriesResult>(`/v1/admin/metrics/series?${params.toString()}`));
}

export async function logs(query: LogQuery): Promise<Result<LogPage, ApiFailure>> {
	return dataOf(await request<LogPage>('/v1/admin/logs', { query }));
}

export async function audit(query: AuditQuery): Promise<Result<AuditPage, ApiFailure>> {
	return dataOf(await request<AuditPage>('/v1/admin/audit', { query }));
}

export async function eventHistory(query: EventHistoryQuery): Promise<Result<EventHistory, ApiFailure>> {
	return dataOf(await request<EventHistory>('/v1/admin/events/history', { query: { types: joinList(query.types), since: query.since } }));
}

export async function transfers(): Promise<Result<TransferList, ApiFailure>> {
	return dataOf(await request<TransferList>('/v1/admin/transfers'));
}
