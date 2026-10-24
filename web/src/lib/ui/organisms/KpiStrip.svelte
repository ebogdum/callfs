<script lang="ts" module>
	export const TOKENS = ['--kpi-strip-gap'] as const;
</script>

<script lang="ts">
	import type { ComponentProps } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import StatTile from '../molecules/StatTile.svelte';

	type StatTileProps = ComponentProps<typeof StatTile>;

	interface Props extends Omit<HTMLAttributes<HTMLUListElement>, 'class'> {
		tiles: StatTileProps[];
		label?: string;
		class?: string;
	}

	let { tiles, label = t('findings.kpi.label'), class: className, ...rest }: Props = $props();

	const MAX_COLUMNS = 4;
	const columns = $derived(Math.max(1, Math.min(MAX_COLUMNS, tiles.length)));
</script>

<ul {...rest} class={['kpi-strip', className]} aria-label={label} style:--kpi-count={columns}>
	{#each tiles as tile}
		<li class="tile">
			<StatTile {...tile} />
		</li>
	{/each}
</ul>

<style>
	.kpi-strip {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--kpi-strip-gap, var(--space-4));
		margin: 0;
		padding: 0;
		list-style: none;
	}

	@media (min-width: 1024px) {
		.kpi-strip {
			grid-template-columns: repeat(var(--kpi-count, 4), minmax(0, 1fr));
		}
	}

	.tile {
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
	}

	.tile :global(.stat-tile) {
		flex: 1;
	}
</style>
