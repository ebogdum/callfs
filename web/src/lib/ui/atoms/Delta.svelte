<script lang="ts" module>
	import { formatBytes } from '$lib/format/bytes';
	import { formatDuration } from '$lib/format/duration';
	import { formatNumber } from '$lib/format/number';
	import type { Unit } from '../types';

	export type DeltaGoodWhen = 'up' | 'down' | 'none';
	export const TOKENS = ['--delta-ok', '--delta-danger', '--delta-neutral', '--delta-gap'] as const;

	type Direction = 'up' | 'down' | 'flat';
	type Tone = 'ok' | 'danger' | 'neutral';

	const DIRECTION_ICONS: Record<Direction, 'arrow-up' | 'arrow-down' | 'minus'> = {
		up: 'arrow-up',
		down: 'arrow-down',
		flat: 'minus',
	};

	const AMOUNT_FORMATTERS: Record<Unit, (magnitude: number) => string> = {
		bytes: (magnitude) => formatBytes(magnitude).text,
		bytes_per_s: (magnitude) => `${formatBytes(magnitude).text}/s`,
		count: (magnitude) => formatNumber(magnitude, 'int'),
		per_s: (magnitude) => `${formatNumber(magnitude, 'int')}/s`,
		ms: (magnitude) => formatDuration(magnitude, 'short'),
		s: (magnitude) => formatDuration(magnitude * 1000, 'short'),
		percent: (magnitude) => `${formatNumber(magnitude, 'int')}%`,
		ratio: (magnitude) => formatNumber(magnitude, 'int'),
		none: (magnitude) => formatNumber(magnitude, 'int'),
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { DASH } from '$lib/format/number';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: number;
		unit: Unit;
		goodWhen: DeltaGoodWhen;
		period: string;
	}

	let { value, unit, goodWhen, period, class: className, ...rest }: Props = $props();

	const isNumber = $derived(Number.isFinite(value));
	const direction: Direction = $derived(false === isNumber || 0 === value ? 'flat' : 0 < value ? 'up' : 'down');
	const isGood = $derived('flat' !== direction && 'none' !== goodWhen && ('up' === goodWhen) === ('up' === direction));
	const tone: Tone = $derived('none' === goodWhen || 'flat' === direction ? 'neutral' : isGood ? 'ok' : 'danger');
	const verdict = $derived('neutral' === tone ? null : 'ok' === tone ? t('delta.better') : t('delta.worse'));
	const sign = $derived('up' === direction ? '+' : 'down' === direction ? '−' : '');
	const amount = $derived(isNumber ? AMOUNT_FORMATTERS[unit](Math.abs(value)) : DASH);
</script>

<span class={['delta', className]} data-tone={tone} data-direction={direction} {...rest}>
	<Icon name={DIRECTION_ICONS[direction]} size="sm" />
	<span class="change">{sign}{amount}</span>
	{#if null !== verdict}<span class="verdict">{verdict}</span>{/if}
	<span class="period">{period}</span>
	{#if false === isNumber}<VisuallyHidden>{t('common.unknown')}</VisuallyHidden>{/if}
</span>

<style>
	.delta {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--delta-gap, var(--space-1));
		color: var(--delta-neutral, var(--text-muted));
		font-variant-numeric: tabular-nums;
	}

	.delta[data-tone='ok'] {
		color: var(--delta-ok, var(--ok));
	}

	.delta[data-tone='danger'] {
		color: var(--delta-danger, var(--danger));
	}

	.verdict {
		font-weight: 600;
	}

	.period {
		color: var(--text-muted);
	}
</style>
