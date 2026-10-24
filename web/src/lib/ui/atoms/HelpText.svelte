<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLParagraphElement> {
		id: string;
		children: Snippet;
	}

	let { id, class: className, children, ...rest }: Props = $props();
</script>

<p {id} class={['help', className]} {...rest}>{@render children()}</p>

<style>
	.help {
		color: var(--help-fg, var(--text-muted));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		letter-spacing: var(--text-sm-tracking);
		overflow-wrap: anywhere;
	}
</style>
