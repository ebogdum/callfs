import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import { httpFailure, toUiError, type ApiFailure, type ErrorContext } from './errors';
import { filePath, fileUrl, recentRequestRecords, request, setCsrfToken } from './http';

const { onUnauthorized } = vi.hoisted(() => ({ onUnauthorized: vi.fn<() => Promise<boolean>>() }));

vi.mock('$lib/stores/session.svelte', () => ({ session: { onUnauthorized } }));

const fetchMock = vi.fn<typeof fetch>();

function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json', ...headers }
	});
}

function hangUntilAborted(_input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
	return new Promise((_resolve, reject) => {
		const signal = init?.signal;
		const fail = () => reject(new DOMException('Aborted', 'AbortError'));
		if (true === signal?.aborted) {
			fail();
			return;
		}
		signal?.addEventListener('abort', fail);
	});
}

function headerOf(call: number, name: string): string | undefined {
	const init = fetchMock.mock.calls[call]?.[1];
	return (init?.headers as Record<string, string> | undefined)?.[name];
}

function fail(status: number, envelope: Record<string, unknown> = {}, retryAfter: string | null = null): ApiFailure {
	return httpFailure(status, JSON.stringify(envelope), retryAfter, 'req-9');
}

const CONTEXT: ErrorContext = { action: 'common.retry', object: 'Notes.txt' };
const RENAME = t('common.retry');

beforeEach(() => {
	fetchMock.mockReset();
	onUnauthorized.mockReset();
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	setCsrfToken(null);
	vi.unstubAllGlobals();
});

describe('request retries', () => {
	it('retries an idempotent GET on 503 and returns the next response', async () => {
		fetchMock
			.mockResolvedValueOnce(json(503, { code: 'UNAVAILABLE', message: 'busy' }, { 'Retry-After': '0' }))
			.mockResolvedValueOnce(json(200, { ok: 1 }));

		const result = await request<{ ok: number }>('/v1/usage');

		expect(result).toMatchObject({ ok: true, value: { data: { ok: 1 }, status: 200 } });
		expect(fetchMock).toHaveBeenCalledTimes(2);
	});

	it('does not retry a POST on 503', async () => {
		fetchMock.mockResolvedValue(json(503, { code: 'UNAVAILABLE', message: 'busy' }, { 'Retry-After': '0' }));

		const result = await request('/v1/links/generate', { method: 'POST', body: {} });

		expect(result).toMatchObject({ ok: false, error: { kind: 'http', status: 503 } });
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('gives up after three retries on a GET', async () => {
		fetchMock.mockImplementation(async () => json(502, {}, { 'Retry-After': '0' }));

		const result = await request('/v1/search');

		expect(result).toMatchObject({ ok: false, error: { kind: 'http', status: 502 } });
		expect(fetchMock).toHaveBeenCalledTimes(4);
	});
});

describe('request headers', () => {
	it('sends the CSRF header only on unsafe methods', async () => {
		setCsrfToken('csrf-1');
		fetchMock.mockResolvedValue(json(200, {}));

		await request('/v1/me');
		await request('/v1/files/a.txt', { method: 'PATCH', body: { mode: '0640' } });

		expect(headerOf(0, 'X-CallFS-CSRF')).toBeUndefined();
		expect(headerOf(1, 'X-CallFS-CSRF')).toBe('csrf-1');
		expect(headerOf(1, 'Content-Type')).toBe('application/json');
	});
});

describe('session recovery', () => {
	it('signs in through onUnauthorized and replays the request once', async () => {
		onUnauthorized.mockResolvedValueOnce(true);
		fetchMock
			.mockResolvedValueOnce(json(401, { code: 'AUTHENTICATION_FAILED', message: 'signed out' }))
			.mockResolvedValueOnce(json(200, { ok: 1 }));

		const result = await request<{ ok: number }>('/v1/me');

		expect(onUnauthorized).toHaveBeenCalledTimes(1);
		expect(fetchMock).toHaveBeenCalledTimes(2);
		expect(result).toMatchObject({ ok: true, value: { data: { ok: 1 } } });
	});

	it('does not open re-auth when the sign-in attempt itself fails', async () => {
		fetchMock.mockResolvedValue(json(401, { code: 'AUTHENTICATION_FAILED', message: 'wrong key' }));

		const result = await request('/v1/session', { method: 'POST', body: { api_key: 'nope' } });

		expect(onUnauthorized).not.toHaveBeenCalled();
		expect(result).toMatchObject({ ok: false, error: { kind: 'http', status: 401 } });
	});

	it('refreshes the CSRF token through /v1/me after CSRF_FAILED and replays once', async () => {
		setCsrfToken('stale');
		fetchMock
			.mockResolvedValueOnce(json(403, { code: 'CSRF_FAILED', message: 'csrf' }))
			.mockResolvedValueOnce(json(200, { user_id: 'alice', csrf_token: 'fresh' }))
			.mockResolvedValueOnce(json(200, { ok: 1 }));

		const result = await request('/v1/files/a.txt', { method: 'DELETE' });

		expect(fetchMock.mock.calls[1]?.[0]).toBe('/v1/me');
		expect(headerOf(2, 'X-CallFS-CSRF')).toBe('fresh');
		expect(result).toMatchObject({ ok: true, value: { status: 200 } });
		expect(fetchMock).toHaveBeenCalledTimes(3);
	});
});

describe('failure values', () => {
	it('returns a network failure value instead of throwing', async () => {
		fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));

		const result = await request('/v1/links/generate', { method: 'POST', body: {} });

		expect(result).toMatchObject({ ok: false, error: { kind: 'network' } });
	});

	it('reports a timeout when the request exceeds timeoutMs', async () => {
		fetchMock.mockImplementation(hangUntilAborted);

		const result = await request('/v1/admin/status', { method: 'POST', timeoutMs: 1 });

		expect(result).toMatchObject({ ok: false, error: { kind: 'timeout' } });
	});

	it('reports an aborted caller as an aborted failure', async () => {
		fetchMock.mockImplementation(hangUntilAborted);
		const controller = new AbortController();
		controller.abort();

		const result = await request('/v1/links/generate', { method: 'POST', body: {}, signal: controller.signal });

		expect(result).toMatchObject({ ok: false, error: { kind: 'aborted' } });
	});
});

