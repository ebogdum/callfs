import type { UiError } from '$lib/ui/types';
import { toUiError } from './errors';
import { filePath, nodePath, request, type RequestOptions } from './http';
import type { Result, SplitFile, SplitFileDetail, VerifyReport } from './types';

export type SplitHealth = SplitFile['health'];

export interface SplitFileFilter {
	prefix?: string;
	health?: SplitHealth;
	limit?: number;
}

export interface SplitFilePage {
	items: SplitFile[];
	next_cursor: string | null;
}

export interface ChunkManifest {
	path: string;
	original_size: number;
	erasure_profile: { data_shards: number; parity_shards: number; shard_size: number };
	shards: { index: number; endpoint: string }[];
}

export interface RepairJob {
	job_id: string;
}

async function fetchData<T>(path: string, action: string, opts: RequestOptions = {}): Promise<Result<T, UiError>> {
	const result = await request<T>(path, opts);
	if (!result.ok) {
		return { ok: false, error: toUiError(result.error, { action }) };
	}
	return { ok: true, value: result.value.data };
}

export function listSplitFiles(filter: SplitFileFilter, cursor: string | null): Promise<Result<SplitFilePage, UiError>> {
	return fetchData<SplitFilePage>('/v1/erasure/files', 'erasure.list', {
		query: {
			prefix: filter.prefix,
			health: filter.health,
			limit: filter.limit,
			cursor
		}
	});
}

export function getSplitFile(path: string): Promise<Result<SplitFileDetail, UiError>> {
	return fetchData<SplitFileDetail>(`/v1/erasure/files/${filePath(path)}`, 'erasure.open');
}

export function manifest(path: string): Promise<Result<ChunkManifest, UiError>> {
	return fetchData<ChunkManifest>(`/v1/erasure/manifest/${filePath(path)}`, 'erasure.manifest');
}

export function verify(path: string): Promise<Result<VerifyReport, UiError>> {
	return fetchData<VerifyReport>(`/v1/erasure/verify/${filePath(path)}`, 'erasure.verify', { method: 'POST' });
}

export function repair(path: string): Promise<Result<RepairJob, UiError>> {
	return fetchData<RepairJob>(`/v1/erasure/repair/${filePath(path)}`, 'erasure.repair', { method: 'POST' });
}

export function pieceUrl(path: string, index: number): string {
	return nodePath(`/v1/shards/${filePath(path)}/${index}`);
}
