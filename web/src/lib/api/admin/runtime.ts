import type { ApiFailure } from '$lib/api/errors';
import { request } from '$lib/api/http';
import type { CacheStats, Locks, RateLimits, Result } from '$lib/api/types';
import { dataOf } from './topology';

export type CacheInvalidation = { prefix: string } | { all: true };

export interface LockRelease {
	released: boolean;
}

export interface CacheInvalidated {
	invalidated: number;
}

export async function locks(path?: string): Promise<Result<Locks, ApiFailure>> {
	return dataOf(await request<Locks>('/v1/admin/locks', { query: { path } }));
}

export async function forceRelease(key: string, reason: string): Promise<Result<LockRelease, ApiFailure>> {
	return dataOf(
		await request<LockRelease>(`/v1/admin/locks/${encodeURIComponent(key)}`, { method: 'DELETE', body: { reason } })
	);
}

export async function cache(): Promise<Result<CacheStats, ApiFailure>> {
	return dataOf(await request<CacheStats>('/v1/admin/cache'));
}

export async function invalidateCache(target: CacheInvalidation): Promise<Result<CacheInvalidated, ApiFailure>> {
	return dataOf(
		await request<CacheInvalidated>('/v1/admin/cache/invalidate', { method: 'POST', body: target })
	);
}

export async function rateLimits(): Promise<Result<RateLimits, ApiFailure>> {
	return dataOf(await request<RateLimits>('/v1/admin/ratelimits'));
}
