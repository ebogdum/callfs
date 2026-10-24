<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { GapToken } from '../types';

	interface Props extends HTMLAttributes<HTMLElement> {
		min?: '8' | '12' | '16' | '20' | '24';
		gap?: GapToken;
		cols?: number;
		children: Snippet;
	}

	let { min = '16', gap = '4', cols, class: className, children, ...rest }: Props = $props();

	const columns = $derived(
		undefined === cols
			? `repeat(auto-fill, minmax(min(100%, ${min}rem), 1fr))`
			: `repeat(${cols}, minmax(0, 1fr))`
	);
</script>

<div
	class={['grid', className]}
	style:--grid-columns={columns}
	style:--grid-gap={`var(--space-${gap})`}
	{...rest}
>
	{@render children()}
</div>

<style>
	.grid {
		display: grid;
		grid-template-columns: var(--grid-columns);
		gap: var(--grid-gap, var(--space-4));
	}
</style>
