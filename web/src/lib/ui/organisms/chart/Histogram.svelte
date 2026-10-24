<script lang="ts" module>
	export interface HistogramBucket {
		le: number;
		count: number;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Unit } from '../../types';
	import BarChart, { formatChartValue, type BarDatum } from './BarChart.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		buckets: HistogramBucket[];
		unit: Unit;
		label: string;
	}

	let { buckets, unit, label, class: className, ...rest }: Props = $props();

	const data: BarDatum[] = $derived(
		buckets.map((bucket, index) => ({
			label:
				Number.isFinite(bucket.le) && 0 <= bucket.le
					? `≤${formatChartValue(bucket.le, unit)}`
					: 0 < index
						? `>${formatChartValue(buckets[index - 1].le, unit)}`
						: formatChartValue(null, unit),
			value: bucket.count
		}))
	);
</script>

<BarChart {data} unit="count" {label} orientation="vertical" sorted={false} class={className} {...rest} />
