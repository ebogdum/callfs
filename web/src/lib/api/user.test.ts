import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cancelJob, me, myEvents, myJobs, signIn, signOut } from './session';
import { generateLink, listLinks, revokeLink } from './links';
import { getSplitFile, listSplitFiles, pieceUrl, repair } from './erasure';
import { deleteTrashItem, emptyTrash, listTrash, restore } from './trash';
import { setCsrfToken } from './http';

vi.mock('$lib/stores/session.svelte', () => ({ session: { onUnauthorized: vi.fn(async () => false) } }));

const fetchMock = vi.fn<typeof fetch>();

function json(status: number, body: unknown): Response {
	return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function callOf(index: number): { url: string; method: string; body: unknown; csrf: string | undefined } {
	const [url, init] = fetchMock.mock.calls[index] as [string, RequestInit];
	const headers = init.headers as Record<string, string>;
	return {
		url,
		method: init.method ?? 'GET',
		body: undefined === init.body ? undefined : JSON.parse(init.body as string),
		csrf: headers['X-CallFS-CSRF']
	};
}

beforeEach(() => {
	fetchMock.mockReset();
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	setCsrfToken(null);
	vi.unstubAllGlobals();
});

describe('session api', () => {
	it('posts the key once and returns the grant', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { user_id: 'alice', is_admin: true, expires_at: '2026-10-24T00:00:00Z', csrf_token: 'tok' }));

		const result = await signIn('secret-key');

		expect(callOf(0)).toMatchObject({ url: '/v1/session', method: 'POST', body: { api_key: 'secret-key' } });
		expect(result).toMatchObject({ ok: true, value: { user_id: 'alice', is_admin: true, csrf_token: 'tok' } });
	});

	it('maps a rejected key to an auth error', async () => {
		fetchMock.mockResolvedValueOnce(json(401, { code: 'AUTHENTICATION_FAILED', message: 'no' }));

		const result = await signIn('wrong');

		expect(result).toMatchObject({ ok: false, error: { status: 401, layer: 'auth' } });
	});

	it('deletes the session and resolves with no value', async () => {
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		const result = await signOut();

		expect(callOf(0)).toMatchObject({ url: '/v1/session', method: 'DELETE' });
		expect(result).toEqual({ ok: true, value: undefined });
	});

	it('returns the current identity from /v1/me', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { user_id: 'bob', is_admin: false, auth_method: 'session', session_expires_at: null, csrf_token: null, permissions_model: 'owner' }));

		const result = await me();

		expect(callOf(0)).toMatchObject({ url: '/v1/me', method: 'GET' });
		expect(result).toMatchObject({ ok: true, value: { user_id: 'bob', is_admin: false } });
	});

	it('sends the csrf header on unsafe job actions', async () => {
		setCsrfToken('csrf-1');
		fetchMock.mockResolvedValueOnce(json(200, { id: 'job/1', state: 'cancelled' }));

		const result = await cancelJob('job/1');

		expect(callOf(0)).toMatchObject({ url: '/v1/jobs/job%2F1/cancel', method: 'POST', csrf: 'csrf-1' });
		expect(result).toMatchObject({ ok: true, value: { id: 'job/1' } });
	});

	it('lists own jobs with only the query keys that are set', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { items: [], next_cursor: null }));

		await myJobs({ state: 'running' });

		expect(callOf(0).url).toBe('/v1/jobs?state=running');
	});

	it('reads own events since a cursor', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { items: [] }));

		const result = await myEvents({ since: '2026-10-22T10:00:00Z', limit: 50 });

		expect(callOf(0).url).toBe('/v1/me/events?since=2026-10-22T10%3A00%3A00Z&limit=50');
		expect(result).toEqual({ ok: true, value: { items: [] } });
	});
});

