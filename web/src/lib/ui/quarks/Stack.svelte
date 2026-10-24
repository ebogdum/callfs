<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { GapToken } from '../types';

	interface Props extends HTMLAttributes<HTMLElement> {
		gap?: GapToken;
		align?: 'start' | 'center' | 'end' | 'stretch' | 'baseline';
		as?: string;
		children: Snippet;
	}

	let { gap = '4', align, as = 'div', class: className, children, ...rest }: Props = $props();
</script>

<svelte:element
	this={as}
	class={['stack', className]}
	style:--stack-gap={`var(--space-${gap})`}
	style:--stack-align={align}
	{...rest}
>
	{@render children()}
</svelte:element>

<style>
	.stack {
		display: flex;
		flex-direction: column;
		gap: var(--stack-gap, var(--space-4));
		align-items: var(--stack-align, stretch);
		min-inline-size: 0;
	}
</style>
