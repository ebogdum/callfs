<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Code from '../atoms/Code.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

<span class={['code-cell', className]} title={value} {...rest}>
	<Code>{value}</Code>
</span>

<style>
	.code-cell {
		display: block;
		max-inline-size: 100%;
		min-inline-size: 0;
		overflow: hidden;
		font-size: var(--table-text);
		white-space: nowrap;
		color: var(--code-cell-fg, var(--text));
	}
</style>
