<script lang="ts" module>
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatBytes } from '../../../format/bytes';
	import { DASH, formatNumber } from '../../../format/number';
	import type { Series, Unit } from '../../types';

	export type ChartScale = (value: number) => number;

	export interface ChartTable {
		columns: string[];
		rows: string[][];
	}

	export interface BarDatum {
		label: string;
		value: number;
		colorIndex?: number;
	}

	const unitSuffixes: Record<Unit, string> = {
		bytes: '',
		bytes_per_s: '/s',
		count: '',
		per_s: '/s',
		ms: ' ms',
		s: ' s',
		percent: '%',
		ratio: '×',
		none: ''
	};

	export const unitSymbols: Record<Unit, string> = {
		bytes: 'B',
		bytes_per_s: 'B/s',
		count: '#',
		per_s: '/s',
		ms: 'ms',
		s: 's',
		percent: '%',
		ratio: '×',
		none: ''
	};

	export const formatChartValue = (value: number | null, unit: Unit): string => {
		if (null === value) {
			return DASH;
		}
		const base = 'bytes' === unit || 'bytes_per_s' === unit ? formatBytes(value).text : formatNumber(value, 'compact');
		return `${base}${unitSuffixes[unit]}`;
	};
</script>

<script lang="ts">
	import { t } from '$lib/copy/t';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ChartFrame from './ChartFrame.svelte';
	import { AXIS_BOTTOM, AXIS_LEFT, PLOT_TOP, linearScale, niceExtent, type Scale } from './scale';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		data: BarDatum[];
		unit: Unit;
		label: string;
		orientation?: 'horizontal' | 'vertical';
		sorted?: boolean;
		stacked?: Series[];
	}

	let {
		data,
		unit,
		label,
		orientation = 'horizontal',
		sorted = true,
		stacked = [],
		class: className,
		...rest
	}: Props = $props();

	const FALLBACK_WIDTH = 640;
	const VERTICAL_HEIGHT = 240;
	const ROW_HEIGHT = 32;
	const BAR_THICKNESS = 14;
	const CATEGORY_MAX = 160;
	const CATEGORY_SHARE = 0.32;
	const VALUE_SPACE = 80;
	const LABEL_GAP = 8;
	const RIGHT_INSET = 16;
	const CHAR_WIDTH = 7;
	const Y_TICK_COUNT = 4;

	let frameWidth = $state(0);
	const chartWidth = $derived(0 < frameWidth ? frameWidth : FALLBACK_WIDTH);
	const isHorizontal = $derived('horizontal' === orientation);

	const rows = $derived(
		data.map((datum, index) => {
			const segments =
				0 < stacked.length
					? stacked.map((series, seriesIndex) => ({
							key: `${datum.label}-${series.id}`,
							value: series.points[index]?.[1] ?? 0,
							colorIndex: series.colorIndex ?? seriesIndex + 1
						}))
					: [{ key: datum.label, value: datum.value, colorIndex: datum.colorIndex ?? 1 }];
			return {
				label: datum.label,
				total: segments.reduce((sum, segment) => sum + segment.value, 0),
				segments
			};
		})
	);

	const ordered = $derived(sorted ? [...rows].sort((a, b) => b.total - a.total) : rows);
	const peak = $derived(Math.max(0, ...ordered.map((row) => row.total)));
	const integerFloor = $derived('count' === unit ? Y_TICK_COUNT : 1);
	const valueSpan = $derived(
		isHorizontal ? (0 < peak ? peak : 1) : niceExtent(0, Math.max(integerFloor, peak), Y_TICK_COUNT)[1]
	);

	const categoryWidth = $derived(Math.min(CATEGORY_MAX, Math.round(chartWidth * CATEGORY_SHARE)));
	const valueStart = $derived(categoryWidth + LABEL_GAP);
	const valueWidth = $derived(Math.max(1, chartWidth - valueStart - VALUE_SPACE));
	const chartHeight = $derived(isHorizontal ? Math.max(1, ordered.length) * ROW_HEIGHT : VERTICAL_HEIGHT);
	const baseline = $derived(VERTICAL_HEIGHT - AXIS_BOTTOM);
	const plotWidth = $derived(Math.max(1, chartWidth - AXIS_LEFT - RIGHT_INSET));
	const bandSize = $derived(isHorizontal ? ROW_HEIGHT : plotWidth / Math.max(1, ordered.length));

	const valueToPosition = (value: number): number =>
		isHorizontal
			? valueStart + (value / valueSpan) * valueWidth
			: baseline - (value / valueSpan) * (baseline - PLOT_TOP);

	const bandStart = (index: number): number => (isHorizontal ? index * ROW_HEIGHT : AXIS_LEFT + index * bandSize);

	const fit = (text: string, room: number): string => {
		const limit = Math.max(3, Math.floor(room / CHAR_WIDTH));
		return text.length <= limit ? text : `${text.slice(0, limit - 1)}…`;
	};

	const labelStride = $derived.by(() => {
		if (isHorizontal || 0 === ordered.length) {
			return 1;
		}
		const widest = Math.max(...ordered.map((row) => row.label.length)) * CHAR_WIDTH + LABEL_GAP;
		return Math.max(1, Math.ceil(widest / Math.max(1, bandSize)));
	});

	const bars = $derived(
		ordered.flatMap((row, index) => {
			let cursor = 0;
			return row.segments.map((segment, segmentIndex) => {
				const start = valueToPosition(cursor);
				cursor += segment.value;
				const end = valueToPosition(cursor);
				const near = Math.min(start, end);
				const extent = Math.abs(end - start);
				const thickness = Math.min(BAR_THICKNESS * 2, bandSize * 0.6);
				return {
					key: segment.key,
					label: row.label,
					tip:
						1 < row.segments.length
							? `${row.label} · ${stacked[segmentIndex]?.label ?? ''}: ${formatChartValue(segment.value, unit)}`
							: `${row.label}: ${formatChartValue(segment.value, unit)}`,
					colorIndex: segment.colorIndex,
					x: isHorizontal ? near : bandStart(index) + (bandSize - thickness) / 2,
					y: isHorizontal ? bandStart(index) + (ROW_HEIGHT - BAR_THICKNESS) / 2 : near,
					width: isHorizontal ? extent : thickness,
					height: isHorizontal ? BAR_THICKNESS : extent
				};
			});
		})
	);

	const tracks = $derived(
		isHorizontal
			? ordered.flatMap((row, index) =>
					0 < row.total
						? [
								{
									key: row.label,
									y: bandStart(index) + (ROW_HEIGHT - BAR_THICKNESS) / 2
								}
							]
						: []
				)
			: []
	);

	const labels = $derived(
		ordered.map((row, index) => ({
			key: index,
			full: row.label,
			category: isHorizontal
				? fit(row.label, categoryWidth)
				: 0 === index % labelStride
					? fit(row.label, bandSize * labelStride - LABEL_GAP)
					: '',
			value: formatChartValue(row.total, unit),
			categoryX: isHorizontal ? 0 : bandStart(index) + bandSize / 2,
			categoryY: isHorizontal ? bandStart(index) + ROW_HEIGHT / 2 : baseline + LABEL_GAP,
			valueX: isHorizontal ? valueToPosition(row.total) + LABEL_GAP : bandStart(index) + bandSize / 2,
			valueY: isHorizontal ? bandStart(index) + ROW_HEIGHT / 2 : valueToPosition(row.total) - LABEL_GAP
		}))
	);

	const lastIndex = $derived(Math.max(0, ordered.length - 1));

	const xScale: Scale = $derived(
		isHorizontal
			? linearScale([0, valueSpan], [valueStart, valueStart + valueWidth])
			: linearScale([0, Math.max(1, lastIndex)], [AXIS_LEFT, AXIS_LEFT + plotWidth])
	);
	const yScale: Scale = $derived(
		isHorizontal
			? linearScale([0, Math.max(1, lastIndex)], [chartHeight, 0])
			: linearScale([0, valueSpan], [baseline, PLOT_TOP])
	);

	const formatTick = (value: number): string => formatChartValue(value, unit);

	const summary = $derived(ordered.map((row) => `${row.label}: ${formatChartValue(row.total, unit)}`).join('; '));

	const table: ChartTable = $derived({
		columns: [label, unitSymbols[unit]],
		rows: ordered.map((row) => [row.label, formatChartValue(row.total, unit)])
	});
