<script lang="ts" module>
	const ARIA_SORT = { asc: 'ascending', desc: 'descending' } as const;
	const SORT_ICON = { asc: 'sort-asc', desc: 'sort-desc' } as const;
</script>

<script lang="ts">
	import type { HTMLThAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';
	import { announce } from '../quarks/announce';
	import type { SortState } from '../types';

	interface Props extends Omit<HTMLThAttributes, 'children'> {
		label: string;
		sortKey: string;
		sort?: SortState | null;
		onsort: (next: SortState | null) => void;
		help?: string;
		as?: 'th' | 'span';
		class?: string;
	}

	let { label, sortKey, sort = null, onsort, help, as = 'th', class: className, ...rest }: Props = $props();

	const direction = $derived(sort?.key === sortKey ? sort.dir : null);
	const ariaSort = $derived(null === direction ? 'none' : ARIA_SORT[direction]);
	const icon = $derived(null === direction ? 'chevrons-up-down' : SORT_ICON[direction]);

	function nextSort(): SortState | null {
		if (null === direction) {
			return { key: sortKey, dir: 'asc' };
		}
		if ('asc' === direction) {
			return { key: sortKey, dir: 'desc' };
		}
		return null;
	}

	function activate(): void {
		const next = nextSort();
		onsort(next);
		if (null === next) {
			announce(t('tablecontrols.sort.cleared', { label }), 'polite');
			return;
		}
		const word = 'asc' === next.dir ? t('tablecontrols.sort.ascending') : t('tablecontrols.sort.descending');
		announce(t('tablecontrols.sort.announce', { label, direction: word }), 'polite');
	}
</script>

{#snippet trigger()}
	<button type="button" class="sort-trigger" data-active={null === direction ? undefined : 'true'} onclick={activate}>
		<span class="sort-label">{label}</span>
		<Icon name={icon} size="sm" />
	</button>
{/snippet}

<svelte:element
	this={as}
	scope={'th' === as ? 'col' : undefined}
	aria-sort={'th' === as ? ariaSort : undefined}
	class={['sort-header', className]}
	{...rest}
>
	{#if help}
		<Tooltip content={help}>{@render trigger()}</Tooltip>
	{:else}
		{@render trigger()}
	{/if}
</svelte:element>

<style>
	.sort-header {
		color: var(--sort-header-fg, var(--text-muted));
		text-align: start;
	}

	.sort-trigger {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		min-block-size: var(--target-min);
		padding: 0;
		border: 0;
		border-radius: var(--radius-xs);
		background: transparent;
		color: inherit;
		font: inherit;
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	.sort-trigger :global(svg) {
		color: var(--text-faint);
	}

	.sort-trigger[data-active='true'] {
		color: var(--text);
	}

	.sort-trigger[data-active='true'] :global(svg) {
		color: var(--accent-text);
	}

	@media (hover: hover) {
		.sort-trigger:hover {
			color: var(--text);
		}
	}

	.sort-trigger:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.sort-label {
		white-space: nowrap;
	}
</style>
