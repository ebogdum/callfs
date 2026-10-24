import { request } from './http';
import type { PermissionDecision, SearchQuery, SearchResult, Usage } from './types';

export interface UsageOptions {
	depth?: number;
	owner?: string;
	histograms?: boolean;
}

export function search(query: SearchQuery, cursor: string | null = null) {
	return request<SearchResult>('/v1/search', { query: { ...query, cursor } });
}

export function usage(path: string, opts: UsageOptions = {}) {
	return request<Usage>('/v1/usage', {
		query: { path, depth: opts.depth, owner: opts.owner, histograms: opts.histograms }
	});
}

export function permissionCheck(path: string, perm: PermissionDecision['perm'], user?: string) {
	return request<PermissionDecision>('/v1/permissions/check', { query: { path, perm, user } });
}
