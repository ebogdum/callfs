import { cluster as loadCluster } from '$lib/api/admin/cluster';
import { health as loadHealth } from '$lib/api/admin/health';
import { me as loadMe } from '$lib/api/session';
import type { ClusterInfo, Health, Me } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { visibleNavItems } from '$lib/nav/model';
import type { UiError } from '$lib/ui/types';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import DegradedClusterProbe from './DegradedClusterProbe.svelte';

vi.mock('$lib/api/admin/health', () => ({ health: vi.fn() }));
vi.mock('$lib/api/admin/cluster', () => ({ cluster: vi.fn() }));
vi.mock('$lib/api/session', () => ({ me: vi.fn() }));

const networkError: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true,
	layer: 'network'
};

const leaderError: UiError = {
	title: 'callfs cannot accept changes right now.',
	body: 'The cluster has no leader. Reading still works.',
	status: 503,
	code: 'NO_LEADER',
	retryable: true,
	layer: 'leader'
};

function healthyAdmin(): void {
	vi.mocked(loadHealth).mockResolvedValue({ ok: true, value: { status: 'ok' } as Health });
	vi.mocked(loadCluster).mockResolvedValue({
		ok: true,
		value: { mode: 'raft', leader_id: 'node-1' } as ClusterInfo
	});
}

function renderCluster(role: 'admin' | 'member') {
	return render(DegradedClusterProbe, { props: { role } });
}

describe('DegradedCluster admin probes', () => {
	beforeEach(() => {
		healthyAdmin();
	});

	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it('renders content without a banner when health and leader are fine', async () => {
		renderCluster('admin');
		const status = await screen.findByTestId('status');
		expect('false').toBe(status.getAttribute('data-degraded'));
		expect(status.textContent).toBe('');
		expect(screen.queryByRole('alert')).toBeNull();
		expect(loadHealth).toHaveBeenCalledWith(true);
	});

	it('shows the no-leader banner and write reason when raft has no leader', async () => {
		vi.mocked(loadCluster).mockResolvedValue({
			ok: true,
			value: { mode: 'raft', leader_id: null } as ClusterInfo
		});
		renderCluster('admin');
		expect(await screen.findByText(t('clusters.degraded.no_leader.title'))).toBeTruthy();
		const status = screen.getByTestId('status');
		await vi.waitFor(() => {
			expect('true').toBe(status.getAttribute('data-degraded'));
		});
		expect(status.textContent).toMatch(/^The server cannot accept changes right now\. No leader since /);
	});

	it('treats a NO_LEADER response from the cluster endpoint as no leader', async () => {
		vi.mocked(loadCluster).mockResolvedValue({ ok: false, error: leaderError });
		renderCluster('admin');
		expect(await screen.findByText(t('clusters.degraded.no_leader.title'))).toBeTruthy();
	});

	it('shows the health-down banner when health reports down', async () => {
		vi.mocked(loadHealth).mockResolvedValue({ ok: true, value: { status: 'down' } as Health });
		renderCluster('admin');
		expect(await screen.findByText(t('clusters.degraded.health.title'))).toBeTruthy();
		await vi.waitFor(() => {
			expect(screen.getByTestId('status').textContent).toBe(t('clusters.degraded.health.write'));
		});
	});

	it('shows the unreachable banner when the network fails', async () => {
		vi.mocked(loadHealth).mockResolvedValue({ ok: false, error: networkError });
		renderCluster('admin');
		expect(await screen.findByText(t('clusters.degraded.unreachable.title'))).toBeTruthy();
		await vi.waitFor(() => {
			expect(screen.getByTestId('status').textContent).toBe(t('clusters.degraded.unreachable.write'));
		});
	});

	it('retries on the manual Try again action and clears the banner after recovery', async () => {
		vi.mocked(loadHealth).mockResolvedValue({ ok: false, error: networkError });
		renderCluster('admin');
		await screen.findByText(t('clusters.degraded.unreachable.title'));
		healthyAdmin();
		await fireEvent.click(screen.getByRole('button', { name: t('clusters.degraded.retry') }));
		await vi.waitFor(() => {
			expect(screen.queryByText(t('clusters.degraded.unreachable.title'))).toBeNull();
		});
		expect(screen.getByTestId('status').getAttribute('data-degraded')).toBe('false');
	});
});

describe('DegradedCluster member probes', () => {
	beforeEach(() => {
		vi.mocked(loadMe).mockResolvedValue({ ok: true, value: { is_admin: false } as Me });
	});

	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it('uses the session endpoint and not admin endpoints', async () => {
		renderCluster('member');
		await screen.findByTestId('status');
		expect(loadMe).toHaveBeenCalled();
		expect(loadHealth).not.toHaveBeenCalled();
		expect(loadCluster).not.toHaveBeenCalled();
		expect(screen.queryByRole('alert')).toBeNull();
	});

	it('shows the unreachable banner when the session endpoint cannot be reached', async () => {
		vi.mocked(loadMe).mockResolvedValue({ ok: false, error: networkError });
		renderCluster('member');
		expect(await screen.findByText(t('clusters.degraded.unreachable.title'))).toBeTruthy();
		await vi.waitFor(() => {
			expect(screen.getByTestId('status').getAttribute('data-degraded')).toBe('true');
		});
	});
});

describe('navigation role filtering', () => {
	it('gives members only the role-scoped top level items', () => {
		const ids = visibleNavItems('member', []).map((item) => item.id);
		expect(ids).toEqual(['overview', 'files', 'sharing', 'activity', 'settings']);
	});

	it('removes cluster children whose capability is off for admins', () => {
		const cluster = visibleNavItems('admin', []).find((item) => 'cluster' === item.id);
		expect(cluster?.children.map((child) => child.id)).not.toContain('cluster.raft');
		expect(cluster?.children.map((child) => child.id)).toContain('cluster.topology');
	});

	it('points members at personal settings and admins at network settings', () => {
		const adminSettings = visibleNavItems('admin', []).find((item) => 'settings' === item.id);
		const memberSettings = visibleNavItems('member', []).find((item) => 'settings' === item.id);
		expect(adminSettings?.href).toBe('/settings/network');
		expect(memberSettings?.href).toBe('/settings/appearance');
	});
});
