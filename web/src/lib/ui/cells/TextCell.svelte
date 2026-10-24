<script lang="ts" module>
	export const TOKENS = ['--text-cell-color'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
		muted?: boolean;
		mono?: boolean;
		wrap?: boolean;
	}

	let { value, muted = false, mono = false, wrap = false, class: className, ...rest }: Props = $props();
</script>

<span class={['text-cell', true === muted && 'muted', true === wrap && 'wrap', true === mono && 'mono', className]} {...rest}>
	{#if wrap}{value}{:else}<MiddleTruncate text={value} {mono} />{/if}
</span>

<style>
	.text-cell {
		display: block;
		min-inline-size: 0;
		font-size: var(--table-text);
		white-space: nowrap;
		color: var(--text-cell-color, var(--text));
	}

	.text-cell.wrap {
		white-space: normal;
		overflow-wrap: anywhere;
		text-wrap: pretty;
		line-height: var(--text-sm-line);
		padding-block: var(--space-2);
	}

	.text-cell.wrap.mono {
		font-family: var(--font-mono);
	}

	.text-cell.muted {
		color: var(--text-muted);
	}
</style>
