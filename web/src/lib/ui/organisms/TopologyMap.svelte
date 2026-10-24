<script lang="ts" module>
	export type TopologyColorBy = 'state' | 'latency' | 'lag' | 'disk';
	export const TOKENS = ['--topology-link', '--topology-node-fill'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { ClusterMember, ServerInfo } from '$lib/api/types';
	import { STATUS, type StatusKey } from '$lib/status/catalog';
	import { t } from '$lib/copy/t';
	import type { Tone } from '../types';
	import Button from '../atoms/Button.svelte';
	import DurationCell from '../cells/DurationCell.svelte';
	import NumberCell from '../cells/NumberCell.svelte';
	import StatusCell from '../cells/StatusCell.svelte';
	import TextCell from '../cells/TextCell.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import Stack from '../quarks/Stack.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		members: ClusterMember[];
		servers: ServerInfo[];
		colorBy?: TopologyColorBy;
		selected?: string | null;
		onopen?: (id: string) => void;
		class?: string;
	}

	let {
		members,
		servers,
		colorBy = 'state',
		selected = $bindable(null),
		onopen,
		class: className,
		...rest
	}: Props = $props();

	interface TopologyNode {
		id: string;
		kind: 'member' | 'server';
		name: string;
		status: StatusKey;
		word: string;
		tone: Tone;
		glyph: string;
		leader: boolean;
		instanceId: string | null;
		role: string;
		serverAddress: string;
		lastSeenMs: number | null;
		behindBy: number | null;
		x: number;
		y: number;
		below: boolean;
	}

	interface TopologyLink {
		key: string;
		kind: 'forward';
		x1: number;
		y1: number;
		x2: number;
		y2: number;
	}

	const VIEW_SIZE = 480;
	const CENTER = 240;
	const RING_RADIUS = 150;
	const NODE_RADIUS = 24;
	const LABEL_GAP = 20;
	const LABEL_LINE = 14;
	const CROWN_OFFSET = 8;

	const MEMBER_STATUS: Record<ClusterMember['state'], StatusKey> = {
		leader: 'member.leader',
		follower: 'member.follower',
		offline: 'member.unreachable'
	};

	const GLYPHS: Record<Tone, string> = {
		neutral: '○',
		accent: '◆',
		ok: '●',
		warn: '▲',
		danger: '■',
		info: '◇'
	};

	function serverStatus(server: ServerInfo): StatusKey {
		if (server.is_self) {
			return 'server.self';
		}
		if (true === server.reachable) {
			return 'server.up';
		}
		if (false === server.reachable) {
			return 'server.down';
		}
		return 'server.unknown';
	}

	function toneOf(status: StatusKey): Tone {
		const tone = STATUS[status].tone;
		return 'progress' === tone || 'muted' === tone ? 'neutral' : tone;
	}

	function latencyTone(milliseconds: number | null): Tone {
		if (null === milliseconds) {
			return 'neutral';
		}
		if (500 >= milliseconds) {
			return 'ok';
		}
		if (2000 >= milliseconds) {
			return 'warn';
		}
		return 'danger';
	}

	function lagTone(behindBy: number | null): Tone {
		if (null === behindBy) {
			return 'neutral';
		}
		if (0 === behindBy) {
			return 'ok';
		}
		if (100 >= behindBy) {
			return 'warn';
		}
		return 'danger';
	}

	const DISK_USED_WARN = 0.8;
	const DISK_USED_DANGER = 0.9;

	function diskTone(usedShare: number | null): Tone {
		if (null === usedShare) {
			return 'neutral';
		}
		if (DISK_USED_WARN > usedShare) {
			return 'ok';
		}
		if (DISK_USED_DANGER > usedShare) {
			return 'warn';
		}
		return 'danger';
	}

	function diskUsedShareOf(instanceId: string | null): number | null {
		const server = servers.find((candidate) => candidate.instance_id === instanceId);
		if (undefined === server || null === server.disk_total_bytes || null === server.disk_free_bytes || 0 >= server.disk_total_bytes) {
			return null;
		}
		return 1 - server.disk_free_bytes / server.disk_total_bytes;
	}

	function nodeTone(
		mode: TopologyColorBy,
		status: StatusKey,
		latencyMs: number | null,
		behindBy: number | null,
		diskUsedShare: number | null
	): Tone {
		if ('latency' === mode) {
			return latencyTone(latencyMs);
		}
		if ('lag' === mode) {
			return lagTone(behindBy);
		}
		if ('disk' === mode) {
			return diskTone(diskUsedShare);
		}
		return toneOf(status);
	}

	function ringPoint(index: number, count: number): { x: number; y: number; below: boolean } {
		if (1 === count) {
			return { x: CENTER, y: CENTER, below: true };
		}
		const angle = -Math.PI / 2 + (2 * Math.PI * index) / count;
		const y = CENTER + RING_RADIUS * Math.sin(angle);
		return {
			x: CENTER + RING_RADIUS * Math.cos(angle),
			y,
			below: CENTER <= y
		};
	}

	const nodes = $derived.by((): TopologyNode[] => {
		const memberInstances = new Set(members.flatMap((member) => (null === member.instance_id ? [] : [member.instance_id])));
		const loneServers = servers.filter((server) => false === memberInstances.has(server.instance_id));
		const total = members.length + loneServers.length;
		const memberNodes = members.map((member, index): TopologyNode => {
			const point = ringPoint(index, total);
			const status = MEMBER_STATUS[member.state];
			const tone = nodeTone(colorBy, status, member.last_seen_ms, member.behind_by, diskUsedShareOf(member.instance_id));
			const leader = 'leader' === member.state;
			const server = servers.find((candidate) => candidate.instance_id === member.instance_id);
			return {
				id: `member:${member.node_id}`,
				kind: 'member',
				name: member.node_id,
				status,
				word: t(STATUS[status].word),
				tone,
				glyph: GLYPHS[tone],
				leader,
				instanceId: member.instance_id,
				role: leader ? t('status.member.leader') : t('topology.member.role_member'),
				serverAddress: member.api_endpoint ?? server?.endpoint ?? '',
				lastSeenMs: member.last_seen_ms,
				behindBy: member.behind_by,
				x: point.x,
				y: point.y,
				below: point.below
			};
		});
		const serverNodes = loneServers.map((server, index): TopologyNode => {
			const point = ringPoint(members.length + index, total);
			const status = serverStatus(server);
			const tone = nodeTone(colorBy, status, server.rtt_ms, null, diskUsedShareOf(server.instance_id));
			return {
				id: `server:${server.instance_id}`,
				kind: 'server',
				name: server.endpoint,
				status,
				word: t(STATUS[status].word),
				tone,
				glyph: GLYPHS[tone],
				leader: false,
				instanceId: server.instance_id,
				role: t('topology.role.server'),
				serverAddress: server.endpoint,
				lastSeenMs: null,
				behindBy: null,
				x: point.x,
				y: point.y,
				below: point.below
			};
		});
		return [...memberNodes, ...serverNodes];
	});

	const links = $derived.by((): TopologyLink[] => {
		const leaderNode = nodes.find((node) => node.leader);
		if (undefined === leaderNode) {
			return [];
		}
		return nodes
			.filter((node) => 'member' === node.kind && false === node.leader)
			.map((node) => ({
				key: `forward:${node.id}`,
				kind: 'forward',
				x1: node.x,
				y1: node.y,
				x2: leaderNode.x,
				y2: leaderNode.y
			}));
	});

	const leaderName = $derived(nodes.find((node) => node.leader)?.name ?? t('common.unknown'));
	const mapSummary = $derived(
		t('topology.map.summary', {
			members: members.length,
			servers: servers.length,
			leader: leaderName
		})
	);
	const activeIndex = $derived.by(() => {
		const found = nodes.findIndex((node) => node.id === selected);
		return 0 > found ? 0 : found;
	});

	function labelY(node: TopologyNode, line: 0 | 1): number {
		if (node.below) {
			return node.y + NODE_RADIUS + LABEL_GAP + line * LABEL_LINE;
		}
		return node.y - NODE_RADIUS - LABEL_GAP - (1 - line) * LABEL_LINE;
	}

	let nodeEls: (SVGGElement | undefined)[] = $state([]);

	function openNode(id: string): void {
		selected = id;
		onopen?.(id);
	}

	function moveTo(index: number): void {
		const target = nodes[index];
		if (undefined === target) {
			return;
		}
		selected = target.id;
		nodeEls[index]?.focus();
	}

	function handleNodeKeydown(event: KeyboardEvent, index: number): void {
		const last = nodes.length - 1;
		let target: number | null = null;
		if ('ArrowRight' === event.key || 'ArrowDown' === event.key) {
			target = last > index ? index + 1 : 0;
		} else if ('ArrowLeft' === event.key || 'ArrowUp' === event.key) {
			target = 0 < index ? index - 1 : last;
		} else if ('Home' === event.key) {
			target = 0;
		} else if ('End' === event.key) {
			target = last;
		} else if ('Enter' === event.key) {
			event.preventDefault();
			const node = nodes[index];
			if (undefined !== node) {
				openNode(node.id);
			}
			return;
		}
		if (null === target) {
			return;
		}
		event.preventDefault();
		moveTo(target);
	}