describe('paths, queries and responses', () => {
	it('serializes query values and skips null and undefined', async () => {
		fetchMock.mockResolvedValue(json(200, {}));

		await request('/v1/search', { query: { q: 'a b', limit: 10, path: undefined, cursor: null, facets: true } });

		expect(fetchMock.mock.calls[0]?.[0]).toBe('/v1/search?q=a+b&limit=10&facets=true');
	});

	it('returns undefined data for a 204 response', async () => {
		fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

		const result = await request('/v1/session', { method: 'DELETE' });

		expect(result).toMatchObject({ ok: true, value: { data: undefined, status: 204 } });
	});

	it('encodes file path segments and keeps a trailing slash', () => {
		expect(filePath('/a b/c#d/')).toBe('a%20b/c%23d/');
		expect(filePath('/')).toBe('');
	});

	it('adds the download query to file URLs', () => {
		expect(fileUrl('/docs/x y.pdf', { download: true })).toBe('/v1/files/docs/x%20y.pdf?download=true');
	});

	it('records request ids and statuses for diagnostics', async () => {
		fetchMock.mockResolvedValue(json(200, {}, { 'X-Request-ID': 'req-1' }));

		await request('/v1/capabilities');

		expect(recentRequestRecords().at(-1)).toMatchObject({ requestId: 'req-1', status: 200 });
	});
});

