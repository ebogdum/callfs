import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
	cluster,
	join,
	preflight,
	removeNode,
	snapshot,
	snapshots,
	stateStats,
	transferLeadership
} from '$lib/api/admin/cluster';
import type { ClusterInfo, ClusterMember, Preflight, Snapshot, StateStats } from '$lib/api/types';
import { CommandRegistry, type CommandContext } from '$lib/commands/registry.svelte';
import { entryFor, invalidate } from '$lib/data/cache';
import { t } from '$lib/copy/t';
import type { UiError } from '$lib/ui/types';
import MembersPlanet, { COMMANDS_CONTEXT } from './MembersPlanet.svelte';
import MemberDetailPlanet from './MemberDetailPlanet.svelte';
import RaftPlanet from './RaftPlanet.svelte';

const pageState = vi.hoisted(() => ({ url: new URL('http://127.0.0.1:4173/ui/cluster/nodes') }));

vi.mock('$app/state', () => ({ page: pageState }));

vi.mock('$lib/api/admin/cluster', () => ({
	cluster: vi.fn(),
	join: vi.fn(),
	preflight: vi.fn(),
	removeNode: vi.fn(),
	transferLeadership: vi.fn(),
	snapshot: vi.fn(),
	snapshots: vi.fn(),
	stateStats: vi.fn()
}));

const CONTEXT: CommandContext = { role: 'admin', route: '/cluster/nodes', selectionCount: 0, capabilities: [] };

const UNREACHABLE: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true,
	layer: 'network'
};

const PASSING_CHECKS: Preflight = {
	checks: [
		{ name: 'id_unique', ok: true, detail: '' },
		{ name: 'addr_unused', ok: true, detail: '' },
		{ name: 'raft_reachable', ok: true, detail: '' },
		{ name: 'api_reachable', ok: true, detail: '' },
		{ name: 'api_https', ok: true, detail: '' },
		{ name: 'secret_ok', ok: true, detail: '' }
	],
	would_replace: []
};

const SNAPSHOT: Snapshot = {
	id: 'snap-100',
	index: 100,
	term: 3,
	bytes: 2048,
	created_at: '2026-10-22T10:00:00Z'
};

const STATS: StateStats = {
	files: 48210,
	directories: 912,
	links: { active: 23, used: 40, expired: 5, revoked: 2 },
	erasure_entries: 12,
	index_entries: 49000
};

const CLUSTER_KEYS = [['cluster'], ['cluster', 'snapshots'], ['cluster', 'state-stats']];

function member(nodeId: string, overrides: Partial<ClusterMember> = {}): ClusterMember {
	return {
		node_id: nodeId,
		raft_addr: `${nodeId}.internal:7000`,
		api_endpoint: `https://${nodeId}.example.com:8443`,
		suffrage: 'voter',
		state: 'follower',
		is_self: false,
		last_contact_ms: 12,
		behind_by: 0,
		instance_id: null,
		...overrides
	};
}

function healthyCluster(overrides: Partial<ClusterInfo> = {}): ClusterInfo {
	return {
		mode: 'raft',
		self_id: 'callfs-node-1',
		leader_id: 'callfs-node-1',
		leader_endpoint: 'https://callfs-node-1.example.com:8443',
		term: 4,
		last_log_index: 120,
		commit_index: 120,
		applied_index: 120,
		last_snapshot: { index: 100, term: 3, at: '2026-10-22T10:00:00Z' },
		voters: 3,
		has_quorum: true,
		can_lose: 1,
		members: [
			member('callfs-node-1', { state: 'leader', is_self: true, last_contact_ms: null, behind_by: null }),
			member('callfs-node-2'),
			member('callfs-node-3')
		],
		...overrides
	};
}

function resetCache(): void {
	for (const parts of CLUSTER_KEYS) {
		const entry = entryFor(parts);
		entry.data = undefined;
		entry.error = null;
		entry.updatedAt = null;
		entry.inFlight = null;
	}
}

function fillNewMember(): void {
	fireEvent.input(screen.getByLabelText(t('members.add.id.label')), { target: { value: 'callfs-node-4' } });
	fireEvent.input(screen.getByLabelText(t('members.add.raft.label')), { target: { value: '10.0.0.14:7000' } });
	fireEvent.input(screen.getByLabelText(t('members.add.api.label')), {
		target: { value: 'https://callfs-node-4.example.com:8443' }
	});
}

