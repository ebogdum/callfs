import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ApiFailure } from '../errors';
import { request, type ApiResponse } from '../http';
import { API_ROUTES } from '../routes';
import type { Job, Result, SeriesResult } from '../types';
import { deadTasks, dropDeadTask, retryDeadTask } from './deadtasks';
import { cancel, job, jobs, retryFailed, runWorker, workers } from './jobs';
import { audit, eventHistory, logs, metrics, series, transfers } from './telemetry';

vi.mock('../http', () => ({ request: vi.fn() }));

const requestMock = vi.mocked(request);

const networkFailure: Result<ApiResponse<unknown>, ApiFailure> = { ok: false, error: { kind: 'network' } };

function success<T>(data: T): Result<ApiResponse<T>, ApiFailure> {
	return { ok: true, value: { data, status: 200, headers: new Headers(), requestId: null, etag: null } };
}

const seriesResult: SeriesResult = { name: 'http_requests_total', measure: 'rate', unit: 'requests', step_s: 60, series: [], sampled_since: null };

const jobRecord: Job = {
	id: 'job-1',
	kind: 'usage_scan',
	state: 'running',
	phase: 'scan',
	progress: { done: 1, total: 2, bytes_done: 10, bytes_total: 20, per_second: 5 },
	created_by: 'root',
	created_at: '2026-10-23T00:00:00Z',
	started_at: null,
	finished_at: null,
	error: null,
	failed_items: [],
	failed_count: 0,
	result: {}
};

beforeEach(() => {
	requestMock.mockReset();
});

describe('admin telemetry api', () => {
	it('joins metric names into one query value', async () => {
		requestMock.mockResolvedValueOnce(success({ generated_at: 'now', families: [] }));

		const result = await metrics(['a_total', 'b_seconds']);

		expect(result).toEqual({ ok: true, value: { generated_at: 'now', families: [] } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/metrics', { query: { names: 'a_total,b_seconds' } });
	});

	it('omits the names query when no names are given', async () => {
		requestMock.mockResolvedValueOnce(success({ generated_at: 'now', families: [] }));

		await metrics();

		expect(requestMock).toHaveBeenCalledWith('/v1/admin/metrics', { query: { names: undefined } });
	});

	it('builds series paths with repeated filters and a by list', async () => {
		requestMock.mockResolvedValueOnce(success(seriesResult));

		const result = await series({
			name: 'http_requests_total',
			measure: 'rate',
			by: ['status'],
			filter: ['route=download', 'status=200'],
			from: 100,
			to: 200,
			stepS: 60
		});

		expect(result).toEqual({ ok: true, value: seriesResult });
		expect(requestMock).toHaveBeenCalledWith(
			'/v1/admin/metrics/series?name=http_requests_total&measure=rate&by=status&filter=route%3Ddownload&filter=status%3D200&from=100&to=200&step_s=60'
		);
	});

	it('passes a failed request through unchanged', async () => {
		requestMock.mockResolvedValueOnce(networkFailure);

		const result = await logs({});

		expect(result).toEqual({ ok: false, error: { kind: 'network' } });
	});

	it('sends log filters as query values', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [], next_cursor: null }));

		await logs({ level: 'warn', q: 'timeout', limit: 50 });

		expect(requestMock).toHaveBeenCalledWith('/v1/admin/logs', { query: { level: 'warn', q: 'timeout', limit: 50 } });
	});

	it('sends audit filters and the cursor', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [], next_cursor: 'c2' }));

		const result = await audit({ op: 'delete', status: '>=400', cursor: 'c1' });

		expect(result).toEqual({ ok: true, value: { items: [], next_cursor: 'c2' } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/audit', { query: { op: 'delete', status: '>=400', cursor: 'c1' } });
	});

	it('joins event types for history', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [] }));

		await eventHistory({ types: ['config.changed', 'job.failed'] });

		expect(requestMock).toHaveBeenCalledWith('/v1/admin/events/history', { query: { types: 'config.changed,job.failed', since: undefined } });
	});

	it('lists active transfers', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [] }));

		const result = await transfers();

		expect(result).toEqual({ ok: true, value: { items: [] } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/transfers');
	});
});

