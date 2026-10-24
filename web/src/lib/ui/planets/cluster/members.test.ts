import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
	cluster,
	joinConfig,
	removeNode,
	snapshot,
	snapshots,
	stateStats,
	transferLeadership
} from '$lib/api/admin/cluster';
import type { ClusterInfo, ClusterMember, Snapshot, StateStats } from '$lib/api/types';
import { CommandRegistry, type CommandContext } from '$lib/commands/registry.svelte';
import { entryFor, invalidate } from '$lib/data/cache';
import { t } from '$lib/copy/t';
import type { UiError } from '$lib/ui/types';
import MembersPlanet, { COMMANDS_CONTEXT } from './MembersPlanet.svelte';
import MemberDetailPlanet from './MemberDetailPlanet.svelte';
import ReplicationPlanet from './ReplicationPlanet.svelte';

const pageState = vi.hoisted(() => ({ url: new URL('http://127.0.0.1:4173/ui/cluster/nodes') }));

vi.mock('$app/state', () => ({ page: pageState }));

vi.mock('$lib/api/admin/cluster', () => ({
	cluster: vi.fn(),
	joinConfig: vi.fn(),
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

const SNAPSHOT: Snapshot = {
	id: 'snap-100',
	seq: 100,
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
		instance_id: null,
		api_endpoint: `https://${nodeId}.example.com:8443`,
		state: 'follower',
		is_self: false,
		online: true,
		last_seen_ms: 12,
		behind_by: 0,
		...overrides
	};
}

function healthyCluster(overrides: Partial<ClusterInfo> = {}): ClusterInfo {
	return {
		mode: 'nats',
		self_id: 'callfs-node-1',
		cluster_name: 'callfs',
		leader_id: 'callfs-node-1',
		leader_endpoint: 'https://callfs-node-1.example.com:8443',
		replicas: 3,
		has_quorum: true,
		can_lose: 1,
		log: {
			first_seq: 1,
			last_seq: 120,
			messages: 120,
			bytes: 4096,
			applied_seq: 120,
			last_snapshot: { id: 'snap-100', seq: 100, bytes: 2048, created_at: '2026-10-22T10:00:00Z' },
			snapshot_interval_ms: 300000
		},
		members: [
			member('callfs-node-1', { state: 'leader', is_self: true, last_seen_ms: null, behind_by: null }),
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
	fireEvent.input(screen.getByLabelText(t('members.add.api.label')), {
		target: { value: 'https://callfs-node-4.example.com:8443' }
	});
	fireEvent.input(screen.getByLabelText(t('members.add.cluster.label')), { target: { value: '10.0.0.14:6222' } });
}

beforeEach(() => {
	vi.clearAllMocks();
	resetCache();
	pageState.url = new URL('http://127.0.0.1:4173/ui/cluster/nodes');
	vi.mocked(cluster).mockResolvedValue({ ok: true, value: healthyCluster() });
	vi.mocked(joinConfig).mockResolvedValue({
		ok: true,
		value: { yaml: 'nats:\n  enabled: true\n', steps: ['Start the new node.'] }
	});
	vi.mocked(removeNode).mockResolvedValue({ ok: true, value: { status: 'removed', remaining: 2 } });
	vi.mocked(transferLeadership).mockResolvedValue({
		ok: true,
		value: { status: 'transferred', leader_id: 'callfs-node-2' }
	});
	vi.mocked(snapshot).mockResolvedValue({ ok: true, value: { id: 'snap-120', seq: 120, bytes: 2048, took_ms: 8 } });
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
				replicas: 1,
				members: [member('callfs-node-1', { state: 'leader', is_self: true })]
			})
		});
		render(MembersPlanet);
		expect(await screen.findByText(t('members.single.title'))).toBeInTheDocument();
		expect(screen.queryByRole('link', { name: 'callfs-node-1' })).toBeNull();
	});

	it('asks for a member ID before creating the config', async () => {
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.add.button') }));
		const dialog = await screen.findByRole('dialog');
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.add.generate') }));
		expect(await screen.findByText(t('members.add.error.id_required'))).toBeInTheDocument();
		expect(joinConfig).not.toHaveBeenCalled();
	});

	it('creates the config for a new member and shows the yaml and the steps', async () => {
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.add.button') }));
		const dialog = await screen.findByRole('dialog');
		fillNewMember();
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.add.generate') }));
		await waitFor(() => {
			expect(joinConfig).toHaveBeenCalledWith({
				node_id: 'callfs-node-4',
				api_endpoint: 'https://callfs-node-4.example.com:8443',
				cluster_addr: '10.0.0.14:6222'
			});
		});
		expect(await within(dialog).findByText('Start the new node.')).toBeInTheDocument();
		expect(within(dialog).getByText(/enabled: true/)).toBeInTheDocument();
		expect(within(dialog).getByRole('button', { name: new RegExp(t('common.copy')) })).toBeInTheDocument();
	});

	it('removes an offline member after confirmation', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/ui/cluster/nodes?remove=callfs-node-3');
		vi.mocked(cluster).mockResolvedValue({
			ok: true,
			value: healthyCluster({
				members: [
					member('callfs-node-1', { state: 'leader', is_self: true }),
					member('callfs-node-2'),
					member('callfs-node-3', { state: 'offline', online: false, last_seen_ms: null, behind_by: null })
				]
			})
		});
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.remove.confirm') }));
		await waitFor(() => {
			expect(removeNode).toHaveBeenCalledWith('callfs-node-3');
		});
	});

	it('explains a conflict when the member to remove is still online', async () => {
		pageState.url = new URL('http://127.0.0.1:4173/ui/cluster/nodes?remove=callfs-node-3');
		vi.mocked(removeNode).mockResolvedValueOnce({
			ok: false,
			error: { title: 'Conflict', body: 'Conflict', status: 409, retryable: false, layer: 'conflict' }
		});
		render(MembersPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.remove.confirm') }));
		expect(await screen.findByText(t('members.remove.online.title', { node: 'callfs-node-3' }))).toBeInTheDocument();
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

describe('ReplicationPlanet', () => {
	it('shows the job title and takes a snapshot from the primary action', async () => {
		render(ReplicationPlanet);
		expect(await screen.findByRole('heading', { level: 1, name: t('members.replication.title') })).toBeInTheDocument();
		expect(screen.getByText(t('members.replication.job'))).toBeInTheDocument();
		fireEvent.click(await screen.findByRole('button', { name: t('members.replication.snapshot') }));
		await waitFor(() => {
			expect(snapshot).toHaveBeenCalledTimes(1);
		});
	});

	it('lists the log facts, the copies and the snapshots', async () => {
		render(ReplicationPlanet);
		expect(await screen.findByText(t('members.replication.applied'))).toBeInTheDocument();
		expect(await screen.findByText('snap-100')).toBeInTheDocument();
		expect(screen.getAllByText('callfs-node-2').length).toBeGreaterThan(0);
	});

	it('hides the technical rows when the toggle is switched off', async () => {
		render(ReplicationPlanet);
		expect(await screen.findByText(t('members.replication.applied'))).toBeInTheDocument();
		const toggle = await screen.findByRole('button', { name: t('common.show_technical') });
		expect(toggle).toHaveAttribute('aria-pressed', 'true');
		fireEvent.click(toggle);
		expect(toggle).toHaveAttribute('aria-pressed', 'false');
		await waitFor(() => {
			expect(screen.queryByText(t('members.replication.applied'))).toBeNull();
		});
	});

	it('moves the leader to the automatic choice', async () => {
		render(ReplicationPlanet);
		fireEvent.click(await screen.findByRole('button', { name: t('members.replication.move') }));
		const dialog = await screen.findByRole('dialog');
		fireEvent.click(within(dialog).getByRole('button', { name: t('members.replication.move.confirm') }));
		await waitFor(() => {
			expect(transferLeadership).toHaveBeenCalledWith(null);
		});
	});

	it('explains that replication is off when the cluster runs alone', async () => {
		vi.mocked(cluster).mockResolvedValue({
			ok: true,
			value: healthyCluster({ mode: 'single', replicas: 1, members: [member('callfs-node-1', { state: 'leader', is_self: true })] })
		});
		render(ReplicationPlanet);
		expect(await screen.findByText(t('members.replication.single.title'))).toBeInTheDocument();
	});

	it('registers the snapshot command while mounted and removes it on destroy', async () => {
		const registry = new CommandRegistry({ context: () => CONTEXT });
		render(ReplicationPlanet, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await waitFor(() => {
			expect(registry.list(CONTEXT).map((entry) => entry.command.id)).toContain('cluster.snapshot');
		});
		cleanup();
		expect(registry.list(CONTEXT).map((entry) => entry.command.id)).not.toContain('cluster.snapshot');
	});
});
