<script lang="ts" module>
	export type NumStyle = 'int' | 'compact' | 'decimal';
	export const TOKENS = ['--num-color', '--num-weight'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { DASH, defaultLocale, formatFixed, formatNumber } from '$lib/format/number';
	import { t } from '$lib/copy/t';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: number | null;
		style?: NumStyle;
		precision?: number;
		unit?: string;
	}

	let { value, style = 'int', precision, unit, class: className, ...rest }: Props = $props();

	const isNumber = $derived(Number.isFinite(value));
	const body = $derived.by(() => {
		if (false === isNumber || null === value) {
			return DASH;
		}
		if ('compact' !== style && undefined !== precision) {
			return formatFixed(value, precision, defaultLocale());
		}
		return formatNumber(value, style);
	});
	const text = $derived(undefined === unit || false === isNumber ? body : `${body} ${unit}`);
	const exact = $derived(isNumber && null !== value && 'compact' === style ? formatNumber(value, 'int') : undefined);
</script>

<span class={['num', className]} title={exact} {...rest}>
	{text}{#if false === isNumber}<VisuallyHidden>{t('common.unknown')}</VisuallyHidden>{/if}
</span>

<style>
	.num {
		color: var(--num-color, inherit);
		font-weight: var(--num-weight, inherit);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
</style>
