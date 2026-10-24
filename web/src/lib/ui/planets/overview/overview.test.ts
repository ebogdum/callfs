import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cluster } from '$lib/api/admin/cluster';
import { findings } from '$lib/api/admin/findings';
import { health } from '$lib/api/admin/health';
import { jobs } from '$lib/api/admin/jobs';
import { status } from '$lib/api/admin/status';
import { series } from '$lib/api/admin/telemetry';
import { listLinks } from '$lib/api/links';
import { usage } from '$lib/api/search';
import { myEvents, myJobs } from '$lib/api/session';
import type {
	AdminStatus,
	ClusterInfo,
	Finding,
	Health,
	Job,
	Me,
	SeriesResult,
	Usage
} from '$lib/api/types';
import { t } from '$lib/copy/t';
import { preferences } from '$lib/stores/preferences.svelte';
import { session } from '$lib/stores/session.svelte';
import OverviewPlanet from './OverviewPlanet.svelte';
import SetupChecklist, { type SetupItem } from './SetupChecklist.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$lib/api/admin/cluster', () => ({ cluster: vi.fn() }));
vi.mock('$lib/api/admin/findings', () => ({
	findings: vi.fn(),
	mute: vi.fn(),
	unmute: vi.fn(),
	ignore: vi.fn(),
	unignore: vi.fn(),
	ignoreUnsupported: vi.fn(() => false)
}));
vi.mock('$lib/api/admin/health', () => ({ health: vi.fn() }));
vi.mock('$lib/api/admin/jobs', () => ({ jobs: vi.fn(), cancel: vi.fn(), retryFailed: vi.fn() }));
vi.mock('$lib/api/admin/status', () => ({ status: vi.fn() }));
vi.mock('$lib/api/admin/telemetry', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/admin/telemetry')>()),
	series: vi.fn()
}));
vi.mock('$lib/api/links', () => ({ listLinks: vi.fn() }));
vi.mock('$lib/api/nodes', () => ({
	listNodes: vi.fn(() => Promise.resolve({ ok: true, value: { self: '', nodes: [] } })),
	roleOf: () => 'none'
}));
vi.mock('$lib/api/search', () => ({ usage: vi.fn() }));
vi.mock('$lib/api/session', () => ({ myEvents: vi.fn(), myJobs: vi.fn() }));

const adminMe: Me = {
	user_id: 'root',
	is_admin: true,
	auth_method: 'session',
	session_expires_at: null,
	csrf_token: null,
	permissions_model: 'owner'
};

const memberMe: Me = { ...adminMe, user_id: 'alice', is_admin: false };

const healthy: Health = { status: 'ok', checked_at: '2026-10-23T10:00:00Z', components: [] };

const singleServer: ClusterInfo = {
	mode: 'single',
	self_id: 'node-1',
	leader_id: 'node-1',
	leader_endpoint: null,
	term: 1,
	last_log_index: 0,
	commit_index: 0,
	applied_index: 0,
	last_snapshot: null,
	voters: 1,
	has_quorum: true,
	can_lose: 0,
	members: []
};

const httpsStatus = { protocol_effective: 'https' } as unknown as AdminStatus;

const usageWith = (files: number): Usage =>
	({
		path: '/',
		files,
		directories: 0,
		bytes: 1_200_000,
		by_storage: { localfs: { files, bytes: 1_200_000 } },
		by_instance: {},
		by_owner: {},
		erasure_files: 0,
		erasure_raw_bytes: 0
	}) as unknown as Usage;

const emptySeries: SeriesResult = {
	name: 'callfs_http_requests_total',
	measure: 'rate',
	unit: 'per_s',
	step_s: 900,
	series: [],
	sampled_since: null
};

const secretFinding: Finding = {
	id: 'secret.default',
	severity: 'critical',
	area: 'security',
	title: 'Default secret in use',
	body: 'Replace it before serving users.',
	affected: [],
	since: '2026-10-22T10:00:00Z',
	fix: { label: 'Fix secret', route: '/settings/security', setting_key: 'auth.internal_proxy_secret', command: null },
	muted: null
};

const okOf = <T,>(value: T) => ({ ok: true as const, value });

const dataResponse = <T,>(data: T) => ({ ok: true as const, value: { data, status: 200, headers: new Headers(), requestId: null, etag: null } });