</script>

<div class={['bar-chart', className]} bind:clientWidth={frameWidth} {...rest}>
	{#if 0 === ordered.length}
		<EmptyState compact variant="first-use" icon="bar-chart" title={t('chart.empty.title')} />
	{:else}
		<ChartFrame
			width={chartWidth}
			height={isHorizontal ? chartHeight : VERTICAL_HEIGHT}
			{xScale}
			{yScale}
			{label}
			summary={`${label}. ${summary}`}
			{table}
			formatY={formatTick}
			showXTicks={false}
			showYTicks={!isHorizontal}
			showAxis={!isHorizontal}
		>
			{#snippet children()}
				{#each tracks as track (track.key)}
					<rect class="track" x={valueStart} y={track.y} width={valueWidth} height={BAR_THICKNESS} rx="3" />
				{/each}
				{#each bars as bar, index (index)}
					<rect
						class="bar"
						data-bar={bar.label}
						x={bar.x}
						y={bar.y}
						width={bar.width}
						height={bar.height}
						rx="3"
						style:fill={`var(--viz-${bar.colorIndex})`}
					><title>{bar.tip}</title></rect>
				{/each}
				{#each labels as item (item.key)}
					<text
						class="category"
						x={item.categoryX}
						y={item.categoryY}
						text-anchor={isHorizontal ? 'start' : 'middle'}
						dominant-baseline={isHorizontal ? 'middle' : 'hanging'}
						><title>{item.full}</title>{item.category}</text
					>
					<text
						class="value"
						x={item.valueX}
						y={item.valueY}
						text-anchor={isHorizontal ? 'start' : 'middle'}
						dominant-baseline={isHorizontal ? 'middle' : 'auto'}>{item.value}</text
					>
				{/each}
			{/snippet}
		</ChartFrame>
	{/if}
</div>

<style>
	.bar-chart {
		display: grid;
		min-inline-size: 0;
	}

	.track {
		fill: var(--surface-hover);
	}

	.category,
	.value {
		font-family: var(--font-sans);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-variant-numeric: tabular-nums;
		fill: var(--text);
	}

	.category {
		fill: var(--text-muted);
	}

	.value {
		font-weight: var(--text-caption-weight);
	}
</style>
