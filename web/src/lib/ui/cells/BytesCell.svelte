<script lang="ts" module>
	export const TOKENS = ['--bytes-cell-color'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Bytes from '../atoms/Bytes.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: number | null;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

<span class={['bytes-cell', className]} {...rest}>
	<Bytes {value} />
</span>

<style>
	.bytes-cell {
		display: block;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
		text-align: end;
		white-space: nowrap;
		color: var(--bytes-cell-color, inherit);
	}
</style>
