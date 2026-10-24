import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cache, forceRelease, invalidateCache, locks, rateLimits } from './runtime';
import {
	backends,
	cleanStaging,
	erasurePlan,
	erasureSummary,
	hygiene,
	instances,
	migrations,
	probeBackend,
	probeInstance,
	removeEndpoint,
	replication,
	retag,
	retryReplication,
	setEndpoint,
	staging,
	verifyAll
} from './topology';
import {
	authFailures,
	createUser,
	deleteUser,
	migratePositional,
	owners,
	reassignOwner,
	rotateKey,
	tls,
	updateUser,
	users
} from './users';

vi.mock('$lib/stores/session.svelte', () => ({ session: { onUnauthorized: vi.fn() } }));

const fetchMock = vi.fn<typeof fetch>();

function json(status: number, body: unknown): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

function sent(): { url: string; method: string | undefined; body: unknown } {
	const init = fetchMock.mock.calls[0]?.[1];
	const body = undefined === init?.body ? undefined : JSON.parse(String(init.body));
	return { url: String(fetchMock.mock.calls[0]?.[0]), method: init?.method, body };
}

const CASES: [string, () => Promise<unknown>, string, string, unknown, unknown][] = [
	['lists instances', () => instances(), 'GET', '/v1/admin/instances', undefined, { self: {}, items: [] }],
	[
		'sets an instance endpoint',
		() => setEndpoint('n1', 'https://n1.example', true),
		'PUT',
		'/v1/admin/instances/n1/endpoint',
		{ endpoint: 'https://n1.example', persist: true },
		{ instance_id: 'n1' }
	],
	[
		'removes an instance endpoint',
		() => removeEndpoint('n1'),
		'DELETE',
		'/v1/admin/instances/n1/endpoint',
		undefined,
		{ instance_id: 'n1' }
	],
	[
		'probes an instance',
		() => probeInstance('n1'),
		'POST',
		'/v1/admin/instances/n1/probe',
		undefined,
		{ reachable: true, rtt_ms: 4, status_code: 200, error: null }
	],
	['lists backends', () => backends(), 'GET', '/v1/admin/backends', undefined, { default: 'localfs', items: [] }],
	[
		'probes a backend',
		() => probeBackend('s3'),
		'POST',
		'/v1/admin/backends/s3/probe',
		undefined,
		{ ok: true, write_ms: 1, read_ms: 1, delete_ms: 1, error: null }
	],
	['reads erasure summary', () => erasureSummary(), 'GET', '/v1/admin/erasure', undefined, { enabled: true }],
	[
		'plans erasure placement',
		() => erasurePlan({ data_shards: 4, parity_shards: 2, instances: ['a', 'b'] }),
		'POST',
		'/v1/admin/erasure/plan',
		{ data_shards: 4, parity_shards: 2, instances: ['a', 'b'] },
		{ total_shards: 6 }
	],
	[
		'starts erasure verify',
		() => verifyAll('/'),
		'POST',
		'/v1/admin/erasure/verify-all',
		{ prefix: '/' },
		{ job_id: 'j1' }
	],
	['reads replication', () => replication(), 'GET', '/v1/admin/replication', undefined, { enabled: false }],
	[
		'retries all replication failures',
		() => retryReplication(null),
		'POST',
		'/v1/admin/replication/retry',
		{ paths: null },
		{ job_id: 'j2' }
	],
	['lists staging', () => staging(), 'GET', '/v1/admin/staging', undefined, { items: [], total_bytes: 0 }],
	[
		'cleans old staging',
		() => cleanStaging(86400),
		'DELETE',
		'/v1/admin/staging?older_than_s=86400',
		undefined,
		{ job_id: 'j3' }
	],
	['reads hygiene', () => hygiene(), 'GET', '/v1/admin/hygiene', undefined, { files_without_owner: 0 }],
	['reads migrations', () => migrations(), 'GET', '/v1/admin/migrations', undefined, { driver: 'none' }],
	['starts retag', () => retag('/a'), 'POST', '/v1/admin/retag', { prefix: '/a' }, { job_id: 'j4' }],
	[
		'lists locks for a path',
		() => locks('/a/b'),
		'GET',
		'/v1/admin/locks?path=%2Fa%2Fb',
		undefined,
		{ type: 'local', held: [] }
	],
	[
		'force releases a lock with an encoded key',
		() => forceRelease('files/a b', 'stuck'),
		'DELETE',
		'/v1/admin/locks/files%2Fa%20b',
		{ reason: 'stuck' },
		{ released: true }
	],
	['reads cache stats', () => cache(), 'GET', '/v1/admin/cache', undefined, { enabled: true }],
	[
		'invalidates a cache prefix',
		() => invalidateCache({ prefix: '/a' }),
		'POST',
		'/v1/admin/cache/invalidate',
		{ prefix: '/a' },
		{ invalidated: 3 }
	],
	[
		'invalidates the whole cache',
		() => invalidateCache({ all: true }),
		'POST',
		'/v1/admin/cache/invalidate',
		{ all: true },
		{ invalidated: 9 }
	],
	['reads rate limits', () => rateLimits(), 'GET', '/v1/admin/ratelimits', undefined, { limits: [] }],
	['lists users', () => users(), 'GET', '/v1/admin/users', undefined, { items: [], positional_count: 0 }],
	[
		'creates an admin user',
		() => createUser('alice', true),
		'POST',
		'/v1/admin/users',
		{ user_id: 'alice', is_admin: true },
		{ user_id: 'alice', api_key: 'k' }
	],
	[
		'rotates a user key',
		() => rotateKey('alice'),
		'POST',
		'/v1/admin/users/alice/rotate',
		undefined,
		{ user_id: 'alice', api_key: 'k2' }
	],
	[
		'updates a user',
		() => updateUser('alice', { is_admin: false }),
		'PATCH',
		'/v1/admin/users/alice',
		{ is_admin: false },
		{ user_id: 'alice', is_admin: false }
	],
	[
		'migrates positional users',
		() => migratePositional([{ index: 0, user_id: 'alice' }]),
		'POST',
		'/v1/admin/users/migrate-positional',
		{ mapping: [{ index: 0, user_id: 'alice' }], reassign_owner: true },
		{ job_id: 'j5' }
	],
	[
		'lists owners from a cursor',
		() => owners('c1'),
		'GET',
		'/v1/admin/owners?cursor=c1',
		undefined,
		{ items: [], next_cursor: null }
	],
	[
		'lists owners from the first page',
		() => owners(null),
		'GET',
		'/v1/admin/owners',
		undefined,
		{ items: [], next_cursor: null }
	],
	[
		'reassigns owner files',
		() => reassignOwner('api-user-0', 'alice', '/'),
		'POST',
		'/v1/admin/owners/reassign',
		{ from: 'api-user-0', to: 'alice', prefix: '/' },
		{ job_id: 'j6' }
	],
	[
		'lists auth failures since a date',
		() => authFailures('2026-10-22'),
		'GET',
		'/v1/admin/auth/failures?since=2026-10-22',
		undefined,
		{ total_24h: 0, items: [] }
	],
	['reads TLS state', () => tls(), 'GET', '/v1/admin/tls', undefined, { protocol_configured: 'https' }]
];

beforeEach(() => {
	fetchMock.mockReset();
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe('admin topology, runtime and users endpoints', () => {
	it.each(CASES)('%s', async (_name, call, method, url, body, data) => {
		fetchMock.mockResolvedValueOnce(json(200, data));

		const result = await call();

		expect(result).toEqual({ ok: true, value: data });
		expect(sent()).toEqual({ url, method, body });
	});

	it('deletes a user and resolves with no value on 204', async () => {
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

		const result = await deleteUser('alice');

		expect(result).toEqual({ ok: true, value: undefined });
		expect(sent()).toEqual({ url: '/v1/admin/users/alice', method: 'DELETE', body: undefined });
	});

	it('returns a permission failure as a result instead of throwing', async () => {
		fetchMock.mockResolvedValueOnce(json(403, { code: 'ADMIN_REQUIRED', message: 'admin only' }));

		const result = await users();

		expect(result).toEqual({ ok: false, error: expect.objectContaining({ kind: 'http', status: 403 }) });
	});
});
