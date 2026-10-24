<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Unit } from '../types';
	import Delta, { type DeltaGoodWhen } from '../atoms/Delta.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		value: number;
		unit: Unit;
		goodWhen: DeltaGoodWhen;
		period: string;
		class?: string;
	}

	let { value, unit, goodWhen, period, class: className, ...rest }: Props = $props();
</script>

<Delta {value} {unit} {goodWhen} {period} class={className} {...rest} />