describe('links api', () => {
	it('lists links with filter keys in snake case and drops empty keys', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { items: [], next_cursor: null, counts: { active: 0, used: 0, expired: 0, revoked: 0 } }));

		await listLinks({ status: 'active', createdBy: 'alice' }, null);

		expect(callOf(0).url).toBe('/v1/links?status=active&created_by=alice');
	});

	it('revokes a link by id and surfaces a conflict', async () => {
		fetchMock.mockResolvedValueOnce(json(409, { code: 'CONFLICT', message: 'not active' }));

		const result = await revokeLink(12);

		expect(callOf(0)).toMatchObject({ url: '/v1/links/12', method: 'DELETE' });
		expect(result).toMatchObject({ ok: false, error: { status: 409 } });
	});

	it('generates a link with the expiry in seconds', async () => {
		fetchMock.mockResolvedValueOnce(json(201, { id: 5, url: 'https://x/download/t', token: 't', expires: '2026-10-23T12:00:00Z' }));

		const result = await generateLink('/docs/a.txt', 3600);

		expect(callOf(0)).toMatchObject({ url: '/v1/links/generate', method: 'POST', body: { path: '/docs/a.txt', expiry_seconds: 3600 } });
		expect(result).toMatchObject({ ok: true, value: { id: 5, token: 't' } });
	});

	it('returns a failure without throwing when the network fails', async () => {
		fetchMock.mockRejectedValueOnce(new TypeError('offline'));

		const result = await revokeLink(1);

		expect(result).toMatchObject({ ok: false, error: { layer: 'network', retryable: true } });
	});
});

describe('erasure api', () => {
	it('lists split files with a health filter', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { items: [], next_cursor: null }));

		await listSplitFiles({ health: 'critical', prefix: '/team' }, 'next');

		expect(callOf(0).url).toBe('/v1/erasure/files?prefix=%2Fteam&health=critical&cursor=next');
	});

	it('encodes each path segment for a file detail', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { path: '/a b/c.bin', shards: [] }));

		const result = await getSplitFile('/a b/c.bin');

		expect(callOf(0).url).toBe('/v1/erasure/files/a%20b/c.bin');
		expect(result).toMatchObject({ ok: true, value: { path: '/a b/c.bin' } });
	});

	it('starts a repair job for a file', async () => {
		fetchMock.mockResolvedValueOnce(json(202, { job_id: 'job-9' }));

		const result = await repair('/docs/a.bin');

		expect(callOf(0)).toMatchObject({ url: '/v1/erasure/repair/docs/a.bin', method: 'POST' });
		expect(result).toEqual({ ok: true, value: { job_id: 'job-9' } });
	});

	it('builds the shard url for one piece', () => {
		expect(pieceUrl('/docs/a b.bin', 3)).toBe('/v1/shards/docs/a%20b.bin/3');
	});
});

describe('trash api', () => {
	it('lists trash with the cursor', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { items: [], next_cursor: null, total_bytes: 0 }));

		const result = await listTrash('abc');

		expect(callOf(0).url).toBe('/v1/trash?cursor=abc');
		expect(result).toMatchObject({ ok: true, value: { total_bytes: 0 } });
	});

	it('restores with defaults when no options are given', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { path: '/docs/a.txt' }));

		const result = await restore('t-1');

		expect(callOf(0)).toMatchObject({ url: '/v1/trash/t-1/restore', method: 'POST', body: { to: null, on_conflict: 'fail' } });
		expect(result).toEqual({ ok: true, value: { path: '/docs/a.txt' } });
	});

	it('restores to a chosen path with replace on conflict', async () => {
		fetchMock.mockResolvedValueOnce(json(200, { path: '/moved/a.txt' }));

		await restore('t-2', { to: '/moved/a.txt', onConflict: 'replace' });

		expect(callOf(0).body).toEqual({ to: '/moved/a.txt', on_conflict: 'replace' });
	});

	it('reports a conflict when the original path exists', async () => {
		fetchMock.mockResolvedValueOnce(json(409, { code: 'CONFLICT', message: 'exists' }));

		const result = await restore('t-3');

		expect(result).toMatchObject({ ok: false, error: { status: 409, code: 'CONFLICT' } });
	});

	it('deletes one item for good', async () => {
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		const result = await deleteTrashItem('t 4');

		expect(callOf(0)).toMatchObject({ url: '/v1/trash/t%204', method: 'DELETE' });
		expect(result).toEqual({ ok: true, value: undefined });
	});

	it('empties trash through a job', async () => {
		fetchMock.mockResolvedValueOnce(json(202, { job_id: 'job-3' }));

		const result = await emptyTrash();

		expect(callOf(0)).toMatchObject({ url: '/v1/trash', method: 'DELETE' });
		expect(result).toEqual({ ok: true, value: { job_id: 'job-3' } });
	});
});
