<script lang="ts" module>
	export type PercentStyle = 'count-first' | 'rate-only';
	export const TOKENS = ['--percent-rate-color', '--percent-gap'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatNumber } from '$lib/format/number';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		num: number;
		den: number;
		style?: PercentStyle;
	}

	let { num, den, style = 'count-first', class: className, ...rest }: Props = $props();

	const hasBase = $derived(Number.isFinite(den) && 0 < den);
	const rate = $derived(hasBase ? Math.min(100, Math.round((100 * num) / den)) : null);
	const count = $derived(hasBase ? `${formatNumber(num, 'int')} ${t('number.of')} ${formatNumber(den, 'int')}` : formatNumber(num, 'int'));
</script>

<span class={['percent', className]} data-style={style} {...rest}>
	{#if 'count-first' === style}<span class="count">{count}</span>{/if}
	{#if null !== rate}
		<span class="rate">({rate}%)</span>
	{:else if 'rate-only' === style}
		<span class="rate">{count}</span>
	{/if}
</span>

<style>
	.percent {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--percent-gap, var(--space-1));
		font-variant-numeric: tabular-nums;
	}

	.rate {
		font-size: var(--text-sm-size);
		color: var(--percent-rate-color, var(--text-muted));
	}
</style>