describe('error mapping', () => {
	const rows: {
		name: string;
		failure: ApiFailure;
		context: ErrorContext;
		title: string;
		layer: string;
		retryable: boolean;
	}[] = [
		{ name: 'network failure', failure: { kind: 'network' }, context: CONTEXT, title: t('errors.network.title'), layer: 'network', retryable: true },
		{ name: 'timeout', failure: { kind: 'timeout' }, context: CONTEXT, title: t('errors.network.title'), layer: 'network', retryable: true },
		{ name: 'aborted caller', failure: { kind: 'aborted' }, context: CONTEXT, title: t('errors.network.title'), layer: 'network', retryable: true },
		{ name: 'malformed body', failure: { kind: 'malformed' }, context: CONTEXT, title: t('errors.server.title', { action: RENAME }), layer: 'server', retryable: false },
		{ name: '400 bad request', failure: fail(400, { code: 'BAD_REQUEST', message: 'name is empty' }), context: CONTEXT, title: t('errors.bad_request.title', { action: RENAME }), layer: 'server', retryable: false },
		{ name: '401 signed out', failure: fail(401, { code: 'AUTHENTICATION_FAILED' }), context: CONTEXT, title: t('errors.unauthenticated.title'), layer: 'auth', retryable: true },
		{ name: '403 not owner', failure: fail(403, { code: 'PERMISSION_DENIED', details: { owner: 'alice' } }), context: CONTEXT, title: t('errors.permission.title', { action: RENAME }), layer: 'permission', retryable: false },
		{ name: '403 admin only', failure: fail(403, { code: 'ADMIN_REQUIRED' }), context: CONTEXT, title: t('errors.admin_required.title'), layer: 'permission', retryable: false },
		{ name: '404 missing', failure: fail(404, { code: 'FILE_NOT_FOUND' }), context: CONTEXT, title: t('errors.not_found.title', { object: 'Notes.txt' }), layer: 'not_found', retryable: false },
		{ name: '404 feature off', failure: fail(404, { code: 'FEATURE_DISABLED' }), context: { action: 'common.retry', object: 'Trash' }, title: t('errors.feature_disabled.title', { feature: 'Trash' }), layer: 'not_found', retryable: false },
		{ name: '409 locked', failure: fail(409, { code: 'RESOURCE_LOCKED' }), context: CONTEXT, title: t('errors.locked.title'), layer: 'locked', retryable: true },
		{ name: '409 already exists', failure: fail(409, { code: 'FILE_ALREADY_EXISTS' }), context: CONTEXT, title: t('errors.conflict.title', { object: 'Notes.txt' }), layer: 'conflict', retryable: false },
		{ name: '410 link gone', failure: fail(410, {}), context: CONTEXT, title: t('errors.gone.title'), layer: 'not_found', retryable: false },
		{ name: '412 changed since opened', failure: fail(412, {}), context: CONTEXT, title: t('errors.precondition.title'), layer: 'precondition', retryable: false },
		{ name: '422 validation', failure: fail(422, { code: 'VALIDATION_FAILED', message: 'name is required', details: { errors: [{ path: 'name', message: 'x' }, { path: 'mode', message: 'y' }] } }), context: CONTEXT, title: t('errors.validation.title'), layer: 'server', retryable: false },
		{ name: '429 rate limited', failure: fail(429, { code: 'RATE_LIMIT_EXCEEDED' }, '5'), context: CONTEXT, title: t('errors.rate_limited.title'), layer: 'limit', retryable: true },
		{ name: '500 on a write', failure: fail(500, { code: 'INTERNAL_ERROR' }), context: CONTEXT, title: t('errors.server.title', { action: RENAME }), layer: 'server', retryable: false },
		{ name: '500 on a read', failure: fail(500, { code: 'INTERNAL_ERROR' }), context: { ...CONTEXT, read: true }, title: t('errors.server.title', { action: RENAME }), layer: 'server', retryable: true },
		{ name: '502 peer did not answer', failure: fail(502, { code: 'PEER_UNAVAILABLE', details: { instance: 'node-b' } }), context: CONTEXT, title: t('errors.peer.title'), layer: 'peer', retryable: true }
	];

	it.each(rows)('maps $name', ({ failure, context, title, layer, retryable }) => {
		expect(toUiError(failure, context)).toMatchObject({ title, layer, retryable });
	});

	it('keeps status, code and request id and fills body and fix links', () => {
		expect(toUiError(fail(404, { code: 'FEATURE_DISABLED' }), { action: 'common.retry', object: 'Trash' })).toMatchObject({
			fixAction: { href: '/settings' }
		});
		expect(toUiError(fail(422, { message: 'name is required', details: { errors: [{}, {}] } }), CONTEXT)).toMatchObject({
			body: t('errors.validation.body', { first: 'name is required', count: 2 })
		});
	});
});
