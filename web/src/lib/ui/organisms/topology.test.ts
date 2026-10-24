import '@testing-library/jest-dom/vitest';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import type { ClusterMember, ServerInfo } from '$lib/api/types';
import { t } from '$lib/copy/t';
import TopologyMap from './TopologyMap.svelte';
import MemberCard from './MemberCard.svelte';
import PieceMap, { type Piece } from './PieceMap.svelte';

afterEach(cleanup);

const members: ClusterMember[] = [
	{
		node_id: 'callfs-node-1',
		instance_id: 'inst-1',
		api_endpoint: 'https://node1.example:8080',
		state: 'leader',
		is_self: true,
		online: true,
		last_seen_ms: 0,
		behind_by: 0
	},
	{
		node_id: 'callfs-node-2',
		instance_id: 'inst-2',
		api_endpoint: 'https://node2.example:8080',
		state: 'follower',
		is_self: false,
		online: true,
		last_seen_ms: 1200,
		behind_by: 3
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

const servers: ServerInfo[] = [
	{
		instance_id: 'inst-1',
		endpoint: 'https://node1.example:8080',
		source: 'self',
		is_self: true,
		scheme: 'https',
		reachable: true,
		rtt_ms: 4,
		checked_at: null,
		files: 10,
		bytes: 1000,
		pieces: 4,
		disk_free_bytes: null,
		disk_total_bytes: null
	},
	{
		instance_id: 'inst-2',
		endpoint: 'https://node2.example:8080',
		source: 'joined',
		is_self: false,
		scheme: 'https',
		reachable: false,
		rtt_ms: null,
		checked_at: null,
		files: 0,
		bytes: 0,
		pieces: 0,
		disk_free_bytes: null,
		disk_total_bytes: null
	}
];

function nodeElements(container: HTMLElement): SVGGElement[] {
	return Array.from(container.querySelectorAll<SVGGElement>('g.node'));
}

function nodeByName(container: HTMLElement, name: string): SVGGElement {
	return nodeElements(container).find((element) =>
		element.getAttribute('aria-label')?.startsWith(name)
	) as SVGGElement;
}

describe('TopologyMap', () => {
	it('renders one node per member and per server without a member, named with the visible name', () => {
		const { container } = render(TopologyMap, { props: { members, servers, selected: null } });
		const nodes = nodeElements(container);
		expect(nodes).toHaveLength(3);
		expect(nodeByName(container, 'callfs-node-2').getAttribute('aria-label')).toContain('callfs-node-2');
		expect(nodeByName(container, 'callfs-node-2').getAttribute('tabindex')).toBe('-1');
		expect(nodes[0]?.getAttribute('tabindex')).toBe('0');
	});

	it('moves focus with arrow keys and wraps with Home and End', async () => {
		const { container } = render(TopologyMap, { props: { members, servers, selected: null } });
		const nodes = nodeElements(container);
		nodes[0]?.focus();
		await fireEvent.keyDown(nodes[0] as SVGGElement, { key: 'ArrowRight' });
		expect(nodes[1]).toHaveFocus();
		await fireEvent.keyDown(nodes[1] as SVGGElement, { key: 'ArrowLeft' });
		expect(nodes[0]).toHaveFocus();
		await fireEvent.keyDown(nodes[0] as SVGGElement, { key: 'ArrowLeft' });
		expect(nodes[2]).toHaveFocus();
		await fireEvent.keyDown(nodes[2] as SVGGElement, { key: 'Home' });
		expect(nodes[0]).toHaveFocus();
		await fireEvent.keyDown(nodes[0] as SVGGElement, { key: 'End' });
		expect(nodes[2]).toHaveFocus();
	});

	it('opens the focused node on Enter with its prefixed id', async () => {
		const onopen = vi.fn();
		const { container } = render(TopologyMap, { props: { members, servers, selected: null, onopen } });
		await fireEvent.keyDown(nodeByName(container, 'callfs-node-1'), { key: 'Enter' });
		expect(onopen).toHaveBeenCalledWith('member:callfs-node-1');
	});

	it('opens a node on click', async () => {
		const onopen = vi.fn();
		const { container } = render(TopologyMap, { props: { members, servers, selected: null, onopen } });
		await fireEvent.click(nodeByName(container, 'callfs-node-2'));
		expect(onopen).toHaveBeenCalledWith('member:callfs-node-2');
	});

	it('adds a node only for servers that no member runs on', () => {
		const lone: ServerInfo = { ...(servers[1] as ServerInfo), instance_id: 'inst-9', endpoint: 'https://lone.example:8080' };
		const { container } = render(TopologyMap, { props: { members, servers: [...servers, lone], selected: null } });
		expect(nodeElements(container)).toHaveLength(4);
		expect(nodeByName(container, 'https://lone.example:8080')).toBeTruthy();
	});

	it('colors nodes by latency and by lag', () => {
		const latency = render(TopologyMap, {
			props: { members, servers, selected: null, colorBy: 'latency' }
		});
		expect(nodeByName(latency.container, 'callfs-node-1').getAttribute('data-tone')).toBe('ok');
		expect(nodeByName(latency.container, 'callfs-node-2').getAttribute('data-tone')).toBe('warn');
		expect(nodeByName(latency.container, 'callfs-node-3').getAttribute('data-tone')).toBe('neutral');
		latency.unmount();
		const lag = render(TopologyMap, { props: { members, servers, selected: null, colorBy: 'lag' } });
		expect(nodeByName(lag.container, 'callfs-node-1').getAttribute('data-tone')).toBe('ok');
		expect(nodeByName(lag.container, 'callfs-node-2').getAttribute('data-tone')).toBe('warn');
	});

	it('marks the leader and lists every member and server in the table twin', () => {
		const { container } = render(TopologyMap, { props: { members, servers, selected: null } });
		expect(nodeByName(container, 'callfs-node-1').getAttribute('data-leader')).toBe('true');
		const table = screen.getByRole('table');
		expect(within(table).getAllByRole('row')).toHaveLength(4);
		expect(
			within(table).getAllByRole('button', { name: t('topology.open_named', { name: 'callfs-node-3' }) })
		).toHaveLength(1);
		expect(within(table).getAllByText(t('status.member.leader')).length).toBeGreaterThan(0);
	});
});

describe('MemberCard', () => {
	it('shows the node id as heading, state word, role and copyable address', () => {
		render(MemberCard, { props: { member: members[1] as ClusterMember } });
		expect(screen.getByRole('heading', { name: 'callfs-node-2' })).toBeInTheDocument();
		expect(screen.getByText(t('status.member.follower'))).toBeInTheDocument();
		expect(screen.getByText('https://node2.example:8080')).toBeInTheDocument();
		expect(screen.getAllByRole('button', { name: new RegExp(t('common.copy')) })).toHaveLength(1);
	});

	it('shows unknown last seen and the offline word when the member never reported', () => {
		render(MemberCard, { props: { member: members[2] as ClusterMember } });
		expect(screen.getAllByText(t('common.unknown')).length).toBeGreaterThanOrEqual(2);
		expect(screen.getByText(t('members.online.no'))).toBeInTheDocument();
	});
});

describe('PieceMap', () => {
	const pieces: Piece[] = [
		{ index: 1, kind: 'data', server: 'callfs-node-1', state: 'piece.ok' },
		{ index: 2, kind: 'spare', server: 'callfs-node-2', state: 'piece.missing' }
	];

	it('renders one square per piece with kind and state in its name, grouped by server', () => {
		const { container } = render(PieceMap, {
			props: { pieces, servers: ['callfs-node-1', 'callfs-node-2'] }
		});
		const squares = container.querySelectorAll<HTMLElement>('.piece');
		expect(squares).toHaveLength(2);
		const spare = container.querySelector<HTMLElement>('.piece[data-kind="spare"]');
		expect(spare?.getAttribute('aria-label')).toContain(`${t('durability.piece.col.piece')} 2`);
		expect(spare?.getAttribute('aria-label')).toContain(t('status.piece.missing'));
		expect(spare?.getAttribute('data-tone')).toBe('danger');
	});

	it('shows an empty line for a server without pieces and a table alternative', () => {
		render(PieceMap, { props: { pieces, servers: ['callfs-node-1', 'callfs-node-3'] } });
		expect(screen.getByText(t('topology.piece.none'))).toBeInTheDocument();
		const table = screen.getByRole('table');
		expect(within(table).getAllByRole('row')).toHaveLength(3);
		expect(within(table).getByText(t('status.piece.ok'))).toBeInTheDocument();
	});
});
