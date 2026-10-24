import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { nodeScope } from '$lib/stores/node-scope.svelte';
import { fileUrl, nodePath, request } from './http';
import { isForwardablePath, isLocalPath, scopedPath } from './scope';

vi.mock('$lib/stores/session.svelte', () => ({ session: { onUnauthorized: vi.fn(() => Promise.resolve(false)) } }));

const fetchMock = vi.fn<typeof fetch>();

function fetchedUrl(call: number): string {
	return String(fetchMock.mock.calls[call]?.[0]);
}

describe('scopedPath', () => {
	it('keeps every path unchanged while this server is selected', () => {
		expect(scopedPath('/v1/admin/health', null)).toBe('/v1/admin/health');
		expect(scopedPath('/v1/files/a.txt', '')).toBe('/v1/files/a.txt');
	});

	it('sends /v1 paths through the selected node', () => {
		expect(scopedPath('/v1/admin/health', 'node-2')).toBe('/v1/nodes/node-2/v1/admin/health');
		expect(scopedPath('/v1/admin/config', 'node-2')).toBe('/v1/nodes/node-2/v1/admin/config');
		expect(scopedPath('/v1/files/docs/a%20b.txt?download=true', 'node-2')).toBe(
			'/v1/nodes/node-2/v1/files/docs/a%20b.txt?download=true'
		);
		expect(scopedPath('/v1/admin/events?types=job.*', 'node-2')).toBe('/v1/nodes/node-2/v1/admin/events?types=job.*');
		expect(scopedPath('/v1/files/ws/a.txt', 'node-2')).toBe('/v1/nodes/node-2/v1/files/ws/a.txt');
	});

	it('sends health paths through the selected node', () => {
		expect(scopedPath('/health', 'node-2')).toBe('/v1/nodes/node-2/health');
		expect(scopedPath('/health/ready', 'node-2')).toBe('/v1/nodes/node-2/health/ready');
	});

	it('keeps session, me and nodes paths on this server', () => {
		for (const path of [
			'/v1/session',
			'/v1/session/refresh',
			'/v1/me',
			'/v1/me?x=1',
			'/v1/me/events',
			'/v1/me/events?limit=5',
			'/v1/nodes',
			'/v1/nodes/node-3/v1/admin/health'
		]) {
			expect(scopedPath(path, 'node-2')).toBe(path);
		}
	});

	it('does not treat paths that only start like /v1/me as local', () => {
		expect(isLocalPath('/v1/metrics')).toBe(false);
		expect(scopedPath('/v1/metrics', 'node-2')).toBe('/v1/nodes/node-2/v1/metrics');
	});

	it('leaves paths outside /v1/ and /health alone', () => {
		expect(isForwardablePath('/ui/favicon.svg')).toBe(false);
		expect(scopedPath('/ui/favicon.svg', 'node-2')).toBe('/ui/favicon.svg');
		expect(scopedPath('/v1', 'node-2')).toBe('/v1');
	});

	it('encodes the node id and stays the same when applied twice', () => {
		const once = scopedPath('/v1/admin/health', 'node a/b');
		expect(once).toBe('/v1/nodes/node%20a%2Fb/v1/admin/health');
		expect(scopedPath(once, 'node a/b')).toBe(once);
	});
});

describe('request scoping', () => {
	beforeEach(() => {
		fetchMock.mockReset();
		fetchMock.mockImplementation(() => Promise.resolve(new Response('{}', { status: 200 })));
		vi.stubGlobal('fetch', fetchMock);
	});

	afterEach(() => {
		nodeScope.select(null);
		vi.unstubAllGlobals();
	});

	it('sends requests to this server when no node is selected', async () => {
		await request('/v1/admin/health');
		expect(fetchedUrl(0)).toBe('/v1/admin/health');
	});

	it('rewrites requests and query strings to the selected node', async () => {
		nodeScope.select('node-2');
		await request('/v1/admin/health', { query: { deep: true } });
		expect(fetchedUrl(0)).toBe('/v1/nodes/node-2/v1/admin/health?deep=true');
	});

	it('keeps me, session and nodes requests local while a node is selected', async () => {
		nodeScope.select('node-2');
		await request('/v1/me');
		await request('/v1/session', { method: 'DELETE' });
		await request('/v1/nodes');
		expect([fetchedUrl(0), fetchedUrl(1), fetchedUrl(2)]).toEqual(['/v1/me', '/v1/session', '/v1/nodes']);
	});

	it('honours an explicit node option over the selected node', async () => {
		nodeScope.select('node-2');
		await request('/v1/files/prefs.json', { node: null });
		await request('/v1/admin/health', { node: 'node-3' });
		expect(fetchedUrl(0)).toBe('/v1/files/prefs.json');
		expect(fetchedUrl(1)).toBe('/v1/nodes/node-3/v1/admin/health');
	});

	it('scopes file links and built paths', () => {
		nodeScope.select('node-2');
		expect(fileUrl('/docs/a.txt', { download: true })).toBe('/v1/nodes/node-2/v1/files/docs/a.txt?download=true');
		expect(nodePath('/v1/admin/logs/stream')).toBe('/v1/nodes/node-2/v1/admin/logs/stream');
		expect(nodePath('/v1/admin/logs/stream', null)).toBe('/v1/admin/logs/stream');
	});
});
