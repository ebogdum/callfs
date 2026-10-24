import { request } from './http';
import type { ObjectPage } from './types';

export interface ObjectInfo {
	path: string;
	size: number;
	etag: string;
	mtime: string;
}

export interface ObjectListPage {
	objects: ObjectInfo[];
	truncated: boolean;
	next_start_after?: string;
}

export interface PrefixDeletePlan {
	would_delete: number;
	bytes: number;
	forbidden: number;
}

export interface PrefixDeleteResult {
	deleted: number;
}

export function listObjects(prefix: string, startAfter: string | null, limit: number) {
	return request<ObjectListPage>('/v1/objects', {
		query: { prefix, start_after: startAfter, limit }
	});
}

export function countObjects(prefix: string) {
	return request<ObjectPage>('/v1/objects', { query: { prefix, count_only: true } });
}

export function planPrefixDelete(prefix: string) {
	return request<PrefixDeletePlan>('/v1/objects', {
		method: 'DELETE',
		query: { prefix, boundary: true, dry_run: true }
	});
}

export function deletePrefix(prefix: string) {
	return request<PrefixDeleteResult>('/v1/objects', {
		method: 'DELETE',
		query: { prefix, boundary: true }
	});
}
