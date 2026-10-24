<script lang="ts" module>
	export const TOKENS = ['--threshold-bar-label', '--threshold-bar-text'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Threshold, Unit } from '../types';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatDuration } from '$lib/format/duration';
	import { formatCount, formatNumber } from '$lib/format/number';
	import Meter from '../atoms/Meter.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		value: number;
		max: number;
		thresholds: Threshold[];
		label: string;
		unit: Unit;
		free?: number | null;
		class?: string;
	}

	let { value, max, thresholds, label, unit, free = null, class: className, ...rest }: Props = $props();

	const AMOUNT_FORMATTERS: Record<Unit, (amount: number) => string> = {
		bytes: (amount) => formatBytes(amount).text,
		bytes_per_s: (amount) => `${formatBytes(amount).text}/s`,
		count: (amount) => formatNumber(amount, 'int'),
		per_s: (amount) => `${formatNumber(amount, 'decimal')}/s`,
		ms: (amount) => formatDuration(amount, 'short'),
		s: (amount) => formatDuration(amount * 1000, 'short'),
		percent: (amount) => `${formatNumber(amount, 'decimal')}%`,
		ratio: (amount) => formatNumber(amount, 'decimal'),
		none: (amount) => formatNumber(amount, 'decimal'),
	};

	const left = $derived(Math.max(0, null === free ? max - value : free));

	const reading = $derived(
		t('u077molecules.free', {
			free: AMOUNT_FORMATTERS[unit](left),
			max: AMOUNT_FORMATTERS[unit](max),
			rate: formatCount(left, max)?.rate ?? '',
		})
	);

	const usedLine = $derived(t('u077molecules.used', { used: AMOUNT_FORMATTERS[unit](value) }));

	const thresholdLines = $derived(
		thresholds.map((threshold, index) => ({
			key: index,
			tone: threshold.tone,
			text: t('danger' === threshold.tone ? 'u077molecules.danger_at' : 'u077molecules.warn_at', {
				left: AMOUNT_FORMATTERS[unit](Math.max(0, max - threshold.value)),
			}),
		}))
	);
</script>

<Stack gap="1-5" class={['threshold-bar', className]} {...rest}>
	<span class="label">{label}</span>
	<Meter {value} {max} {label} {thresholds} {unit} />
	<Stack gap="1">
		<p class="reading">{reading}</p>
		<p class="used">{usedLine}</p>
		{#each thresholdLines as line (line.key)}
			<p class="threshold" data-tone={line.tone}>{line.text}</p>
		{/each}
	</Stack>
</Stack>

<style>
	.label {
		color: var(--threshold-bar-label, var(--text-muted));
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
	}

	.reading,
	.used,
	.threshold {
		margin: 0;
		color: var(--threshold-bar-text, var(--text));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-sm-weight);
		letter-spacing: var(--text-sm-tracking);
		font-variant-numeric: tabular-nums;
		text-wrap: pretty;
	}

	.reading {
		font-weight: var(--text-label-weight);
	}

	.used {
		color: var(--text-muted);
	}

	.threshold[data-tone='warn'] {
		color: var(--warn-text);
	}

	.threshold[data-tone='danger'] {
		color: var(--danger-text);
	}
</style>
