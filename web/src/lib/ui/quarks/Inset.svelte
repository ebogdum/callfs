<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { GapToken } from '../types';

	interface Props extends HTMLAttributes<HTMLElement> {
		pad: GapToken;
		padInline?: GapToken;
		padBlock?: GapToken;
		children: Snippet;
	}

	let { pad, padInline, padBlock, class: className, children, ...rest }: Props = $props();
</script>

<div
	class={['inset', className]}
	style:--inset-block={`var(--space-${padBlock ?? pad})`}
	style:--inset-inline={`var(--space-${padInline ?? pad})`}
	{...rest}
>
	{@render children()}
</div>

<style>
	.inset {
		padding-block: var(--inset-block, var(--space-4));
		padding-inline: var(--inset-inline, var(--space-4));
	}
</style>
