import type { UiError } from '$lib/ui/types';
import { toUiError } from './errors';
import { request, type RequestOptions } from './http';
import type { Capabilities, Event, Job, Me, Result } from './types';

export interface SessionGrant {
	user_id: string;
	is_admin: boolean;
	expires_at: string;
	csrf_token: string;
	idle_timeout_seconds: number;
}

export interface JobQuery {
	state?: string;
	limit?: number;
	cursor?: string | null;
}

export interface JobPage {
	items: Job[];
	next_cursor: string | null;
}

export interface EventQuery {
	since?: string | null;
	limit?: number;
}

export interface EventPage {
	items: Event[];
}

async function fetchData<T>(path: string, action: string, opts: RequestOptions = {}): Promise<Result<T, UiError>> {
	const result = await request<T>(path, opts);
	if (!result.ok) {
		return { ok: false, error: toUiError(result.error, { action }) };
	}
	return { ok: true, value: result.value.data };
}

export function signIn(key: string): Promise<Result<SessionGrant, UiError>> {
	return fetchData<SessionGrant>('/v1/session', 'session.sign_in', { method: 'POST', body: { api_key: key } });
}

export function signOut(): Promise<Result<void, UiError>> {
	return fetchData<void>('/v1/session', 'session.sign_out', { method: 'DELETE' });
}

export function me(): Promise<Result<Me, UiError>> {
	return fetchData<Me>('/v1/me', 'session.me');
}

export function capabilities(): Promise<Result<Capabilities, UiError>> {
	return fetchData<Capabilities>('/v1/capabilities', 'session.capabilities');
}

export function myJobs(query: JobQuery = {}): Promise<Result<JobPage, UiError>> {
	return fetchData<JobPage>('/v1/jobs', 'jobs.list', {
		query: { state: query.state, limit: query.limit, cursor: query.cursor }
	});
}

export function job(id: string): Promise<Result<Job, UiError>> {
	return fetchData<Job>(`/v1/jobs/${encodeURIComponent(id)}`, 'jobs.open');
}

export function cancelJob(id: string): Promise<Result<Job, UiError>> {
	return fetchData<Job>(`/v1/jobs/${encodeURIComponent(id)}/cancel`, 'jobs.cancel', { method: 'POST' });
}

export function retryJobFailed(id: string): Promise<Result<Job, UiError>> {
	return fetchData<Job>(`/v1/jobs/${encodeURIComponent(id)}/retry-failed`, 'jobs.retry', { method: 'POST' });
}

export function myEvents(query: EventQuery = {}): Promise<Result<EventPage, UiError>> {
	return fetchData<EventPage>('/v1/me/events', 'session.events', {
		query: { since: query.since, limit: query.limit }
	});
}
