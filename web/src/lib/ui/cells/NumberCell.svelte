<script lang="ts" module>
	export const TOKENS = ['--number-cell-color'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Num from '../atoms/Num.svelte';
	import type { NumStyle } from '../atoms/Num.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'style'> {
		value: number | null;
		style?: NumStyle;
		unit?: string;
	}

	let { value, style, unit, class: className, ...rest }: Props = $props();
</script>

<span class={['number-cell', className]} {...rest}>
	<Num {value} {style} {unit} />
</span>

<style>
	.number-cell {
		display: block;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
		text-align: end;
		white-space: nowrap;
		color: var(--number-cell-color, inherit);
	}
</style>