describe('admin jobs api', () => {
	it('filters the job list by state and kind', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [jobRecord] }));

		const result = await jobs({ state: ['running', 'failed'], kind: ['usage_scan'] });

		expect(result).toEqual({ ok: true, value: { items: [jobRecord] } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/jobs', { query: { state: 'running,failed', kind: 'usage_scan', created_by: undefined } });
	});

	it('encodes the job id in the path', async () => {
		requestMock.mockResolvedValueOnce(success(jobRecord));

		await job('job 1/x');

		expect(requestMock).toHaveBeenCalledWith('/v1/admin/jobs/job%201%2Fx');
	});

	it('cancels a job with POST', async () => {
		requestMock.mockResolvedValueOnce(success(jobRecord));

		const result = await cancel('job-1');

		expect(result).toEqual({ ok: true, value: jobRecord });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/jobs/job-1/cancel', { method: 'POST' });
	});

	it('retries failed items with POST', async () => {
		requestMock.mockResolvedValueOnce(success(jobRecord));

		await retryFailed('job-1');

		expect(requestMock).toHaveBeenCalledWith('/v1/admin/jobs/job-1/retry-failed', { method: 'POST' });
	});

	it('lists background workers', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [] }));

		const result = await workers();

		expect(result).toEqual({ ok: true, value: { items: [] } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/workers');
	});

	it('starts a worker and returns its job id', async () => {
		requestMock.mockResolvedValueOnce(success({ job_id: 'job-9' }));

		const result = await runWorker('link_cleanup');

		expect(result).toEqual({ ok: true, value: { job_id: 'job-9' } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/workers/link_cleanup/run', { method: 'POST' });
	});
});

describe('admin dead tasks api', () => {
	it('lists dead tasks before a sequence', async () => {
		requestMock.mockResolvedValueOnce(success({ items: [], next_before: null }));

		const result = await deadTasks(40);

		expect(result).toEqual({ ok: true, value: { items: [], next_before: null } });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/tasks/dead', { query: { before: 40 } });
	});

	it('retries and drops by id', async () => {
		requestMock.mockResolvedValue(success(undefined));

		await retryDeadTask(7);
		await dropDeadTask(8);

		expect(requestMock).toHaveBeenCalledWith('/v1/admin/tasks/dead/7/retry', { method: 'POST' });
		expect(requestMock).toHaveBeenCalledWith('/v1/admin/tasks/dead/8', { method: 'DELETE' });
	});
});

describe('route registry', () => {
	it('gives every route a unique id', () => {
		const ids = API_ROUTES.map((route) => route.id);

		expect(new Set(ids).size).toBe(ids.length);
	});

	it('lists only screens S01 to S40', () => {
		const screenPattern = /^S(0[1-9]|[1-3]\d|40)$/;

		expect(API_ROUTES.every((route) => route.screens.every((screen) => screenPattern.test(screen)))).toBe(true);
	});

	it('keeps admin routes under /v1/admin and only those', () => {
		expect(API_ROUTES.every((route) => route.admin === route.path.startsWith('/v1/admin/'))).toBe(true);
	});

	it('gives internal routes no screens and a reason', () => {
		const internal = API_ROUTES.filter((route) => true === route.internal);

		expect(internal).toHaveLength(3);
		expect(internal.every((route) => 0 === route.screens.length && 0 < (route.reason ?? '').length)).toBe(true);
	});

	it('uses dotted lowercase command ids', () => {
		const commandPattern = /^[a-z-]+\.[a-z-]+$/;

		expect(API_ROUTES.flatMap((route) => route.commands).every((command) => commandPattern.test(command))).toBe(true);
	});
});
