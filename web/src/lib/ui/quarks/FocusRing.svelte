<script lang="ts" module>
	export const TOKENS = [] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		children: Snippet;
		inset?: boolean;
		class?: string;
	}

	let { children, inset = false, class: className = '', ...rest }: Props = $props();
</script>

<span class="focus-ring {className}" class:inset {...rest}>{@render children()}</span>

<style>
	.focus-ring {
		display: block;
		border-radius: var(--radius-sm);
	}

	.focus-ring:has(:focus-visible) {
		box-shadow: var(--focus-ring);
	}

	.focus-ring.inset:has(:focus-visible) {
		box-shadow: none;
		outline: var(--hairline) solid var(--focus-ring-color);
		outline-offset: calc(-1 * var(--hairline));
	}
</style>
