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

const NATS_MEMBERS: ClusterMember[] = [
	{
		node_id: 'callfs-node-1',
		instance_id: 'callfs-node-1',
		api_endpoint: 'https://10.0.0.1:8443',
		state: 'leader',
		is_self: true,
		online: true,
		last_seen_ms: 0,
		behind_by: 0
	},
	{
		node_id: 'callfs-node-2',
		instance_id: 'callfs-node-2',
		api_endpoint: 'https://10.0.0.2:8443',
		state: 'follower',
		is_self: false,
		online: true,
		last_seen_ms: 12,
		behind_by: 0
	},
	{
		node_id: 'callfs-node-3',
		instance_id: null,
		api_endpoint: null,
		state: 'offline',
		is_self: false,
		online: false,
		last_seen_ms: null,
		behind_by: null
	}
];

function clusterInfo(overrides: Partial<ClusterInfo> = {}): ClusterInfo {
	return {
		mode: 'nats',
		self_id: 'callfs-node-1',
		cluster_name: 'callfs',
		leader_id: 'callfs-node-1',
		leader_endpoint: 'https://10.0.0.1:8443',
		replicas: 3,
		has_quorum: true,
		can_lose: 1,
		log: { first_seq: 1, last_seq: 120, messages: 120, bytes: 4096, applied_seq: 120, last_snapshot: null, snapshot_interval_ms: 300000 },
		members: NATS_MEMBERS,
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
			pieces: 0,
			disk_free_bytes: null,
			disk_total_bytes: null
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
			pieces: 0,
			disk_free_bytes: null,
			disk_total_bytes: null
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
				replicas: 1,
				members: [NATS_MEMBERS[0]],
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
