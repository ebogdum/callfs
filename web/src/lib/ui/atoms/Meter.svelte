<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Threshold, Unit } from '../types';
	import { formatBytes } from '$lib/format/bytes';
	import { formatNumber } from '$lib/format/number';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		value: number;
		max: number;
		label: string;
		thresholds?: Threshold[];
		unit?: Unit;
		class?: string;
	}

	let { value, max, label, thresholds = [], unit = 'none', class: className, ...rest }: Props = $props();

	const percentOf = (amount: number): number => (0 < max ? (Math.min(max, Math.max(0, amount)) / max) * 100 : 0);

	const formatMeasure = (amount: number, measure: Unit): string => {
		switch (measure) {
			case 'bytes':
				return formatBytes(amount).text;
			case 'bytes_per_s':
				return `${formatBytes(amount).text}/s`;
			case 'percent':
				return `${formatNumber(amount, 'decimal')} %`;
			case 'ms':
				return `${formatNumber(amount, 'int')} ms`;
			case 's':
				return `${formatNumber(amount, 'decimal')} s`;
			case 'per_s':
				return `${formatNumber(amount, 'decimal')}/s`;
			case 'count':
				return formatNumber(amount, 'int');
			default:
				return formatNumber(amount, 'decimal');
		}
	};

	const current = $derived(Math.min(max, Math.max(0, value)));
	const fillPercent = $derived(percentOf(value));
	const highestCrossed = $derived(
		[...thresholds]
			.sort((first, second) => first.value - second.value)
			.filter((threshold) => threshold.value <= value)
			.at(-1)
	);
	const tone = $derived(highestCrossed?.tone ?? 'accent');
	const valueText = $derived(formatMeasure(value, unit));
</script>

<span
	class={['meter', className]}
	data-tone={tone}
	role="meter"
	aria-label={label}
	aria-valuemin={0}
	aria-valuemax={max}
	aria-valuenow={current}
	aria-valuetext={valueText}
	{...rest}
>
	<span class="fill" style:inline-size={`${fillPercent}%`}></span>
	{#each thresholds as threshold}
		<span
			class="tick"
			data-tone={threshold.tone}
			title={threshold.label}
			aria-hidden="true"
			style:inset-inline-start={`${percentOf(threshold.value)}%`}
		></span>
	{/each}
</span>

<style>
	.meter {
		--meter-tone: var(--accent);
		position: relative;
		display: flex;
		align-items: stretch;
		inline-size: 100%;
		block-size: var(--meter-h, var(--space-2));
		border-radius: var(--radius-full);
		background: var(--meter-track, var(--surface-sunken));
	}

	.meter[data-tone='warn'] {
		--meter-tone: var(--warn);
	}

	.meter[data-tone='danger'] {
		--meter-tone: var(--danger);
	}

	.fill {
		flex: none;
		max-inline-size: 100%;
		border-radius: var(--radius-full);
		background: var(--meter-fill, var(--meter-tone));
		transition: inline-size var(--dur-medium) var(--ease-out);
	}

	.tick {
		position: absolute;
		inset-block: 0;
		inline-size: var(--space-0-5);
		transform: translateX(-50%);
		background: var(--meter-tick, var(--text));
		box-shadow: 0 0 0 var(--hairline) var(--surface);
	}

	.tick[data-tone='warn'] {
		background: var(--warn);
	}

	.tick[data-tone='danger'] {
		background: var(--danger);
	}
</style>