</script>

<Stack gap="4" class={['topology-map', className]} {...rest}>
	<svg
		class="map"
		viewBox={`0 0 ${VIEW_SIZE} ${VIEW_SIZE}`}
		role="group"
		aria-label={`${t('topology.map.label')}. ${mapSummary}`}
	>
		{#each links as link (link.key)}
			<line
				class="link"
				data-kind={link.kind}
				x1={link.x1}
				y1={link.y1}
				x2={link.x2}
				y2={link.y2}
			/>
		{/each}
		{#each nodes as node, index (node.id)}
			<g
				class="node"
				data-kind={node.kind}
				data-tone={node.tone}
				data-leader={node.leader}
				role="button"
				tabindex={index === activeIndex ? 0 : -1}
				aria-label={`${node.name}. ${node.word}`}
				aria-current={node.id === selected ? 'true' : undefined}
				bind:this={nodeEls[index]}
				onclick={() => openNode(node.id)}
				onkeydown={(event) => handleNodeKeydown(event, index)}
			>
				<circle class="shape" cx={node.x} cy={node.y} r={NODE_RADIUS} />
				<text class="glyph" x={node.x} y={node.y} aria-hidden="true">{node.glyph}</text>
				{#if node.leader}
					<text class="crown" x={node.x + NODE_RADIUS} y={node.y - NODE_RADIUS + CROWN_OFFSET} aria-hidden="true"
						>♛</text
					>
				{/if}
				<text class="name" x={node.x} y={labelY(node, 0)}>{node.name}</text>
				<text class="word" x={node.x} y={labelY(node, 1)}>{node.word}</text>
			</g>
		{/each}
	</svg>
	<Scroll axis="x" label={t('topology.table.label')}>
		<table class="twin">
			<caption>
				<VisuallyHidden>{t('topology.table.label')}</VisuallyHidden>
			</caption>
			<thead>
				<tr>
					<th scope="col">{t('topology.table.name')}</th>
					<th scope="col">{t('topology.table.role')}</th>
					<th scope="col">{t('topology.table.state')}</th>
					<th scope="col">{t('topology.table.server_address')}</th>
					<th scope="col">{t('topology.table.last_seen')}</th>
					<th scope="col">{t('topology.table.behind')}</th>
					<th scope="col">{t('topology.table.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each nodes as node (node.id)}
					<tr data-kind={node.kind} data-selected={node.id === selected}>
						<td><TextCell value={node.name} mono /></td>
						<td><TextCell value={node.role} /></td>
						<td><StatusCell status={node.status} /></td>
						<td><TextCell value={node.serverAddress} mono wrap /></td>
						<td>
							{#if null === node.lastSeenMs}
								<TextCell value={t('common.unknown')} muted />
							{:else}
								<DurationCell value={node.lastSeenMs} />
							{/if}
						</td>
						<td><NumberCell value={node.behindBy} /></td>
						<td>
							<Button
								variant="ghost"
								size="sm"
								aria-label={t('topology.open_named', { name: node.name })}
								onclick={() => openNode(node.id)}>{t('topology.open')}</Button
							>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</Scroll>
</Stack>

<style>
	.map {
		display: block;
		inline-size: 100%;
		max-inline-size: min(100%, calc(var(--space-4) * 34));
		margin-inline: auto;
		block-size: auto;
	}

	.link {
		stroke: var(--topology-link, var(--border-strong));
		stroke-width: var(--focus-width);
	}

	.link[data-kind='forward'] {
		stroke-dasharray: var(--focus-width) var(--focus-width);
	}

	.node {
		cursor: pointer;
		--topology-node-tone: var(--border-strong);
	}

	.node[data-tone='accent'] {
		--topology-node-tone: var(--accent-text);
	}

	.node[data-tone='ok'] {
		--topology-node-tone: var(--ok-text);
	}

	.node[data-tone='warn'] {
		--topology-node-tone: var(--warn-text);
	}

	.node[data-tone='danger'] {
		--topology-node-tone: var(--danger-text);
	}

	.node[data-tone='info'] {
		--topology-node-tone: var(--info-text);
	}

	.shape {
		fill: var(--topology-node-fill, var(--surface-raised));
		stroke: var(--topology-node-tone);
		stroke-width: var(--focus-width);
	}

	.node[aria-current='true'] .shape {
		fill: var(--surface-selected);
	}

	@media (hover: hover) {
		.node:hover .shape {
			fill: var(--surface-hover);
		}
	}

	.node:focus-visible .shape {
		stroke: var(--focus-ring-color);
	}

	.glyph,
	.crown,
	.name,
	.word {
		font-family: var(--font-sans);
		font-size: var(--text-caption-size);
		text-anchor: middle;
		dominant-baseline: central;
	}

	.glyph {
		fill: var(--topology-node-tone);
	}

	.crown {
		fill: var(--accent-text);
	}

	.name {
		fill: var(--text);
	}

	.word {
		fill: var(--text-muted);
	}

	.twin {
		inline-size: 100%;
		border-collapse: collapse;
		font-size: var(--table-text);
	}

	.twin th,
	.twin td {
		padding-inline: var(--cell-px);
		padding-block: var(--space-2);
		text-align: start;
		vertical-align: middle;
	}

	.twin th {
		color: var(--text-muted);
		font-weight: var(--text-caption-weight);
	}

	.twin caption {
		text-align: start;
	}

	.twin tr[data-selected='true'] {
		background: var(--surface-selected);
	}
</style>
