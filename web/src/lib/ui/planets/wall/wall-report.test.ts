import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import ReportPlanet from '$lib/ui/planets/report/ReportPlanet.svelte';
import WallPlanet from '$lib/ui/planets/wall/WallPlanet.svelte';

const queries = vi.hoisted(() => new Map<string, unknown>());

vi.mock('$lib/data/query.svelte', () => ({
	createQuery: (options: { key: readonly unknown[] }) => queries.get(String(options.key.at(-1)))
}));

interface QueryOverrides {
	status?: string;
	error?: { title: string; body: string; retryable: boolean; layer: string } | null;
	updatedAt?: number | null;
}

function fakeQuery(data: unknown, overrides: QueryOverrides = {}) {
	return {
		data,
		status: overrides.status ?? 'success',
		error: overrides.error ?? null,
		updatedAt: undefined === overrides.updatedAt ? Date.now() : overrides.updatedAt,
		fetching: false,
		refetch: vi.fn(async () => undefined),
		setData: vi.fn()
	};
}

function seed(name: string, data: unknown, overrides?: QueryOverrides): void {
	queries.set(name, fakeQuery(data, overrides));
}

function seedHealthy(): void {
	seed('health', {
		status: 'ok',
		checked_at: '2026-10-23T10:00:00Z',
		components: [{ name: 'disk', status: 'ok', latency_ms: 1, detail: '', since: null }]
	});
	seed('findings', { items: [], counts: { critical: 0, warning: 0, info: 0 } });
	seed('cluster', {
		mode: 'single',
		self_id: 'callfs-1',
		leader_id: 'callfs-1',
		voters: 1,
		has_quorum: true,
		can_lose: 0,
		members: [
			{
				node_id: 'callfs-1',
				raft_addr: '',
				api_endpoint: '',
				suffrage: 'voter',
				state: 'leader',
				is_self: true,
				last_contact_ms: 0,
				behind_by: null,
				instance_id: null
			}
		]
	});
	seed('requests', { series: [] });
	seed('latency', { series: [] });
	seed('usage', {
		path: '/',
		files: 3,
		directories: 1,
		bytes: 4096,
		erasure_files: 0,
		by_storage: { localfs: { files: 3, bytes: 4096 } }
	});
	seed('growth', { series: [] });
	seed('status', { version: '1.0.0', instance_id: 'callfs-1' });
	seed('config', { config_path: '/etc/callfs.yaml', restart_required: false, pending: [], keys: [] });
}

beforeEach(() => {
	queries.clear();
	seedHealthy();
});

