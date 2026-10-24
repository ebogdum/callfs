<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		children: Snippet;
		class?: string;
	}

	let { children, class: className, ...rest }: Props = $props();
</script>

<div class={['dock', className]} {...rest}>{@render children()}</div>

<style>
	.dock {
		position: fixed;
		inset-inline: 0;
		inset-block-end: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		max-block-size: 50dvh;
		overflow-y: auto;
		padding-block: var(--space-3);
		padding-inline: var(--gutter);
		background: var(--surface-raised);
		box-shadow: var(--shadow-md);
		z-index: var(--z-dock);
	}

	.dock:empty {
		display: none;
	}
</style>
