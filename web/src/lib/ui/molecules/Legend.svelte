<script lang="ts" module>
	export interface LegendSeries {
		id: string;
		label: string;
		colorIndex: number;
		dashed?: boolean;
		hidden?: boolean;
	}

	export const TOKENS = ['--legend-gap', '--legend-entry-h', '--legend-hover'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class' | 'ontoggle'> {
		series: LegendSeries[];
		ontoggle?: (id: string) => void;
		class?: string;
	}

	const VIZ_SLOT_COUNT = 8;

	let { series, ontoggle, class: className, ...rest }: Props = $props();

	const colorFor = (index: number): string => `var(--viz-${(Math.max(0, index) % VIZ_SLOT_COUNT) + 1})`;
</script>

<Cluster gap="1" class={['legend', className]} role="group" aria-label={t('u077molecules.legend_label')} {...rest}>
	{#each series as entry (entry.id)}
		<button
			type="button"
			class="entry"
			data-state={entry.hidden ? 'hidden' : 'shown'}
			aria-pressed={!entry.hidden}
			onclick={() => ontoggle?.(entry.id)}
		>
			<span class="sample" data-dashed={true === entry.dashed} style:--legend-color={colorFor(entry.colorIndex)}></span>
			<span class="label">{entry.label}</span>
		</button>
	{/each}
</Cluster>

<style>
	.entry {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1-5);
		min-block-size: var(--legend-entry-h, var(--target-min));
		padding-inline: var(--space-1-5);
		border: 0;
		border-radius: var(--radius-sm);
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		cursor: pointer;
	}

	@media (hover: hover) {
		.entry:hover {
			background: var(--legend-hover, var(--surface-hover));
		}
	}

	.entry:active {
		background: var(--surface-pressed);
	}

	.entry:focus-visible {
		box-shadow: var(--focus-ring);
		outline: var(--focus-width) solid transparent;
	}

	.entry[data-state='hidden'] {
		color: var(--text-faint);
		text-decoration: line-through;
	}

	.sample {
		flex: none;
		display: inline-block;
		inline-size: var(--space-3);
		block-size: 0;
		border-block-start: var(--space-0-5) solid var(--legend-color, currentColor);
		border-radius: var(--radius-full);
	}

	.sample[data-dashed='true'] {
		border-block-start-style: dashed;
	}

	.entry[data-state='hidden'] .sample {
		border-block-start-color: var(--text-faint);
	}
</style>
