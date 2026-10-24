import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cancel as cancelAdminJob, jobs, job as adminJob, retryFailed as retryAdminJob, runWorker, workers } from '$lib/api/admin/jobs';
import { deadTasks, dropDeadTask, retryDeadTask } from '$lib/api/admin/deadtasks';
import { toUiError, type ApiFailure } from '$lib/api/errors';
import { cancelJob, job as ownJob, myJobs, retryJobFailed } from '$lib/api/session';
import type { DeadTask, Job, Worker } from '$lib/api/types';
import { goto } from '$app/navigation';
import { t } from '$lib/copy/t';
import { invalidateAll } from '$lib/data/cache';
import { formatNumber } from '$lib/format/number';
import JobDetailPlanet from './JobDetailPlanet.svelte';
import JobsPlanet from './JobsPlanet.svelte';

const sessionState = vi.hoisted(() => ({ isAdmin: true }));
const pageState = vi.hoisted(() => ({ url: new URL('http://127.0.0.1:4173/activity/jobs') }));

vi.mock('$lib/api/admin/jobs', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/admin/jobs')>()),
	jobs: vi.fn(),
	job: vi.fn(),
	cancel: vi.fn(),
	retryFailed: vi.fn(),
	workers: vi.fn(),
	runWorker: vi.fn()
}));

vi.mock('$lib/api/admin/deadtasks', () => ({
	deadTasks: vi.fn(),
	retryDeadTask: vi.fn(),
	dropDeadTask: vi.fn()
}));

vi.mock('$lib/api/session', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/session')>()),
	myJobs: vi.fn(),
	job: vi.fn(),
	cancelJob: vi.fn(),
	retryJobFailed: vi.fn()
}));

vi.mock('$lib/stores/session.svelte', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/stores/session.svelte')>()),
	session: sessionState
}));

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

vi.mock('$app/state', () => ({ page: pageState }));

const job = (overrides: Partial<Job> = {}): Job => ({
	id: 'job-1',
	kind: 'delete_recursive',
	state: 'running',
	phase: 'delete',
	progress: { done: 120, total: 1204, bytes_done: 0, bytes_total: null, per_second: 0 },
	created_by: 'alice',
	created_at: '2026-10-20T10:00:00Z',
	started_at: '2026-10-20T10:00:01Z',
	finished_at: null,
	error: null,
	failed_items: [],
	failed_count: 0,
	result: { path: '/team/old' },
	...overrides
});

const worker = (overrides: Partial<Worker> = {}): Worker => ({
	name: 'link_cleanup',
	interval_s: 300,
	running: false,
	last_run_at: '2026-10-23T09:55:00Z',
	last_duration_ms: 42,
	last_result: {},
	last_error: null,
	next_run_at: '2026-10-23T10:00:00Z',
	...overrides
});

const deadTask = (overrides: Partial<DeadTask> = {}): DeadTask => ({
	id: 12,
	pool: 'replicate',
	reason: 'max deliveries exceeded',
	failed_at: '2026-10-23T09:00:00Z',
	summary: '/team/report.pdf',
	...overrides
});

const httpFailure: ApiFailure = {
	kind: 'http',
	status: 404,
	code: 'NOT_FOUND',
	message: null,
	details: null,
	requestId: null,
	retryAfterS: null
};

beforeEach(() => {
	vi.resetAllMocks();
	sessionState.isAdmin = true;
	pageState.url = new URL('http://127.0.0.1:4173/activity/jobs');
	vi.mocked(goto).mockResolvedValue(undefined);
	invalidateAll();
});

afterEach(() => {
	cleanup();
});

