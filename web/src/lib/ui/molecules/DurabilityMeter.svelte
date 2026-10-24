<script lang="ts" module>
	export const TOKENS = ['--durability-piece-data', '--durability-piece-spare', '--durability-piece-missing'] as const;

	export type DurabilityState = 'full' | 'reduced' | 'critical' | 'unreadable';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		data: number;
		spare: number;
		healthy: number;
		class?: string;
	}

	let { data, spare, healthy, class: className, ...rest }: Props = $props();

	const glyphTones = { full: 'ok', reduced: 'warn', critical: 'danger', unreadable: 'danger' } as const;

	const total = $derived(data + spare);
	const up = $derived(Math.min(total, Math.max(0, healthy)));
	const surplus = $derived(up - data);
	const state: DurabilityState = $derived(
		0 > surplus ? 'unreadable' : 0 === surplus ? 'critical' : 0 < total - up ? 'reduced' : 'full'
	);
	const pieces = $derived(
		Array.from({ length: total }, (_, index) => ({
			kind: index < data ? 'data' : 'spare',
			up: index < up
		}))
	);

	function sentenceFor(current: DurabilityState): string {
		const params = { healthy: up, total, count: surplus };
		if ('unreadable' === current) {
			return t('protection.durability.unreadable', params);
		}
		if ('critical' === current) {
			return t('protection.durability.at_risk', params);
		}
		return t('protection.durability.can_lose', params);
	}

	const sentence = $derived(sentenceFor(state));
</script>

<div class={['durability-meter', className]} data-state={state} {...rest}>
	<Stack gap="2">
		<Cluster gap="1" class="durability-pieces" aria-hidden="true" as="ul">
			{#each pieces as piece, index (index)}
				<li class="piece" data-kind={piece.kind} data-up={piece.up}></li>
			{/each}
		</Cluster>
		<Cluster gap="2" align="start">
			<StatusGlyph tone={glyphTones[state]} />
			<p class="sentence">{sentence}</p>
		</Cluster>
		<Cluster gap="3" class="durability-legend" as="ul">
			<li class="legend-item"><span class="swatch" data-kind="data"></span>{t('protection.durability.piece_data')}</li>
			<li class="legend-item"><span class="swatch" data-kind="spare"></span>{t('protection.durability.piece_spare')}</li>
			<li class="legend-item"><span class="swatch" data-up="true"></span>{t('protection.durability.piece_ok')}</li>
			<li class="legend-item"><span class="swatch" data-up="false"></span>{t('protection.durability.piece_missing')}</li>
		</Cluster>
	</Stack>
</div>

<style>
	.durability-meter {
		min-inline-size: 0;
	}

	:global(.durability-pieces),
	:global(.durability-legend),
	:global(.durability-pieces) li,
	:global(.durability-legend) li {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	:global(.durability-pieces) {
		display: flex;
		align-items: stretch;
		inline-size: 100%;
	}

	.piece {
		flex: 1 1 0;
		block-size: var(--space-4);
		border: var(--hairline) solid var(--durability-piece-data, var(--accent));
		border-radius: var(--radius-xs);
	}

	.piece[data-kind='data'][data-up='true'] {
		background: var(--durability-piece-data, var(--accent));
	}

	.piece[data-kind='spare'][data-up='true'] {
		border-color: var(--durability-piece-spare, var(--accent));
		background: repeating-linear-gradient(
			45deg,
			var(--durability-piece-spare, var(--accent)) 0 var(--space-1),
			transparent var(--space-1) calc(var(--space-1) * 2)
		);
	}

	.piece[data-up='false'] {
		border-color: var(--durability-piece-missing, var(--danger));
		border-style: dashed;
	}

	.sentence {
		margin: 0;
		color: var(--text);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
		text-wrap: pretty;
	}

	.legend-item {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1-5);
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.swatch {
		display: inline-block;
		inline-size: var(--space-3);
		block-size: var(--space-3);
		border: var(--hairline) solid var(--durability-piece-data, var(--accent));
		border-radius: var(--radius-xs);
	}

	.swatch[data-kind='data'] {
		background: var(--durability-piece-data, var(--accent));
	}

	.swatch[data-kind='spare'] {
		background: repeating-linear-gradient(
			45deg,
			var(--durability-piece-spare, var(--accent)) 0 var(--space-1),
			transparent var(--space-1) calc(var(--space-1) * 2)
		);
	}

	.swatch[data-up='false'] {
		border-color: var(--durability-piece-missing, var(--danger));
		border-style: dashed;
		background: transparent;
	}
</style>
