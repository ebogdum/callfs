<script lang="ts" module>
	import type { StatusKey } from '$lib/status/catalog';

	export interface Piece {
		index: number;
		kind: 'data' | 'spare';
		server: string;
		state: StatusKey;
	}

	export const TOKENS = ['--piece-map-square', '--piece-map-data-fill', '--piece-map-spare-fill'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { STATUS } from '$lib/status/catalog';
	import { t } from '$lib/copy/t';
	import StatusCell from '../cells/StatusCell.svelte';
	import TextCell from '../cells/TextCell.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';
	import Text from '../atoms/Text.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Scroll from '../quarks/Scroll.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		pieces: Piece[];
		servers: string[];
		class?: string;
	}

	let { pieces, servers, class: className, ...rest }: Props = $props();

	const groups = $derived.by(() => {
		const names = new Set<string>(servers);
		for (const piece of pieces) {
			names.add(piece.server);
		}
		return [...names].map((server) => ({
			server,
			pieces: pieces.filter((piece) => server === piece.server)
		}));
	});

	function kindWord(kind: Piece['kind']): string {
		return 'data' === kind ? t('protection.durability.piece_data') : t('protection.durability.piece_spare');
	}

	function pieceLabel(piece: Piece): string {
		return `${t('durability.piece.col.piece')} ${piece.index}`;
	}
</script>

<Stack gap="4" class={['piece-map', className]} {...rest}>
	{#each groups as group (group.server)}
		<Stack gap="2" class="piece-group">
			<Text weight={600}>{group.server}</Text>
			{#if 0 === group.pieces.length}
				<Text tone="muted">{t('topology.piece.none')}</Text>
			{:else}
				<Cluster gap="2">
					{#each group.pieces as piece (piece.index)}
						<Tooltip content={`${pieceLabel(piece)}, ${kindWord(piece.kind)}, ${t(STATUS[piece.state].word)}`}>
							{#snippet children()}
								<span
									class="piece"
									role="img"
									tabindex="0"
									data-kind={piece.kind}
									data-state={piece.state}
									data-tone={STATUS[piece.state].tone}
									aria-label={`${pieceLabel(piece)}, ${kindWord(piece.kind)}, ${t(STATUS[piece.state].word)}`}
								>
									<StatusGlyph tone={STATUS[piece.state].tone} />
									<span class="index">{piece.index}</span>
								</span>
							{/snippet}
						</Tooltip>
					{/each}
				</Cluster>
			{/if}
		</Stack>
	{/each}
	<Scroll axis="x" label={t('durability.piece.table.label')}>
		<table class="twin">
			<caption>
				<VisuallyHidden>{t('durability.piece.table.label')}</VisuallyHidden>
			</caption>
			<thead>
				<tr>
					<th scope="col">{t('durability.piece.col.piece')}</th>
					<th scope="col">{t('durability.piece.col.kind')}</th>
					<th scope="col">{t('inspector.details.server')}</th>
					<th scope="col">{t('durability.piece.col.state')}</th>
				</tr>
			</thead>
			<tbody>
				{#each pieces as piece (piece.index)}
					<tr>
						<td><TextCell value={pieceLabel(piece)} /></td>
						<td><TextCell value={kindWord(piece.kind)} /></td>
						<td><TextCell value={piece.server} mono /></td>
						<td><StatusCell status={piece.state} /></td>
					</tr>
				{/each}
			</tbody>
		</table>
	</Scroll>
</Stack>

<style>
	.piece {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-1);
		min-inline-size: var(--piece-map-square, var(--control-h));
		block-size: var(--piece-map-square, var(--control-h));
		padding-inline: var(--space-1);
		border: var(--focus-width) solid var(--border-strong);
		border-radius: var(--radius-sm);
		background: var(--surface-raised);
		font-size: var(--text-caption-size);
		font-variant-numeric: tabular-nums;
	}

	.piece[data-kind='spare'] {
		background: repeating-linear-gradient(
			45deg,
			var(--piece-map-spare-fill, var(--surface-sunken)) 0 25%,
			var(--surface-raised) 25% 50%
		);
	}

	.piece[data-kind='data'] {
		background: var(--piece-map-data-fill, var(--accent-soft));
	}

	.piece[data-tone='ok'] {
		border-color: var(--ok);
	}

	.piece[data-tone='warn'] {
		border-color: var(--warn);
	}

	.piece[data-tone='danger'] {
		border-color: var(--danger);
	}

	.piece:focus-visible {
		box-shadow: var(--focus-ring);
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
</style>
