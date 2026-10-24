import { beforeEach, describe, expect, it, vi } from 'vitest';
import { httpFailure, type ApiFailure } from '../errors';
import type { ApiResponse, RequestOptions } from '../http';
import type { Result } from '../types';
import { status, restart } from './status';
import { health } from './health';
import { findings, mute, unmute } from './findings';
import { exportConfig, getConfig, patchRuntime, save, schema, validate } from './config';
import {
	cluster,
	joinConfig,
	removeNode,
	snapshot,
	snapshots,
	stateStats,
	transferLeadership
} from './cluster';

const { requestMock } = vi.hoisted(() => ({
	requestMock: vi.fn<(path: string, opts?: RequestOptions) => Promise<Result<ApiResponse<unknown>, ApiFailure>>>()
}));

vi.mock('../http', () => ({ request: requestMock }));

function success(data: unknown): Result<ApiResponse<unknown>, ApiFailure> {
	return { ok: true, value: { data, status: 200, headers: new Headers(), requestId: 'req-1', etag: null } };
}

function networkFailure(): Result<ApiResponse<unknown>, ApiFailure> {
	return { ok: false, error: { kind: 'network' } };
}

function conflictFailure(): Result<ApiResponse<unknown>, ApiFailure> {
	return { ok: false, error: httpFailure(409, JSON.stringify({ code: 'CONFLICT' }), null, 'req-9') };
}

function lastCall(): { path: string; opts: RequestOptions | undefined } {
	const call = requestMock.mock.calls.at(-1);
	return { path: call?.[0] ?? '', opts: call?.[1] };
}

const NODE = { node_id: 'node-b', api_endpoint: 'https://10.0.0.2:8080', cluster_addr: '10.0.0.2:6222' };

beforeEach(() => {
	requestMock.mockReset();
});

describe('admin status', () => {
	it('reads the server status from GET /v1/admin/status', async () => {
		const body = { version: '1.2.3', node_id: 'node-a' };
		requestMock.mockResolvedValueOnce(success(body));
		const result = await status();
		expect(lastCall()).toEqual({ path: '/v1/admin/status', opts: undefined });
		expect(result).toEqual({ ok: true, value: body });
	});

	it('maps a network failure to a retryable network UiError', async () => {
		requestMock.mockResolvedValueOnce(networkFailure());
		const result = await status();
		expect(result.ok).toBe(false);
		if (result.ok) {
			return;
		}
		expect(result.error).toMatchObject({ layer: 'network', retryable: true });
	});

	it('posts the restart reason to POST /v1/admin/restart', async () => {
		requestMock.mockResolvedValueOnce(success({ restarting: true }));
		const result = await restart('config changed');
		expect(lastCall()).toEqual({
			path: '/v1/admin/restart',
			opts: { method: 'POST', body: { reason: 'config changed' } }
		});
		expect(result).toEqual({ ok: true, value: { restarting: true } });
	});
});

describe('admin health', () => {
	it('reads GET /v1/admin/health without a fresh flag by default', async () => {
		requestMock.mockResolvedValueOnce(success({ status: 'ok', checked_at: 'now', components: [] }));
		const result = await health();
		expect(lastCall()).toEqual({ path: '/v1/admin/health', opts: { query: { fresh: undefined } } });
		expect(result.ok).toBe(true);
	});

	it('sends fresh=true when a cache bypass is requested', async () => {
		requestMock.mockResolvedValueOnce(success({ status: 'warn', checked_at: 'now', components: [] }));
		await health(true);
		expect(lastCall()).toEqual({ path: '/v1/admin/health', opts: { query: { fresh: true } } });
	});
});

describe('admin findings', () => {
	it('sends no filters by default', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [], counts: { critical: 0, warning: 0, info: 0 } }));
		await findings();
		expect(lastCall()).toEqual({
			path: '/v1/admin/findings',
			opts: { query: { area: undefined, include_muted: undefined } }
		});
	});

	it('passes the area and include_muted filters through', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [], counts: { critical: 0, warning: 0, info: 0 } }));
		await findings('security', true);
		expect(lastCall()).toEqual({
			path: '/v1/admin/findings',
			opts: { query: { area: 'security', include_muted: true } }
		});
	});

	it('posts duration and reason to the encoded mute path', async () => {
		const finding = { id: 'tls.plain_http', severity: 'critical' };
		requestMock.mockResolvedValueOnce(success(finding));
		const result = await mute('tls/plain http', 3600, 'maintenance');
		expect(lastCall()).toEqual({
			path: '/v1/admin/findings/tls%2Fplain%20http/mute',
			opts: { method: 'POST', body: { duration_s: 3600, reason: 'maintenance' } }
		});
		expect(result).toEqual({ ok: true, value: finding });
	});

	it('deletes the mute on the encoded path', async () => {
		requestMock.mockResolvedValueOnce(success({ id: 'tls.plain_http' }));
		await unmute('tls.plain_http');
		expect(lastCall()).toEqual({
			path: '/v1/admin/findings/tls.plain_http/mute',
			opts: { method: 'DELETE' }
		});
	});
});

