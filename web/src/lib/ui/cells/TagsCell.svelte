<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Num from '../atoms/Num.svelte';
	import Tag from '../atoms/Tag.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class' | 'children'> {
		values: string[];
		max?: number;
		class?: string;
	}

	let { values, max = 2, class: className, ...rest }: Props = $props();

	const shown = $derived(values.slice(0, max));
	const hidden = $derived(values.slice(max));
	const hiddenList = $derived(hidden.join(', '));
</script>

<span class={['tags-cell', className]} {...rest}>
	{#each shown as value, index (index)}
		<Tag label={value} />
	{/each}
	{#if 0 < hidden.length}
		<Tooltip content={hiddenList}>
			<span class="overflow" tabindex="0">+<Num value={hidden.length} /></span>
		</Tooltip>
	{/if}
</span>

<style>
	.tags-cell {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.overflow {
		border-radius: var(--radius-xs);
		color: var(--text-muted);
		font-size: var(--text-sm-size);
	}

	.overflow:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}
</style>