afterEach(() => {
	cleanup();
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe('WallPlanet', () => {
	it('shows the health board with the verdict as its level 1 heading', async () => {
		render(WallPlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: t('wall-report.board.health') })).toBeTruthy();
		expect(screen.getByText(t('wall-report.verdict.ok'))).toBeTruthy();
	});

	it('counts only unmuted critical and warning findings in the verdict', async () => {
		seed('findings', {
			items: [
				{ id: 'tls.plain_http', severity: 'critical', title: 'a', body: 'b', muted: null, fix: { label: 'Fix' } },
				{ id: 'disk.low', severity: 'warning', title: 'c', body: 'd', muted: null, fix: { label: 'Fix' } },
				{ id: 'x.muted', severity: 'warning', title: 'e', body: 'f', muted: { until: 'later' }, fix: { label: 'Fix' } }
			],
			counts: { critical: 1, warning: 2, info: 0 }
		});
		render(WallPlanet);
		await tick();
		expect(screen.getByText(t('wall-report.verdict.warn', { count: 2 }))).toBeTruthy();
	});

	it('rotates to the next board every 20 seconds', async () => {
		vi.useFakeTimers();
		render(WallPlanet);
		await tick();
		await vi.advanceTimersByTimeAsync(20000);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: t('wall-report.board.traffic') })).toBeTruthy();
	});

	it('stops rotating while Pause rotation is pressed', async () => {
		vi.useFakeTimers();
		render(WallPlanet);
		await tick();
		const pause = screen.getByRole('button', { name: t('wall-report.wall.pause') });
		await fireEvent.click(pause);
		await tick();
		expect(pause.getAttribute('aria-pressed')).toBe('true');
		await vi.advanceTimersByTimeAsync(40000);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: t('wall-report.board.health') })).toBeTruthy();
	});

	it('moves between boards with the arrow keys and wraps around', async () => {
		render(WallPlanet);
		await tick();
		await fireEvent.keyDown(window, { key: 'ArrowLeft' });
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: t('wall-report.board.storage') })).toBeTruthy();
		await fireEvent.keyDown(window, { key: 'ArrowRight' });
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: t('wall-report.board.health') })).toBeTruthy();
	});

	it('warns in the header when the data is older than 30 seconds', async () => {
		seed('health', { status: 'ok', checked_at: '', components: [] }, { updatedAt: Date.now() - 45000 });
		render(WallPlanet);
		await tick();
		expect(screen.getByText(t('wall-report.wall.stale', { seconds: 45 }))).toBeTruthy();
	});

	it('shows the plain error copy when the first health load fails', async () => {
		seed('health', undefined, {
			status: 'error',
			error: {
				title: 'We cannot reach callfs.',
				body: 'Check your connection. We will try again.',
				retryable: true,
				layer: 'network'
			}
		});
		render(WallPlanet);
		await tick();
		expect(screen.getByText('We cannot reach callfs.')).toBeTruthy();
		expect(screen.queryByText(t('wall-report.verdict.ok'))).toBeNull();
	});

	it('shows no verdict while the first health load is pending', async () => {
		seed('health', undefined, { status: 'loading' });
		render(WallPlanet);
		await tick();
		expect(screen.queryByText(t('wall-report.verdict.ok'))).toBeNull();
	});

	it('shows the storage board numbers from the usage response', async () => {
		render(WallPlanet);
		await tick();
		await fireEvent.keyDown(window, { key: 'ArrowLeft' });
		await tick();
		expect(screen.getByText(t('wall-report.kpi.files'))).toBeTruthy();
		expect(screen.getByText(t('wall-report.kpi.split'))).toBeTruthy();
	});
});

describe('ReportPlanet', () => {
	it('shows the report title as the level 1 heading', async () => {
		render(ReportPlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: t('wall-report.report.title') })).toBeTruthy();
	});

	it('calls window.print from the print button', async () => {
		const print = vi.spyOn(window, 'print').mockImplementation(() => undefined);
		render(ReportPlanet);
		await tick();
		await fireEvent.click(screen.getByRole('button', { name: t('wall-report.report.print') }));
		expect(print).toHaveBeenCalledTimes(1);
	});

	it('lists open findings with their fix labels', async () => {
		seed('findings', {
			items: [
				{
					id: 'tls.plain_http',
					severity: 'critical',
					title: 'Plain HTTP is on',
					body: 'Clients send keys unencrypted.',
					muted: null,
					fix: { label: 'Turn on HTTPS' }
				}
			],
			counts: { critical: 1, warning: 0, info: 0 }
		});
		render(ReportPlanet);
		await tick();
		expect(screen.getByText('Plain HTTP is on')).toBeTruthy();
		expect(screen.getByText(t('wall-report.report.fix', { label: 'Turn on HTTPS' }))).toBeTruthy();
	});

	it('lists only settings that differ from defaults and does not show secret values', async () => {
		seed('config', {
			config_path: '/etc/callfs.yaml',
			restart_required: false,
			pending: [],
			keys: [
				{ path: 'server.port', value: 8080, default: 8443, source: 'file', secret: false, set: true },
				{ path: 'log.level', value: 'info', default: 'info', source: 'default', secret: false, set: true },
				{ path: 'auth.secret', value: null, default: null, source: 'env', secret: true, set: true }
			]
		});
		render(ReportPlanet);
		await tick();
		expect(screen.getByText('server.port')).toBeTruthy();
		expect(screen.getByText('auth.secret')).toBeTruthy();
		expect(screen.queryByText('log.level')).toBeNull();
	});

	it('renders the cluster members table with one row per member', async () => {
		render(ReportPlanet);
		await tick();
		const table = screen.getByRole('table', { name: t('wall-report.report.members_table') });
		expect(within(table).getAllByRole('row')).toHaveLength(2);
		expect(within(table).getByText('callfs-1')).toBeTruthy();
	});
});
