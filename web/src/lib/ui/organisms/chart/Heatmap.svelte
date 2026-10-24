<script lang="ts" module>
	export interface HeatCell {
		x: number;
		y: number;
		value: number;
		yLabel?: string;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { DASH, defaultLocale, formatNumber } from '../../../format/number';
	import type { Unit } from '../../types';
	import ChartFrame from './ChartFrame.svelte';
	import { formatChartValue, type ChartTable } from './BarChart.svelte';
	import { AXIS_BOTTOM, type Scale } from './scale';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		cells: HeatCell[];
		xUnit: Unit;
		yUnit: Unit;
		label: string;
		scale: 'seq';
	}

	let { cells, xUnit, yUnit, label, scale, class: className, ...rest }: Props = $props();

	const FALLBACK_WIDTH = 640;
	const RIGHT_INSET = 16;
	const EPOCH_FLOOR = 1e8;
	const EPOCH_SECONDS_LIMIT = 1e11;
	const plotLeft = 96;
	const clockFormat = new Intl.DateTimeFormat(defaultLocale(), { hour: 'numeric', minute: '2-digit' });

	let frameWidth = $state(0);
	const chartWidth = $derived(0 < frameWidth ? frameWidth : FALLBACK_WIDTH);
	const plotWidth = $derived(Math.max(1, chartWidth - plotLeft - RIGHT_INSET));
	const cellHeight = 24;
	const rampSteps = 7;

	const xValues = $derived([...new Set(cells.map((cell) => cell.x))].sort((a, b) => a - b));
	const yValues = $derived([...new Set(cells.map((cell) => cell.y))].sort((a, b) => a - b));

	const range = $derived.by(() => {
		if (0 === cells.length) {
			return { low: 0, high: 0 };
		}
		const values = cells.map((cell) => cell.value);
		return { low: Math.min(...values), high: Math.max(...values) };
	});

	const cellWidth = $derived(plotWidth / Math.max(1, xValues.length));
	const chartHeight = $derived(Math.max(1, yValues.length) * cellHeight);

	const stepOf = (value: number): number =>
		0 < range.high - range.low
			? 1 + Math.min(rampSteps - 1, Math.floor(((value - range.low) / (range.high - range.low)) * rampSteps))
			: rampSteps;

	const columnOf = (x: number): number => plotLeft + xValues.indexOf(x) * cellWidth;
	const rowOf = (y: number): number => yValues.indexOf(y) * cellHeight;

	const tiles = $derived(
		cells.map((cell) => ({
			key: `${cell.x}:${cell.y}`,
			x: columnOf(cell.x) + 1,
			y: rowOf(cell.y) + 1,
			width: cellWidth - 2,
			height: cellHeight - 2,
			step: stepOf(cell.value),
			tip: `${formatChartValue(cell.x, xUnit)}, ${cell.yLabel ?? formatChartValue(cell.y, yUnit)}: ${formatNumber(cell.value, 'compact')}`
		}))
	);

	const TICK_CHARS = 14;
	const shorten = (text: string): string => (TICK_CHARS < text.length ? `${text.slice(0, TICK_CHARS - 1)}…` : text);

	const rowLabels = $derived(
		new Map(cells.flatMap((cell) => (undefined === cell.yLabel ? [] : [[cell.y, cell.yLabel] as const])))
	);

	const tickLabels = $derived(
		yValues.map((value, index) => ({
			key: index,
			full: rowLabels.get(value) ?? formatChartValue(value, yUnit),
			text: shorten(rowLabels.get(value) ?? formatChartValue(value, yUnit)),
			y: rowOf(value) + cellHeight / 2
		}))
	);

	const categoryScale = (values: number[], apply: (value: number) => number, range: readonly [number, number]): Scale => ({
		domain: [values[0] ?? 0, values[values.length - 1] ?? 0],
		range,
		apply,
		invert: (position) =>
			values.reduce((best, value) => (Math.abs(apply(value) - position) < Math.abs(apply(best) - position) ? value : best), values[0] ?? 0),
		ticks: (count) => {
			const every = Math.max(1, Math.ceil(values.length / Math.max(1, count)));
			return values.filter((_, index) => 0 === index % every);
		}
	});

	const formatX = (value: number): string => {
		if (EPOCH_FLOOR > Math.abs(value)) return formatChartValue(value, xUnit);
		return clockFormat.format(EPOCH_SECONDS_LIMIT > Math.abs(value) ? value * 1000 : value);
	};

	const xScale = $derived(
		categoryScale(xValues, (value) => columnOf(value) + cellWidth / 2, [plotLeft, plotLeft + plotWidth])
	);
	const yScale = $derived(categoryScale(yValues, (value) => rowOf(value) + cellHeight / 2, [chartHeight, 0]));

	const summary = $derived(`${formatNumber(range.low, 'compact')}–${formatNumber(range.high, 'compact')}`);

	const table: ChartTable = $derived({
		columns: [label, ...xValues.map((value) => formatChartValue(value, xUnit))],
		rows: yValues.map((y) => [
			rowLabels.get(y) ?? formatChartValue(y, yUnit),
			...xValues.map((x) => {
				const cell = cells.find((candidate) => candidate.x === x && candidate.y === y);
				return undefined === cell ? DASH : formatNumber(cell.value, 'compact');
			})
		])
	});
</script>

<div class={['heatmap', className]} bind:clientWidth={frameWidth} {...rest}>
	<ChartFrame
		width={chartWidth}
		height={chartHeight + AXIS_BOTTOM}
		{xScale}
		{yScale}
		{label}
		summary={`${label}. ${summary}`}
		{table}
		{formatX}
		showYTicks={false}
		showAxis={false}
	>
		{#snippet children()}
			{#each tiles as tile (tile.key)}
				<rect class="cell" x={tile.x} y={tile.y} width={tile.width} height={tile.height} style:fill={`var(--${scale}-${tile.step})`}>
					<title>{tile.tip}</title>
				</rect>
			{/each}
			{#each tickLabels as tick (tick.key)}
				<text class="tick" x={plotLeft - 8} y={tick.y} text-anchor="end" dominant-baseline="middle"><title>{tick.full}</title>{tick.text}</text>
			{/each}
		{/snippet}
	</ChartFrame>
</div>

<style>
	.heatmap {
		display: grid;
		min-inline-size: 0;
	}

	.tick {
		font-family: var(--font-sans);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-variant-numeric: tabular-nums;
		fill: var(--text-muted);
	}
</style>
