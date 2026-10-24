import type { UiError } from '$lib/ui/types';
import { toUiError } from './errors';
import { request, type RequestOptions } from './http';
import type { Result, TrashItem } from './types';

export interface TrashPage {
	items: TrashItem[];
	next_cursor: string | null;
	total_bytes: number;
}

export interface RestoreOptions {
	to?: string | null;
	onConflict?: 'fail' | 'keep_both' | 'replace';
}

export interface RestoredItem {
	path: string;
}

export interface EmptyTrashJob {
	job_id: string;
}

async function fetchData<T>(path: string, action: string, opts: RequestOptions = {}): Promise<Result<T, UiError>> {
	const result = await request<T>(path, opts);
	if (!result.ok) {
		return { ok: false, error: toUiError(result.error, { action }) };
	}
	return { ok: true, value: result.value.data };
}

export function listTrash(cursor: string | null): Promise<Result<TrashPage, UiError>> {
	return fetchData<TrashPage>('/v1/trash', 'trash.list', { query: { cursor } });
}

export function restore(id: string, opts: RestoreOptions = {}): Promise<Result<RestoredItem, UiError>> {
	return fetchData<RestoredItem>(`/v1/trash/${encodeURIComponent(id)}/restore`, 'trash.restore', {
		method: 'POST',
		body: { to: opts.to ?? null, on_conflict: opts.onConflict ?? 'fail' }
	});
}

export function deleteTrashItem(id: string): Promise<Result<void, UiError>> {
	return fetchData<void>(`/v1/trash/${encodeURIComponent(id)}`, 'trash.delete', { method: 'DELETE' });
}

export function emptyTrash(): Promise<Result<EmptyTrashJob, UiError>> {
	return fetchData<EmptyTrashJob>('/v1/trash', 'trash.empty', { method: 'DELETE' });
}
