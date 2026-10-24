<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { GapToken } from '../types';

	interface Props extends HTMLAttributes<HTMLElement> {
		gap?: GapToken;
		justify?: 'start' | 'end' | 'center' | 'space-between' | 'space-around';
		align?: 'start' | 'center' | 'end' | 'stretch' | 'baseline';
		wrap?: boolean;
		as?: string;
		children: Snippet;
	}

	let {
		gap = '2',
		justify,
		align = 'center',
		wrap = true,
		as = 'div',
		class: className,
		children,
		...rest
	}: Props = $props();
</script>

<svelte:element
	this={as}
	class={['cluster', className]}
	style:--cluster-gap={`var(--space-${gap})`}
	style:--cluster-justify={justify}
	style:--cluster-align={align}
	style:--cluster-wrap={wrap ? 'wrap' : 'nowrap'}
	{...rest}
>
	{@render children()}
</svelte:element>

<style>
	.cluster {
		display: flex;
		flex-direction: row;
		flex-wrap: var(--cluster-wrap, wrap);
		gap: var(--cluster-gap, var(--space-2));
		align-items: var(--cluster-align, center);
		justify-content: var(--cluster-justify, start);
	}
</style>