describe('JobsPlanet', () => {
	it('loads the running jobs from the admin feed and renders each one', async () => {
		vi.mocked(jobs).mockResolvedValue({ ok: true, value: { items: [job()] } });
		render(JobsPlanet);
		await waitFor(() => expect(screen.getByRole('button', { name: t('support.jobs.open') })).toBeTruthy());
		expect(jobs).toHaveBeenCalledWith();
	});

	it('shows the empty copy when the selected group has no jobs', async () => {
		vi.mocked(jobs).mockResolvedValue({ ok: true, value: { items: [] } });
		render(JobsPlanet);
		await waitFor(() => expect(screen.getByText(t('jobs.empty.running.title'))).toBeTruthy());
	});

	it('shows the error state with the mapped title when the feed cannot be loaded', async () => {
		const failure: ApiFailure = { kind: 'network' };
		vi.mocked(jobs).mockResolvedValue({ ok: false, error: failure });
		render(JobsPlanet);
		const title = toUiError(failure, { action: 'jobs.action.load' }).title;
		await waitFor(() => expect(screen.getByText(title)).toBeTruthy());
	});

	it('lists the background workers for admins and starts one with Run now', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity/jobs?tab=background');
		vi.mocked(workers).mockResolvedValue({ ok: true, value: { items: [worker()] } });
		vi.mocked(deadTasks).mockResolvedValue({ ok: true, value: { items: [], next_before: null } });
		vi.mocked(runWorker).mockResolvedValue({ ok: true, value: { job_id: 'job-9' } });
		render(JobsPlanet);
		const run = await waitFor(() => screen.getByRole('button', { name: t('jobs.workers.run') }));
		await fireEvent.click(run);
		await waitFor(() => expect(runWorker).toHaveBeenCalledWith('link_cleanup'));
	});

	it('lists failed background tasks and retries one', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity/jobs?tab=background');
		vi.mocked(workers).mockResolvedValue({ ok: true, value: { items: [] } });
		vi.mocked(deadTasks).mockResolvedValue({ ok: true, value: { items: [deadTask()], next_before: null } });
		vi.mocked(retryDeadTask).mockResolvedValue({ ok: true, value: undefined });
		render(JobsPlanet);
		await waitFor(() => expect(screen.getAllByText('/team/report.pdf').length).toBeGreaterThan(0));
		await fireEvent.click(screen.getByRole('button', { name: t('jobs.dead.retry') }));
		await waitFor(() => expect(retryDeadTask).toHaveBeenCalledWith(12));
	});

	it('asks before dropping a failed background task', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity/jobs?tab=background');
		vi.mocked(workers).mockResolvedValue({ ok: true, value: { items: [] } });
		vi.mocked(deadTasks).mockResolvedValue({ ok: true, value: { items: [deadTask()], next_before: null } });
		vi.mocked(dropDeadTask).mockResolvedValue({ ok: true, value: undefined });
		render(JobsPlanet);
		await fireEvent.click(await waitFor(() => screen.getByRole('button', { name: t('jobs.dead.drop') })));
		expect(dropDeadTask).not.toHaveBeenCalled();
		await fireEvent.click(await waitFor(() => screen.getByRole('button', { name: t('jobs.dead.drop.confirm') })));
		await waitFor(() => expect(dropDeadTask).toHaveBeenCalledWith(12));
	});

	it('shows the empty copy when no background task failed', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity/jobs?tab=background');
		vi.mocked(workers).mockResolvedValue({ ok: true, value: { items: [] } });
		vi.mocked(deadTasks).mockResolvedValue({ ok: true, value: { items: [], next_before: null } });
		render(JobsPlanet);
		await waitFor(() => expect(screen.getByText(t('jobs.dead.empty.title'))).toBeTruthy());
	});

	it('shows the error state when failed background tasks cannot be loaded', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/activity/jobs?tab=background');
		const failure: ApiFailure = { kind: 'network' };
		vi.mocked(workers).mockResolvedValue({ ok: true, value: { items: [] } });
		vi.mocked(deadTasks).mockResolvedValue({ ok: false, error: failure });
		render(JobsPlanet);
		const title = toUiError(failure, { action: 'jobs.action.dead' }).title;
		await waitFor(() => expect(screen.getAllByText(title).length).toBeGreaterThan(0));
	});

	it('hides the background group from members and loads only their own jobs', async () => {
		sessionState.isAdmin = false;
		vi.mocked(myJobs).mockResolvedValue({ ok: true, value: { items: [], next_cursor: null } });
		render(JobsPlanet);
		await waitFor(() => expect(screen.getByText(t('jobs.empty.running.title'))).toBeTruthy());
		expect(myJobs).toHaveBeenCalledWith();
		expect(jobs).not.toHaveBeenCalled();
		expect(screen.queryByText(t('jobs.tabs.background'))).toBeNull();
	});

	it('cancels a job only after the confirmation dialog is accepted', async () => {
		vi.mocked(jobs).mockResolvedValue({ ok: true, value: { items: [job()] } });
		vi.mocked(cancelAdminJob).mockResolvedValue({ ok: true, value: job({ state: 'cancelling' }) });
		render(JobsPlanet);
		await fireEvent.click(await waitFor(() => screen.getByRole('button', { name: t('support.jobs.cancel') })));
		await waitFor(() => expect(screen.getByText(t('jobs.cancel.delete.title'))).toBeTruthy());
		expect(cancelAdminJob).not.toHaveBeenCalled();
		await fireEvent.click(screen.getByRole('button', { name: t('jobs.cancel.confirm') }));
		await waitFor(() => expect(cancelAdminJob).toHaveBeenCalledWith('job-1'));
	});
});

