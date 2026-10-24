<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Duration from '../atoms/Duration.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: number | string;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

<span class={['duration-cell', className]} {...rest}>
	<Duration {value} style="short" />
</span>

<style>
	.duration-cell {
		display: block;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
		text-align: end;
		white-space: nowrap;
		color: var(--duration-cell-color, inherit);
	}
</style>
