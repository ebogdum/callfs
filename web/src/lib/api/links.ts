import type { UiError } from '$lib/ui/types';
import { toUiError } from './errors';
import { request, type RequestOptions } from './http';
import type { Link, LinkPage, LinkStats, Result } from './types';

export type LinkStatus = Link['status'];
export type LinkWindow = LinkStats['window'];

export interface LinkFilter {
	status?: LinkStatus;
	path?: string;
	createdBy?: string;
	limit?: number;
}

export interface RevokedLink {
	id: number;
	status: 'revoked';
}

export interface GeneratedLink {
	id: number;
	url: string;
	token: string;
	expires: string;
}

async function fetchData<T>(path: string, action: string, opts: RequestOptions = {}): Promise<Result<T, UiError>> {
	const result = await request<T>(path, opts);
	if (!result.ok) {
		return { ok: false, error: toUiError(result.error, { action }) };
	}
	return { ok: true, value: result.value.data };
}

export function listLinks(filter: LinkFilter, cursor: string | null): Promise<Result<LinkPage, UiError>> {
	return fetchData<LinkPage>('/v1/links', 'links.list', {
		query: {
			status: filter.status,
			path: filter.path,
			created_by: filter.createdBy,
			limit: filter.limit,
			cursor
		}
	});
}

export function getLink(id: number): Promise<Result<Link, UiError>> {
	return fetchData<Link>(`/v1/links/${id}`, 'links.open');
}

export function revokeLink(id: number): Promise<Result<RevokedLink, UiError>> {
	return fetchData<RevokedLink>(`/v1/links/${id}`, 'links.revoke', { method: 'DELETE' });
}

export function linkStats(window: LinkWindow): Promise<Result<LinkStats, UiError>> {
	return fetchData<LinkStats>('/v1/links/stats', 'links.stats', { query: { window } });
}

export function generateLink(path: string, expirySeconds: number): Promise<Result<GeneratedLink, UiError>> {
	return fetchData<GeneratedLink>('/v1/links/generate', 'links.generate', {
		method: 'POST',
		body: { path, expiry_seconds: expirySeconds }
	});
}
