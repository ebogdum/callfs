import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { goto } from '$app/navigation';
import { cluster } from '$lib/api/admin/cluster';
import { instances } from '$lib/api/admin/topology';
import type { ClusterInfo, ClusterMember } from '$lib/api/types';
import { clearCache } from '$lib/data/cache';
import { t } from '$lib/copy/t';
import TopologyPlanet from './TopologyPlanet.svelte';

vi.mock('$lib/api/admin/cluster', () => ({ cluster: vi.fn() }));
vi.mock('$lib/api/admin/topology', () => ({ instances: vi.fn() }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

const RAFT_MEMBERS: ClusterMember[] = [
	{
		node_id: 'callfs-node-1',
		raft_addr: '10.0.0.1:7000',
		api_endpoint: 'https://10.0.0.1:8443',
		suffrage: 'voter',
		state: 'leader',
		is_self: true,
		last_contact_ms: 0,
		behind_by: 0,
		instance_id: 'callfs-node-1'
	},
	{
		node_id: 'callfs-node-2',
		raft_addr: '10.0.0.2:7000',
		api_endpoint: 'https://10.0.0.2:8443',
		suffrage: 'voter',
		state: 'follower',
		is_self: false,
		last_contact_ms: 12,
		behind_by: 0,
		instance_id: 'callfs-node-2'
	},
	{
		node_id: 'callfs-node-3',
		raft_addr: '10.0.0.3:7000',
		api_endpoint: 'https://10.0.0.3:8443',
		suffrage: 'voter',
		state: 'unreachable',
		is_self: false,
		last_contact_ms: null,
		behind_by: null,
		instance_id: null
	}
];

function clusterInfo(overrides: Partial<ClusterInfo> = {}): ClusterInfo {
	return {
		mode: 'raft',
		self_id: 'callfs-node-1',
		leader_id: 'callfs-node-1',
		leader_endpoint: 'https://10.0.0.1:8443',
		term: 7,
		last_log_index: 120,
		commit_index: 120,
		applied_index: 120,
		last_snapshot: null,
		voters: 3,
		has_quorum: true,
		can_lose: 1,
		members: RAFT_MEMBERS,
		...overrides
	};
}

const SERVER_LIST = {
	self: { instance_id: 'callfs-node-1', external_url: null, started_at: '2026-10-20T00:00:00Z' },
	items: [
		{
			instance_id: 'callfs-node-1',
			endpoint: 'https://10.0.0.1:8443',
			source: 'self' as const,
			is_self: true,
			scheme: 'https' as const,
			reachable: true,
			rtt_ms: 2,
			checked_at: null,
			files: 10,
			bytes: 1024,
			pieces: 0
		},
		{
			instance_id: 'callfs-node-2',
			endpoint: 'https://10.0.0.2:8443',
			source: 'joined' as const,
			is_self: false,
			scheme: 'https' as const,
			reachable: true,
			rtt_ms: 4,
			checked_at: null,
			files: 0,
			bytes: 0,
			pieces: 0
		}
	]
};

beforeEach(() => {
	clearCache();
	vi.mocked(goto).mockReset();
	vi.mocked(cluster).mockResolvedValue({ ok: true, value: clusterInfo() });
	vi.mocked(instances).mockResolvedValue({ ok: true, value: SERVER_LIST });
});

afterEach(() => {
	cleanup();
});

describe('TopologyPlanet', () => {
	it('renders the page title as the only h1 and one primary action in the success state', async () => {
		render(TopologyPlanet);
		expect(await screen.findByRole('heading', { level: 1, name: t('topology.title') })).toBeTruthy();
		expect(await screen.findByText(t('topology.kpi.members'))).toBeTruthy();
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		expect(screen.getAllByRole('button', { name: t('topology.primary.add_member') })).toHaveLength(1);
	});

	it('shows no primary action while the cluster is loading', () => {
		vi.mocked(cluster).mockReturnValue(new Promise<never>(() => undefined));
		render(TopologyPlanet);
		expect(screen.getByRole('heading', { level: 1, name: t('topology.title') })).toBeTruthy();
		expect(screen.queryByRole('button', { name: t('topology.primary.add_member') })).toBeNull();
	});

	it('opens the members page with the add member action when the primary button is pressed', async () => {
		render(TopologyPlanet);
		const button = await screen.findByRole('button', { name: t('topology.primary.add_member') });
		await fireEvent.click(button);
		expect(goto).toHaveBeenCalledWith('/ui/cluster/nodes?add=member');
	});

	it('explains a single server with a not-available message and no primary action', async () => {
		vi.mocked(cluster).mockResolvedValue({
			ok: true,
			value: clusterInfo({
				mode: 'single',
				leader_id: null,
				voters: 1,
				members: [RAFT_MEMBERS[0]],
				has_quorum: true,
				can_lose: 0
			})
		});
		vi.mocked(instances).mockResolvedValue({ ok: true, value: { ...SERVER_LIST, items: [SERVER_LIST.items[0]] } });
		render(TopologyPlanet);
		expect(await screen.findByText(t('topology.single.title'))).toBeTruthy();
		expect(screen.getByText(t('topology.single.body'))).toBeTruthy();
		expect(screen.queryByRole('button', { name: t('topology.primary.add_member') })).toBeNull();
	});

	it('shows the error state with the cluster error title when the cluster request fails', async () => {
		vi.mocked(cluster).mockResolvedValue({
			ok: false,
			error: {
				title: 'We cannot reach callfs.',
				body: 'Check your connection. We will try again.',
				retryable: true,
				layer: 'network'
			}
		});
		render(TopologyPlanet);
		expect(await screen.findByText('We cannot reach callfs.')).toBeTruthy();
		expect(screen.queryByRole('button', { name: t('topology.primary.add_member') })).toBeNull();
	});

	it('shows the partial alert when the server list fails but the cluster loads', async () => {
		vi.mocked(instances).mockResolvedValue({ ok: false, error: { kind: 'network' } });
		render(TopologyPlanet);
		expect(await screen.findByText(t('topology.partial.title'))).toBeTruthy();
		expect(screen.getByText(t('topology.partial.body'))).toBeTruthy();
		expect(screen.getByText(t('topology.kpi.members'))).toBeTruthy();
	});
});