describe('JobDetailPlanet', () => {
	it('shows the counts, the phase steps and the failed items of a finished job', async () => {
		vi.mocked(adminJob).mockResolvedValue({
			ok: true,
			value: job({
				state: 'partial',
				progress: { done: 1180, total: 1204, bytes_done: 0, bytes_total: null, per_second: 0 },
				failed_count: 24,
				failed_items: [{ path: '/team/old/a.txt', code: 'PERMISSION_DENIED', message: 'Owner only' }]
			})
		});
		render(JobDetailPlanet, { props: { id: 'job-1' } });
		const counts = t('jobs.counts', {
			done: formatNumber(1180),
			total: formatNumber(1204),
			failed: formatNumber(24)
		});
		await waitFor(() => expect(screen.getByText(counts)).toBeTruthy());
		expect(screen.getByText(t('jobs.phase.delete'))).toBeTruthy();
		expect(screen.getByLabelText('Owner only')).toBeTruthy();
		expect(screen.getByLabelText('PERMISSION_DENIED')).toBeTruthy();
		expect(adminJob).toHaveBeenCalledWith('job-1');
	});

	it('offers Retry failed items only when a finished job has failures and retries it on press', async () => {
		vi.mocked(adminJob).mockResolvedValue({
			ok: true,
			value: job({
				state: 'partial',
				failed_count: 2,
				failed_items: [{ path: '/team/old/a.txt', code: 'PERMISSION_DENIED', message: 'Owner only' }]
			})
		});
		vi.mocked(retryAdminJob).mockResolvedValue({ ok: true, value: job({ id: 'job-1', state: 'running' }) });
		render(JobDetailPlanet, { props: { id: 'job-1' } });
		const retry = await waitFor(() => screen.getByRole('button', { name: t('jobs.retry') }));
		await fireEvent.click(retry);
		await waitFor(() => expect(retryAdminJob).toHaveBeenCalledWith('job-1'));
	});

	it('hides Retry failed items while the job is still running', async () => {
		vi.mocked(adminJob).mockResolvedValue({ ok: true, value: job({ state: 'running', failed_count: 1 }) });
		render(JobDetailPlanet, { props: { id: 'job-1' } });
		await waitFor(() => expect(screen.getByText(t('jobs.facts.kind'))).toBeTruthy());
		await waitFor(() => expect(screen.queryByRole('button', { name: t('jobs.retry') })).toBeNull());
	});

	it('shows the error state with the mapped title when the job cannot be loaded', async () => {
		vi.mocked(adminJob).mockResolvedValue({ ok: false, error: httpFailure });
		render(JobDetailPlanet, { props: { id: 'job-1' } });
		await waitFor(() => expect(screen.getByText(t('activity.jobs.gone.title'))).toBeTruthy());
	});

	it('shows the partial notice when some failed items are not listed', async () => {
		vi.mocked(adminJob).mockResolvedValue({
			ok: true,
			value: job({
				state: 'partial',
				failed_count: 1500,
				failed_items: [{ path: '/team/old/a.txt', code: 'PERMISSION_DENIED', message: 'Owner only' }]
			})
		});
		render(JobDetailPlanet, { props: { id: 'job-1' } });
		await waitFor(() => expect(screen.getByText(t('jobs.partial.title'))).toBeTruthy());
	});

	it('uses the member feed and cancels through it when the viewer is not an admin', async () => {
		sessionState.isAdmin = false;
		vi.mocked(ownJob).mockResolvedValue({ ok: true, value: job() });
		vi.mocked(cancelJob).mockResolvedValue({ ok: true, value: job({ state: 'cancelling' }) });
		render(JobDetailPlanet, { props: { id: 'job-1' } });
		await fireEvent.click(await waitFor(() => screen.getByRole('button', { name: t('jobs.cancel') })));
		await fireEvent.click(await waitFor(() => screen.getByRole('button', { name: t('jobs.cancel.confirm') })));
		await waitFor(() => expect(cancelJob).toHaveBeenCalledWith('job-1'));
		expect(ownJob).toHaveBeenCalledWith('job-1');
		expect(adminJob).not.toHaveBeenCalled();
		expect(retryJobFailed).not.toHaveBeenCalled();
	});
});