describe('admin config', () => {
	it('reads GET /v1/admin/config', async () => {
		requestMock.mockResolvedValueOnce(success({ config_path: '/etc/callfs.yaml', keys: [] }));
		const result = await getConfig();
		expect(lastCall()).toEqual({ path: '/v1/admin/config', opts: undefined });
		expect(result.ok).toBe(true);
	});

	it('reads GET /v1/admin/config/schema', async () => {
		requestMock.mockResolvedValueOnce(success({ keys: [] }));
		const result = await schema();
		expect(lastCall()).toEqual({ path: '/v1/admin/config/schema', opts: undefined });
		expect(result).toEqual({ ok: true, value: { keys: [] } });
	});

	it('posts the changes to the validate endpoint', async () => {
		const changes = { 'server.read_timeout': '2m' };
		requestMock.mockResolvedValueOnce(
			success({ valid: true, errors: [], warnings: [], restart_required: [], applies_now: [] })
		);
		await validate(changes);
		expect(lastCall()).toEqual({
			path: '/v1/admin/config/validate',
			opts: { method: 'POST', body: { changes } }
		});
	});

	it('saves changes with the expected revision through PUT /v1/admin/config', async () => {
		const changes = { 'log.level': 'debug' };
		requestMock.mockResolvedValueOnce(
			success({ written: true, backup_path: 'config.yaml.bak.1', revision: 'sha256:b', restart_required: [], applied_now: [] })
		);
		const result = await save(changes, 'sha256:a');
		expect(lastCall()).toEqual({
			path: '/v1/admin/config',
			opts: { method: 'PUT', body: { changes, expected_revision: 'sha256:a' } }
		});
		expect(result.ok).toBe(true);
	});

	it('maps a 409 revision conflict to a UiError with status 409', async () => {
		requestMock.mockResolvedValueOnce(conflictFailure());
		const result = await save({}, 'sha256:stale');
		expect(result.ok).toBe(false);
		if (result.ok) {
			return;
		}
		expect(result.error.status).toBe(409);
	});

	it('patches runtime keys with persist enabled', async () => {
		requestMock.mockResolvedValueOnce(success({ applied: ['log.level'], persisted: true, revision: 'sha256:c' }));
		await patchRuntime({ 'log.level': 'info' });
		expect(lastCall()).toEqual({
			path: '/v1/admin/config/runtime',
			opts: { method: 'PATCH', body: { 'log.level': 'info', persist: true } }
		});
	});

	it('requests the export as raw text with format and effective flags', async () => {
		requestMock.mockResolvedValueOnce(success('server:\n  port: 8080\n'));
		const result = await exportConfig('yaml', true);
		expect(lastCall()).toEqual({
			path: '/v1/admin/config/export',
			opts: { query: { format: 'yaml', effective: true }, raw: true }
		});
		expect(result).toEqual({ ok: true, value: 'server:\n  port: 8080\n' });
	});
});

describe('admin cluster', () => {
	it('reads GET /v1/admin/cluster', async () => {
		requestMock.mockResolvedValueOnce(success({ mode: 'single', members: [] }));
		const result = await cluster();
		expect(lastCall()).toEqual({ path: '/v1/admin/cluster', opts: undefined });
		expect(result.ok).toBe(true);
	});

	it('reads the join config with the new member in the query', async () => {
		requestMock.mockResolvedValueOnce(success({ yaml: 'nats:\n  enabled: true\n', steps: ['Start the node.'] }));
		const result = await joinConfig(NODE);
		expect(lastCall()).toEqual({
			path: '/v1/admin/cluster/join-config',
			opts: { query: { node_id: 'node-b', api_endpoint: 'https://10.0.0.2:8080', cluster_addr: '10.0.0.2:6222' } }
		});
		expect(result).toEqual({ ok: true, value: { yaml: 'nats:\n  enabled: true\n', steps: ['Start the node.'] } });
	});

	it('deletes the encoded node', async () => {
		requestMock.mockResolvedValueOnce(success({ status: 'removed', remaining: 2 }));
		const result = await removeNode('node b');
		expect(lastCall()).toEqual({
			path: '/v1/admin/cluster/nodes/node%20b',
			opts: { method: 'DELETE' }
		});
		expect(result).toEqual({ ok: true, value: { status: 'removed', remaining: 2 } });
	});

	it('maps a conflict on removal to a UiError with status 409', async () => {
		requestMock.mockResolvedValueOnce(conflictFailure());
		const result = await removeNode('node-b');
		expect(result.ok).toBe(false);
		if (result.ok) {
			return;
		}
		expect(result.error).toMatchObject({ status: 409 });
	});

	it('posts the target node to leadership transfer', async () => {
		requestMock.mockResolvedValueOnce(success({ status: 'transferred', leader_id: 'node-b' }));
		await transferLeadership('node-b');
		expect(lastCall()).toEqual({
			path: '/v1/admin/cluster/leadership-transfer',
			opts: { method: 'POST', body: { node_id: 'node-b' } }
		});
	});

	it('posts a snapshot request', async () => {
		requestMock.mockResolvedValueOnce(success({ id: 'snap-10', seq: 10, bytes: 512, took_ms: 4 }));
		await snapshot();
		expect(lastCall()).toEqual({ path: '/v1/admin/cluster/snapshot', opts: { method: 'POST' } });
	});

	it('reads the snapshot list and the state stats', async () => {
		requestMock.mockResolvedValueOnce(success({ retain: 3, items: [] }));
		await snapshots();
		expect(lastCall()).toEqual({ path: '/v1/admin/cluster/snapshots', opts: undefined });
		requestMock.mockResolvedValueOnce(success({ files: 1, directories: 1 }));
		await stateStats();
		expect(lastCall()).toEqual({ path: '/v1/admin/cluster/state-stats', opts: undefined });
	});
});