beforeEach(() => {
	vi.clearAllMocks();
	resetCache();
	pageState.url = new URL('http://127.0.0.1:4173/ui/cluster/nodes');
	vi.mocked(cluster).mockResolvedValue({ ok: true, value: healthyCluster() });
	vi.mocked(preflight).mockResolvedValue({ ok: true, value: PASSING_CHECKS });
	vi.mocked(join).mockResolvedValue({
		ok: true,
		value: { status: 'joined', leader_id: 'callfs-node-1', replaced: [] }
	});
	vi.mocked(removeNode).mockResolvedValue({
		ok: true,
		value: { status: 'removed', leader_id: 'callfs-node-1', voters_after: 2 }
	});
	vi.mocked(transferLeadership).mockResolvedValue({
		ok: true,
		value: { status: 'transferred', leader_id: 'callfs-node-2' }
	});
	vi.mocked(snapshot).mockResolvedValue({ ok: true, value: { index: 120, term: 4, bytes: 2048, took_ms: 8 } });
	vi.mocked(snapshots).mockResolvedValue({ ok: true, value: { retain: 2, items: [SNAPSHOT] } });
	vi.mocked(stateStats).mockResolvedValue({ ok: true, value: STATS });
});

describe('MembersPlanet', () => {
	it('shows the job title, the members and one primary action', async () => {
		render(MembersPlanet);
		expect(await screen.findByRole('heading', { level: 1, name: t('members.title') })).toBeInTheDocument();
		expect(screen.getByText(t('members.job'))).toBeInTheDocument();
		expect(await screen.findByRole('link', { name: 'callfs-node-2' })).toHaveAttribute(
			'href',
			'/ui/cluster/nodes/callfs-node-2'
		);
		expect(screen.getAllByRole('button', { name: t('members.add.button') })).toHaveLength(1);
	});

	it('shows the empty copy when no members are listed', async () => {
		vi.mocked(cluster).mockResolvedValue({ ok: true, value: healthyCluster({ members: [] }) });
		render(MembersPlanet);
		expect(await screen.findByText(t('members.empty.title'))).toBeInTheDocument();
	});

	it('shows the error copy and retries the load', async () => {
		vi.mocked(cluster).mockResolvedValue({ ok: false, error: UNREACHABLE });
		render(MembersPlanet);
		expect(await screen.findByText(UNREACHABLE.title)).toBeInTheDocument();
		fireEvent.click(screen.getByRole('button', { name: t('common.retry') }));
		await waitFor(() => {
			expect(vi.mocked(cluster).mock.calls.length).toBeGreaterThan(1);
		});
	});

	it('keeps the listed members and shows the failure when a refresh fails', async () => {
		render(MembersPlanet);
		expect(await screen.findByRole('link', { name: 'callfs-node-2' })).toBeInTheDocument();
		vi.mocked(cluster).mockResolvedValue({ ok: false, error: UNREACHABLE });
		invalidate(['cluster']);
		expect(await screen.findByText(UNREACHABLE.title)).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'callfs-node-2' })).toBeInTheDocument();
	});

	it('replaces the member table with a note when the cluster runs alone', async () => {
		vi.mocked(cluster).mockResolvedValue({
			ok: true,
			value: healthyCluster({
				mode: 'single',
				voters: 1,
				members: [member('callfs-node-1', { state: 'leader', is_self: true })]
			})
		});
		render(MembersPlanet);
		expect(await screen.findByText(t('members.single.title'))).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'callfs-node-1' })).toBeNull();
	});

	it('asks for a member ID before running the checks', async () => {
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.add.button') }));
		const dialog = await screen.findByRole('dialog');
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.add.next') }));
		expect(await screen.findByText(t('members.add.error.id_required'))).toBeInTheDocument();
		expect(preflight).not.toHaveBeenCalled();
	});

	it('runs the checks for a new member and joins it after review', async () => {
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.add.button') }));
		const dialog = await screen.findByRole('dialog');
		fillNewMember();
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.add.next') }));
		await waitFor(() => {
			expect(preflight).toHaveBeenCalledWith({
				node_id: 'callfs-node-4',
				raft_addr: '10.0.0.14:7000',
				api_endpoint: 'https://callfs-node-4.example.com:8443'
			});
		});
		await waitFor(() => {
			expect(within(dialog).getByRole('button', { name: t('members.add.next') })).not.toHaveAttribute(
				'aria-disabled',
				'true'
			);
		});
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.add.next') }));
		expect(
			await screen.findByText(
				t('members.review.sentence', { node: 'callfs-node-4', before: '3', after: '4', quorum: '3' })
			)
		).toBeInTheDocument();
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.add.button') }));
		await waitFor(() => {
			expect(join).toHaveBeenCalledWith({
				node_id: 'callfs-node-4',
				raft_addr: '10.0.0.14:7000',
				api_endpoint: 'https://callfs-node-4.example.com:8443'
			});
		});
	});

	it('blocks removing a member when the cluster would lose quorum', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/ui/cluster/nodes?remove=callfs-node-2');
		vi.mocked(cluster).mockResolvedValue({
			ok: true,
			value: healthyCluster({
				members: [
					member('callfs-node-1', { state: 'leader', is_self: true }),
					member('callfs-node-2', { state: 'unreachable' }),
					member('callfs-node-3', { state: 'unreachable' })
				]
			})
		});
		render(MembersPlanet);
		expect(
			await screen.findByText(t('members.remove.blocked.title', { node: 'callfs-node-2' }))
		).toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('members.remove.anyway') })).toBeInTheDocument();
	});

	it('removes a member that keeps quorum after confirmation', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/ui/cluster/nodes?remove=callfs-node-3');
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.remove.confirm') }));
		await waitFor(() => {
			expect(removeNode).toHaveBeenCalledWith('callfs-node-3', false);
		});
	});

	it('registers the add member command while mounted and removes it on destroy', async () => {
		const registry = new CommandRegistry({ context: () => CONTEXT });
		render(MembersPlanet, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await waitFor(() => {
			expect(registry.list(CONTEXT).map((entry) => entry.command.id)).toContain('cluster.member.add');
		});
		cleanup();
		expect(registry.list(CONTEXT).map((entry) => entry.command.id)).not.toContain('cluster.member.add');
	});
});

