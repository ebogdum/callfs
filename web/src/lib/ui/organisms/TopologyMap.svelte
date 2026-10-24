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
		address: string;
		serverAddress: string;
		lastContactMs: number | null;
		behindBy: number | null;
		x: number;
		y: number;
	}

	interface TopologyLink {
		key: string;
		kind: 'forward' | 'serves';
		x1: number;
		y1: number;
		x2: number;
		y2: number;
	}

	const VIEW_SIZE = 480;
	const CENTER = 240;
	const MEMBER_RADIUS = 104;
	const SERVER_RADIUS = 200;
	const NODE_RADIUS = 24;
	const LABEL_OFFSET = 40;
	const LABEL_LINE = 14;
	const CROWN_OFFSET = 8;

	const MEMBER_STATUS: Record<ClusterMember['state'], StatusKey> = {
		leader: 'member.leader',
		follower: 'member.follower',
		candidate: 'member.candidate',
		shutdown: 'member.shutdown',
		unreachable: 'member.unreachable'
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

	function nodeTone(
		mode: TopologyColorBy,
		status: StatusKey,
		latencyMs: number | null,
		behindBy: number | null
	): Tone {
		if ('latency' === mode) {
			return latencyTone(latencyMs);
		}
		if ('lag' === mode) {
			return lagTone(behindBy);
		}
		if ('disk' === mode) {
			return 'neutral';
		}
		return toneOf(status);
	}

	function ringPoint(index: number, count: number, radius: number): { x: number; y: number } {
		const angle = -Math.PI / 2 + (2 * Math.PI * index) / count;
		return {
			x: CENTER + radius * Math.cos(angle),
			y: CENTER + radius * Math.sin(angle)
		};
	}

	const nodes = $derived.by((): TopologyNode[] => {
		const memberNodes = members.map((member, index): TopologyNode => {
			const point = ringPoint(index, members.length, MEMBER_RADIUS);
			const status = MEMBER_STATUS[member.state];
			const tone = nodeTone(colorBy, status, member.last_contact_ms, member.behind_by);
			const leader = 'leader' === member.state;
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
				role: leader
					? t('status.member.leader')
					: 'voter' === member.suffrage
						? t('topology.role.voter')
						: t('topology.role.nonvoter'),
				address: member.raft_addr,
				serverAddress: member.api_endpoint,
				lastContactMs: member.last_contact_ms,
				behindBy: member.behind_by,
				x: point.x,
				y: point.y
			};
		});
		const serverNodes = servers.map((server, index): TopologyNode => {
			const point = ringPoint(index, servers.length, SERVER_RADIUS);
			const status = serverStatus(server);
			const tone = nodeTone(colorBy, status, server.rtt_ms, null);
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
				address: server.instance_id,
				serverAddress: server.endpoint,
				lastContactMs: null,
				behindBy: null,
				x: point.x,
				y: point.y
			};
		});
		return [...memberNodes, ...serverNodes];
	});

	const links = $derived.by((): TopologyLink[] => {
		const leaderNode = nodes.find((node) => node.leader);
		const result: TopologyLink[] = [];
		for (const node of nodes) {
			if ('member' !== node.kind) {
				continue;
			}
			if (undefined !== leaderNode && false === node.leader) {
				result.push({
					key: `forward:${node.id}`,
					kind: 'forward',
					x1: node.x,
					y1: node.y,
					x2: leaderNode.x,
					y2: leaderNode.y
				});
			}
			if (null === node.instanceId) {
				continue;
			}
			const server = nodes.find(
				(candidate) => 'server' === candidate.kind && candidate.instanceId === node.instanceId
			);
			if (undefined !== server) {
				result.push({
					key: `serves:${node.id}`,
					kind: 'serves',
					x1: node.x,
					y1: node.y,
					x2: server.x,
					y2: server.y
				});
			}
		}
		return result;
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
					<text class="crown" x={node.x} y={node.y - NODE_RADIUS - CROWN_OFFSET} aria-hidden="true"
						>♛</text
					>
				{/if}
				<text class="name" x={node.x} y={node.y + LABEL_OFFSET}>{node.name}</text>
				<text class="word" x={node.x} y={node.y + LABEL_OFFSET + LABEL_LINE}>{node.word}</text>
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
					<th scope="col">{t('topology.table.address')}</th>
					<th scope="col">{t('topology.table.server_address')}</th>
					<th scope="col">{t('topology.table.last_contact')}</th>
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
						<td><TextCell value={node.address} mono /></td>
						<td><TextCell value={node.serverAddress} mono /></td>
						<td>
							{#if null === node.lastContactMs}
								<TextCell value={t('common.unknown')} muted />
							{:else}
								<DurationCell value={node.lastContactMs} />
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
		max-inline-size: var(--content-max);
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