function setUpAdminApi(options: { files?: number; findingItems?: Finding[] } = {}): void {
	vi.mocked(health).mockResolvedValue(okOf(healthy));
	vi.mocked(status).mockResolvedValue(okOf(httpsStatus));
	vi.mocked(cluster).mockResolvedValue(okOf(singleServer));
	vi.mocked(findings).mockResolvedValue(
		okOf({ items: options.findingItems ?? [], counts: { critical: 0, warning: 0, info: 0 } })
	);
	vi.mocked(jobs).mockResolvedValue(okOf({ items: [] as Job[] }));
	vi.mocked(usage).mockResolvedValue(dataResponse(usageWith(options.files ?? 12)) as never);
	vi.mocked(series).mockResolvedValue(okOf(emptySeries));
}

function setUpMemberApi(): void {
	vi.mocked(usage).mockResolvedValue(dataResponse(usageWith(3)) as never);
	vi.mocked(listLinks).mockResolvedValue(okOf({ items: [], next_cursor: null, counts: { active: 0, used: 0, expired: 0, revoked: 0 } }));
	vi.mocked(myJobs).mockResolvedValue(okOf({ items: [], next_cursor: null }));
	vi.mocked(myEvents).mockResolvedValue(okOf({ items: [] }));
}

beforeEach(() => {
	vi.clearAllMocks();
	session.me = adminMe;
	preferences.update({ dismissed: [] });
});

afterEach(() => {
	cleanup();
});

describe('SetupChecklist', () => {
	const items: SetupItem[] = [
		{ id: 'https', label: 'Use safe connections (HTTPS)', done: true, href: '/settings/network#server.protocol' },
		{ id: 'secrets', label: 'Replace default secrets', done: false, href: '/settings/security' }
	];

	it('shows progress and links each item to its setting', () => {
		render(SetupChecklist, { props: { items, ondismiss: vi.fn() } });
		expect(screen.getByText(t('overview.setup.progress', { done: 1, total: 2 }))).toBeTruthy();
		expect(screen.getByRole('link', { name: `Replace default secrets ${t('overview.setup.todo')}` }).getAttribute('href')).toBe('/settings/security');
	});

	it('names each item state with a word beside its glyph', () => {
		render(SetupChecklist, { props: { items, ondismiss: vi.fn() } });
		expect(screen.getByText(t('overview.setup.done'))).toBeTruthy();
		expect(screen.getByText(t('overview.setup.todo'))).toBeTruthy();
	});

	it('calls ondismiss when the dismiss button is pressed', async () => {
		const ondismiss = vi.fn();
		render(SetupChecklist, { props: { items, ondismiss } });
		await fireEvent.click(screen.getByRole('button', { name: t('overview.setup.dismiss') }));
		expect(ondismiss).toHaveBeenCalledTimes(1);
	});
});

describe('OverviewPlanet', () => {
	it('states that all systems work when nothing needs attention', async () => {
		setUpAdminApi();
		render(OverviewPlanet);
		expect(await screen.findByRole('heading', { name: t('overview.verdict.ok') })).toBeTruthy();
	});

	it('counts attention items and names the top finding as the cause', async () => {
		setUpAdminApi({ findingItems: [secretFinding] });
		render(OverviewPlanet);
		expect(
			await screen.findByRole('heading', { name: t('overview.verdict.attention', { count: 1 }) })
		).toBeTruthy();
		expect(screen.getAllByText('Default secret in use').length).toBeGreaterThan(0);
	});

	it('shows the first-use empty state when there are no files', async () => {
		setUpAdminApi({ files: 0 });
		render(OverviewPlanet);
		expect(await screen.findByText(t('overview.empty.title'))).toBeTruthy();
		expect(screen.getByText(t('overview.empty.body'))).toBeTruthy();
	});

	it('hides the setup checklist after it is dismissed', async () => {
		setUpAdminApi();
		render(OverviewPlanet);
		await fireEvent.click(await screen.findByRole('button', { name: t('overview.setup.dismiss') }));
		await waitFor(() => {
			expect(screen.queryByRole('region', { name: t('overview.setup.title') })).toBeNull();
		});
		expect(preferences.value.dismissed).toContain('overview.setup');
	});

	it('shows the member view without admin-only regions', async () => {
		session.me = memberMe;
		setUpMemberApi();
		render(OverviewPlanet);
		expect(await screen.findByRole('heading', { name: t('overview.verdict.member') })).toBeTruthy();
		expect(screen.queryByRole('region', { name: t('overview.regions.charts') })).toBeNull();
		expect(screen.queryByRole('region', { name: t('overview.setup.title') })).toBeNull();
	});
});