describe('MemberDetailPlanet', () => {
	it('shows the member and transfers leadership to a follower', async () => {
		render(MemberDetailPlanet, { props: { id: 'callfs-node-2' } });
		expect(
			await screen.findByRole('heading', { level: 1, name: t('members.detail.title', { node: 'callfs-node-2' }) })
		).toBeInTheDocument();
		fireEvent.click(await screen.findByRole('button', { name: t('members.detail.transfer') }));
		await waitFor(() => {
			expect(transferLeadership).toHaveBeenCalledWith('callfs-node-2');
		});
	});

	it('reports a failed transfer in place', async () => {
		vi.mocked(transferLeadership).mockResolvedValueOnce({ ok: false, error: UNREACHABLE });
		render(MemberDetailPlanet, { props: { id: 'callfs-node-2' } });
		fireEvent.click(await screen.findByRole('button', { name: t('members.detail.transfer') }));
		expect(await screen.findByText(UNREACHABLE.title)).toBeInTheDocument();
	});

	it('explains that the member is not in the cluster', async () => {
		render(MemberDetailPlanet, { props: { id: 'callfs-node-9' } });
		expect(
			await screen.findByText(t('members.detail.missing.title', { node: 'callfs-node-9' }))
		).toBeInTheDocument();
	});

	it('registers the transfer and remove commands while mounted', async () => {
		const registry = new CommandRegistry({ context: () => CONTEXT });
		render(MemberDetailPlanet, {
			props: { id: 'callfs-node-2' },
			context: new Map([[COMMANDS_CONTEXT, registry]])
		});
		await waitFor(() => {
			expect(registry.list(CONTEXT).map((entry) => entry.command.id)).toEqual(
				expect.arrayContaining(['cluster.member.transfer', 'cluster.member.remove'])
			);
		});
		cleanup();
	});
});

describe('RaftPlanet', () => {
	it('shows the job title and takes a snapshot from the primary action', async () => {
		render(RaftPlanet);
		expect(await screen.findByRole('heading', { level: 1, name: t('members.raft.title') })).toBeInTheDocument();
		expect(screen.getByText(t('members.raft.job'))).toBeInTheDocument();
		fireEvent.click(await screen.findByRole('button', { name: t('members.raft.snapshot') }));
		await waitFor(() => {
			expect(snapshot).toHaveBeenCalledTimes(1);
		});
	});

	it('hides the technical rows when the toggle is switched off', async () => {
		render(RaftPlanet);
		expect(await screen.findByText(t('members.raft.commit'))).toBeInTheDocument();
		const toggle = await screen.findByRole('button', { name: t('common.show_technical') });
		expect(toggle).toHaveAttribute('aria-pressed', 'true');
		fireEvent.click(toggle);
		expect(toggle).toHaveAttribute('aria-pressed', 'false');
		await waitFor(() => {
			expect(screen.queryByText(t('members.raft.commit'))).toBeNull();
		});
	});

	it('explains that Raft is off when the cluster runs alone', async () => {
		vi.mocked(cluster).mockResolvedValue({
			ok: true,
			value: healthyCluster({ mode: 'single', voters: 1, members: [member('callfs-node-1', { state: 'leader', is_self: true })] })
		});
		render(RaftPlanet);
		expect(await screen.findByText(t('members.raft.single.title'))).toBeInTheDocument();
	});

	it('registers the snapshot command while mounted and removes it on destroy', async () => {
		const registry = new CommandRegistry({ context: () => CONTEXT });
		render(RaftPlanet, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await waitFor(() => {
			expect(registry.list(CONTEXT).map((entry) => entry.command.id)).toContain('cluster.snapshot');
		});
		cleanup();
		expect(registry.list(CONTEXT).map((entry) => entry.command.id)).not.toContain('cluster.snapshot');
	});
});
