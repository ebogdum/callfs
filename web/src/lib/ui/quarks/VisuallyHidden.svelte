<script lang="ts" module>
	export const TOKENS = [] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		children: Snippet;
		focusable?: boolean;
		class?: string;
	}

	let { children, focusable = false, class: className = '', ...rest }: Props = $props();
</script>

<span class="visually-hidden {className}" class:focusable {...rest}>{@render children()}</span>

<style>
	.visually-hidden {
		position: absolute;
		inline-size: var(--hairline);
		block-size: var(--hairline);
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.visually-hidden.focusable:focus-within {
		position: static;
		inline-size: auto;
		block-size: auto;
		overflow: visible;
		clip-path: none;
		white-space: normal;
	}
</style>
