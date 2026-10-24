<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { GapToken } from '../types';

	interface Props extends HTMLAttributes<HTMLElement> {
		level?: 'base' | 'raised' | 'sunken' | 'overlay';
		pad?: GapToken | 'card';
		radius?: 'sm' | 'md' | 'lg' | 'xl';
		border?: boolean;
		as?: string;
		children: Snippet;
	}

	let {
		level = 'raised',
		pad,
		radius = 'lg',
		border = true,
		as = 'div',
		class: className,
		children,
		...rest
	}: Props = $props();

	const padding = $derived(
		undefined === pad ? undefined : 'card' === pad ? 'var(--card-pad)' : `var(--space-${pad})`
	);
</script>

<svelte:element
	this={as}
	class={['surface', level, border && 'bordered', className]}
	style:--surface-pad={padding}
	style:--surface-radius={`var(--radius-${radius})`}
	{...rest}
>
	{@render children()}
</svelte:element>

<style>
	.surface {
		display: block;
		min-inline-size: 0;
		padding: var(--surface-pad, 0);
		border-radius: var(--surface-radius, var(--radius-lg));
	}

	.base {
		background: var(--bg);
		box-shadow: none;
	}

	.raised {
		background: var(--surface-raised);
		box-shadow: var(--shadow-xs);
	}

	.sunken {
		background: var(--surface-sunken);
		box-shadow: none;
	}

	.overlay {
		background: var(--surface-overlay);
		box-shadow: var(--shadow-md);
	}

	.bordered {
		border: var(--hairline) solid var(--border-subtle);
	}
</style>
