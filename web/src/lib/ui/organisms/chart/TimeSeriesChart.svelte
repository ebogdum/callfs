<script lang="ts" module>
	import { defaultLocale } from '$lib/format/number';

	export type ChartHeight = 'sm' | 'md' | 'lg';
	export type EmptyReason = 'no_samples' | 'not_reported' | 'empty_range';

	const HEIGHTS: Record<ChartHeight, number> = { sm: 160, md: 240, lg: 360 };
	const FALLBACK_WIDTH = 640;
	const SLOT_COUNT = 8;
	const OTHER_SLOT = 7;
	const Y_TICK_COUNT = 4;
	const MARKER_RADIUS = 4;
	const LABEL_OFFSET = 6;
	const BYTE_UNITS: readonly string[] = ['bytes', 'bytes_per_s'];
	const TONE_COLOR: Record<string, string> = {
		neutral: 'var(--text-muted)',
		accent: 'var(--accent)',
		ok: 'var(--ok)',
		warn: 'var(--warn)',
		danger: 'var(--danger)',
		info: 'var(--info)'
	};
	const clockFormat = new Intl.DateTimeFormat(defaultLocale(), { hour: 'numeric', minute: '2-digit' });
	const secondFormat = new Intl.DateTimeFormat(defaultLocale(), { hour: 'numeric', minute: '2-digit', second: '2-digit' });
	const dayFormat = new Intl.DateTimeFormat(defaultLocale(), { month: 'short', day: 'numeric' });
	const SECOND_SPAN_MS = 15 * 60 * 1000;
	const DAY_SPAN_MS = 2 * 24 * 60 * 60 * 1000;
	const EPOCH_SECONDS_LIMIT = 1e11;
	let chartCount = 0;
</script>

<script lang="ts">
	import { base } from '$app/paths';
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Annotation, QueryStatus, Series, Threshold, UiError, Unit } from '../../types';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { DASH, formatNumber } from '$lib/format/number';
	import { formatTimestamp } from '$lib/format/time';
	import Legend from '../../molecules/Legend.svelte';
	import EmptyState from '../../molecules/EmptyState.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import { formatChartValue } from './BarChart.svelte';
	import ChartFrame, { type ChartTable } from './ChartFrame.svelte';
	import { CrosshairState, crosshairGroup } from './crosshair.svelte';
	import {
		DEFAULT_RIGHT_INSET,
		downsampleSeries,
		linearScale,
		nearestIndex,
		niceExtent,
		plotBox,
		runsOf,
		timeScale,
		type Point,
		type Scale,
		type SeriesPoint
	} from './scale';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		series: Series[];
		unit: Unit;
		thresholds?: Threshold[];
		annotations?: Annotation[];
		stacked?: boolean;
		area?: boolean;
		syncKey?: string;
		domain?: readonly [number, number];
		height?: ChartHeight;
		status: QueryStatus;
		error?: UiError;
		emptyReason?: EmptyReason;
		emptyCopy?: { title: string; body: string };
		onretry?: () => void;
	}

	interface Segment {
		upper: Point[];
		lower: Point[];
	}

	interface Layer {
		id: string;
		label: string;
		slot: number;
		dashed: boolean;
		segments: Segment[];
	}

	let {
		series,
		unit,
		thresholds = [],
		annotations = [],
		stacked = false,
		area = false,
		syncKey,
		domain,
		height = 'md',
		status,
		error,
		emptyReason = 'empty_range',
		emptyCopy,
		onretry,
		class: className,
		...rest
	}: Props = $props();

	const chartId = `time-series-${(chartCount += 1)}`;
	const readoutId = `${chartId}-readout`;
	const crosshair: CrosshairState = $derived(undefined === syncKey ? new CrosshairState() : crosshairGroup(syncKey));
	const failure: UiError = $derived(
		error ?? { title: t('chart.error.title'), body: t('chart.error.body'), retryable: true }
	);

	let hiddenIds = $state<string[]>([]);
	let frameWidth = $state(0);
	const width = $derived(0 < frameWidth ? frameWidth : FALLBACK_WIDTH);
	const plotHeight = $derived(HEIGHTS[height]);
	const unitLabel = $derived('none' === unit ? '' : t(`chart.unit.${unit}`));
	
	const collapseSeries = (list: readonly Series[]): Series[] => {
		if (list.length <= SLOT_COUNT) return [...list];
		const rest = list.slice(OTHER_SLOT);
		const totals = new Map<number, number | null>();
		for (const member of rest) {
			for (const [x, y] of member.points) {
				if (null === y) {
					if (!totals.has(x)) totals.set(x, null);
					continue;
				}
				const previous = totals.get(x);
				totals.set(x, 'number' === typeof previous ? previous + y : y);
			}
		}
		const other: Series = {
			id: 'other',
			label: t('chart.other'),
			unit,
			points: [...totals.entries()].sort((a, b) => a[0] - b[0]),
			colorIndex: OTHER_SLOT
		};
		return [...list.slice(0, OTHER_SLOT), other];
	};

	const pool = $derived(collapseSeries(series));
	const showLegend = $derived(0 < pool.length);
	const legendItems = $derived(
		pool.map((entry, index) => ({
			id: entry.id,
			label: entry.label,
			colorIndex: entry.colorIndex ?? index,
			dashed: entry.dashed,
			hidden: hiddenIds.includes(entry.id)
		}))
	);
	const visible = $derived(pool.filter((entry) => !hiddenIds.includes(entry.id)));
		const summary = $derived(
			t('chart.summary', { count: visible.length, names: visible.map((entry) => entry.label).join(', ') })
		);
	const plotted = $derived(
		visible.map((entry) => ({ series: entry, slot: entry.colorIndex ?? pool.indexOf(entry) }))
	);
	const sampled = $derived(
		plotted.map((entry) => ({
			id: entry.series.id,
			label: entry.series.label,
			slot: entry.slot,
			dashed: entry.series.dashed ?? false,
			points: downsampleSeries(entry.series.points, width)
		}))
	);
	const filled = $derived(area || stacked);

	const stackLayers = (entries: typeof sampled): Layer[] => {
		const floors = new Map<number, number>();
		return entries.map((entry) => {
			const upper: Point[] = [];
			const lower: Point[] = [];
			for (const [x, y] of entry.points) {
				if (null === y) continue;
				const floor = floors.get(x) ?? 0;
				floors.set(x, floor + y);
				upper.push([x, floor + y]);
				lower.push([x, floor]);
			}
			return { id: entry.id, label: entry.label, slot: entry.slot, dashed: entry.dashed, segments: [{ upper, lower }] };
		});
	};

	const lineLayers = (entries: typeof sampled): Layer[] =>
		entries.map((entry) => ({
			id: entry.id,
			label: entry.label,
			slot: entry.slot,
			dashed: entry.dashed,
			segments: runsOf(entry.points).map((run) => ({
				upper: run,
				lower: run.map(([x]): Point => [x, 0])
			}))
		}));

	const layers = $derived(stacked ? stackLayers(sampled) : lineLayers(sampled));

	const extentOf = (values: readonly number[]): [number, number] => {
		if (0 === values.length) return [0, 1];
		return values.reduce<[number, number]>(
			([low, high], value) => [Math.min(low, value), Math.max(high, value)],
			[Infinity, -Infinity]
		);
	};

	const xRange = $derived(
		domain ??
			extentOf(layers.flatMap((layer) => layer.segments.flatMap((segment) => segment.upper.map(([x]) => x))))
	);
	const yRange = $derived(
		extentOf([
			...layers.flatMap((layer) =>
				layer.segments.flatMap((segment) => [...segment.upper, ...segment.lower].map(([, y]) => y))
			),
			...thresholds.map((threshold) => threshold.value)
		])
	);
	const box = $derived(plotBox(width, plotHeight, DEFAULT_RIGHT_INSET));
	const msPerUnit = $derived(EPOCH_SECONDS_LIMIT > Math.abs(xRange[1]) ? 1000 : 1);
	const xScale: Scale = $derived(timeScale(xRange, [box.left, box.right], msPerUnit));
	const yScale: Scale = $derived(
		linearScale(niceExtent(Math.min(0, yRange[0]), 0 < yRange[1] ? yRange[1] : 1, Y_TICK_COUNT), [box.bottom, box.top])
	);

	const toPixels = (points: readonly Point[]): Point[] =>
		points.map(([x, y]): Point => [xScale.apply(x), yScale.apply(y)]);
	const pathFrom = (pixels: readonly Point[]): string =>
		pixels.map(([x, y], index) => `${0 === index ? 'M' : 'L'}${x.toFixed(1)} ${y.toFixed(1)}`).join(' ');
	const areaFrom = (top: readonly Point[], bottom: readonly Point[]): string => {
		if (0 === top.length) return '';
		const back = [...bottom].reverse().map(([x, y]) => `L${x.toFixed(1)} ${y.toFixed(1)}`);
		return `${pathFrom(top)} ${back.join(' ')} Z`;
	};
	const vizColor = (slot: number): string => `var(--viz-${(slot % SLOT_COUNT) + 1})`;
	const dashFor = (slot: number, dashed: boolean): string =>
		`var(--dash-${dashed ? 2 : (Math.floor(slot / SLOT_COUNT) % 4) + 1})`;

	const drawn = $derived(
		layers.map((layer) => ({
			id: layer.id,
			label: layer.label,
			color: vizColor(layer.slot),
			dash: dashFor(layer.slot, layer.dashed),
			segments: layer.segments.map((segment) => ({
				top: toPixels(segment.upper),
				bottom: toPixels(segment.lower)
			}))
		}))
	);


	const shownAnnotations = $derived(
		annotations.filter((annotation) => xScale.domain[0] <= annotation.at && annotation.at <= xScale.domain[1])
	);

	const formatAxis = (value: number, step: number): string =>
		BYTE_UNITS.includes(unit) ? formatBytes(value, { precision: 0 }).text : formatChartValue(value, 'none', step);
	const formatClock = (value: number): string => {
		const span = (xScale.domain[1] - xScale.domain[0]) * msPerUnit;
		const at = value * msPerUnit;
		if (DAY_SPAN_MS <= span) return dayFormat.format(at);
		if (SECOND_SPAN_MS > span) return secondFormat.format(at);
		return clockFormat.format(at);
	};

	const formatReading = (value: number | null): string => {
		const isBytes = BYTE_UNITS.includes(unit);
		const text = null === value ? DASH : isBytes ? formatBytes(value).text : formatNumber(value, 'decimal');
		return t('chart.readout.value', { value: text, unit: isBytes ? '' : unitLabel }).trim();
	};

	const nearestValue = (points: readonly SeriesPoint[], time: number): number | null => {
		const index = nearestIndex(
			points.map(([x]) => x),
			time
		);
		return -1 === index ? null : points[index][1];
	};

	const readout = $derived.by(() => {
		if (null === crosshair.time) return null;
		const time = crosshair.time;
		return {
			time: formatTimestamp(time * msPerUnit),
			x: Math.min(box.right, Math.max(box.left, xScale.apply(time))),
			rows: plotted.map((entry) => {
				const value = nearestValue(entry.series.points, time);
				return {
					id: entry.series.id,
					text: t('chart.readout.row', { label: entry.series.label, value: formatReading(value) }),
					color: vizColor(entry.slot),
					y: null === value ? null : yScale.apply(value)
				};
			})
		};
	});

	const stepTimes = $derived(
		visible[0] ? visible[0].points.filter(([, y]) => null !== y).map(([x]) => x) : []
	);

	const tableData: ChartTable = $derived.by(() => {
		const times = Array.from(new Set(visible.flatMap((entry) => entry.points.map(([x]) => x)))).sort((a, b) => a - b);
		const lookups = visible.map((entry) => new Map<number, number | null>(entry.points));
		return {
			columns: [t('chart.table.time'), ...visible.map((entry) => entry.label)],
			rows: times.map((time) => [
				formatTimestamp(time * msPerUnit),
				...lookups.map((lookup) => formatReading(lookup.get(time) ?? null))
			])
		};
	});

	const toggleSeries = (id: string): void => {
		hiddenIds = hiddenIds.includes(id) ? hiddenIds.filter((hidden) => hidden !== id) : [...hiddenIds, id];
	};

	const handlePointerMove = (event: PointerEvent & { currentTarget: HTMLDivElement }): void => {
		const bounds = event.currentTarget.getBoundingClientRect();
		const time = xScale.invert(event.clientX - bounds.left);
		const [first, last] = xScale.domain;
		crosshair.place(Math.min(last, Math.max(first, time)), chartId);
	};

	const handlePointerLeave = (): void => {
		crosshair.clear(chartId);
	};

	const handleKeydown = (event: KeyboardEvent): void => {
		let step = 0;
		if ('ArrowRight' === event.key) step = 1;
		if ('ArrowLeft' === event.key) step = -1;
		if (0 === step) return;
		event.preventDefault();
		if (0 === stepTimes.length) return;
		const base =
			null === crosshair.time ? (0 < step ? -1 : stepTimes.length) : nearestIndex(stepTimes, crosshair.time);
		const next = Math.min(stepTimes.length - 1, Math.max(0, base + step));
		crosshair.place(stepTimes[next], chartId);
	};
</script>

<div class={['time-series', className]} data-status={status} data-height={height} bind:clientWidth={frameWidth} {...rest}>
	{#if 'loading' === status}
		<Skeleton shape="rect" height={`${plotHeight}px`} />
	{:else if 'error' === status}
		<ErrorState error={failure} onretry={onretry} variant="region" />
	{:else if 'empty' === status}
		{#if undefined !== emptyCopy}
			<EmptyState
				compact
				variant={'no_samples' === emptyReason ? 'first-use' : 'no-results'}
				icon={'no_samples' === emptyReason ? 'clock' : 'activity'}
				title={emptyCopy.title}
				body={emptyCopy.body}
			/>
		{:else if 'no_samples' === emptyReason}
			<EmptyState
				compact
				variant="first-use"
				icon="clock"
				title={t('chart.no_samples.title')}
				body={t('chart.no_samples.body')}
			/>
		{:else if 'not_reported' === emptyReason}
			<EmptyState
				compact
				variant="not-available"
				icon="eye-off"
				title={t('chart.not_reported.title')}
				primary={{ label: t('chart.not_reported.action'), href: `${base}/insights/metrics` }}
			/>
		{:else}
			<EmptyState
				compact
				variant="no-results"
				icon="activity"
				title={t('chart.empty_range.title')}
				body={t('chart.empty_range.body')}
			/>
		{/if}
	{:else}
		{#if 'partial' === status}
			<InlineAlert tone="warn" title={t('chart.partial.title')} body={t('chart.partial.body')} />
		{/if}
		{#if showLegend}
			<Legend series={legendItems} ontoggle={toggleSeries} />
		{/if}
		<ChartFrame
			width={width}
			height={plotHeight}
			{xScale}
			{yScale}
			label={t('chart.label')}
			{summary}
			table={tableData}
			yTitle={unitLabel}
			formatX={formatClock}
			formatY={formatAxis}
			describedby={readoutId}
			onkeydown={handleKeydown}
			onpointermove={handlePointerMove}
			onpointerleave={handlePointerLeave}
		>
			{#snippet children()}
				{#each thresholds as threshold (threshold.value)}
					{@const y = yScale.apply(threshold.value)}
					<rect
						class="threshold-band"
						x={box.left}
						y={box.top}
						width={box.right - box.left}
						height={Math.max(0, y - box.top)}
						style:fill={TONE_COLOR[threshold.tone]}
					/>
					<line
						class="threshold"
						x1={box.left}
						x2={box.right}
						y1={y}
						y2={y}
						style:stroke={TONE_COLOR[threshold.tone]}
					/>
					<text class="mark-label" x={box.right} y={y - LABEL_OFFSET} text-anchor="end">{threshold.label}</text>
				{/each}
				{#each drawn as layer (layer.id)}
					{#each layer.segments as segment, index (index)}
						{#if filled}
							<path class="area" d={areaFrom(segment.top, segment.bottom)} style:fill={layer.color} />
						{/if}
						<path class="series" d={pathFrom(segment.top)} style:stroke={layer.color} style:stroke-dasharray={layer.dash} />
						{#if 1 === segment.top.length}
							<circle class="dot" cx={segment.top[0][0]} cy={segment.top[0][1]} r={MARKER_RADIUS} style:fill={layer.color} />
						{/if}
					{/each}
				{/each}
				{#each shownAnnotations as annotation (`${annotation.at}:${annotation.label}`)}
					{@const x = xScale.apply(annotation.at)}
					<g class="annotation">
						<line class="annotation-line" x1={x} x2={x} y1={box.top} y2={box.bottom} style:stroke={TONE_COLOR[annotation.tone]} />
						{#if annotation.href}
							<a href={annotation.href}>
								<text class="mark-label" x={x + LABEL_OFFSET} y={box.top + LABEL_OFFSET} dominant-baseline="hanging">{annotation.label}</text>
							</a>
						{:else}
							<text class="mark-label" x={x + LABEL_OFFSET} y={box.top + LABEL_OFFSET} dominant-baseline="hanging">{annotation.label}</text>
						{/if}
					</g>
				{/each}
				{#if readout}
					<line class="crosshair" x1={readout.x} x2={readout.x} y1={box.top} y2={box.bottom} />
					{#each readout.rows as row (row.id)}
						{#if null !== row.y}
							<circle class="marker" cx={readout.x} cy={row.y} r={MARKER_RADIUS} style:fill={row.color} />
						{/if}
					{/each}
				{/if}
			{/snippet}
		</ChartFrame>
		{#if readout}
			<div class="readout" id={readoutId} style:inset-inline-start={`${readout.x}px`}>
				<span class="readout-time">{readout.time}</span>
				{#each readout.rows as row (row.id)}
					<span class="readout-row" style:color={row.color}>{row.text}</span>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.time-series {
		position: relative;
		display: grid;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.series {
		fill: none;
		stroke-width: 2;
		stroke-linejoin: round;
	}

	.dot {
		stroke: none;
	}

	.area {
		fill-opacity: 0.16;
		stroke: none;
	}

	.threshold-band {
		fill-opacity: 0.1;
	}

	.threshold {
		stroke-width: var(--hairline);
		stroke-dasharray: var(--dash-2);
	}

	.annotation-line {
		stroke: var(--border-strong);
		stroke-width: var(--hairline);
		stroke-dasharray: var(--dash-3);
	}

	.mark-label {
		fill: var(--text-muted);
		font-family: var(--font-sans);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
	}

	.crosshair {
		stroke: var(--border-strong);
		stroke-width: var(--hairline);
	}

	.marker {
		stroke: var(--surface);
		stroke-width: var(--hairline);
	}

	.readout {
		position: absolute;
		inset-block-start: 0;
		transform: translateX(-50%);
		display: grid;
		gap: var(--space-0-5);
		padding: var(--space-1) var(--space-2);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-sm);
		background: var(--surface-overlay);
		color: var(--text);
		box-shadow: var(--shadow-md);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
		pointer-events: none;
		transition: opacity var(--dur-fade) var(--ease-standard);
	}

	.readout-time {
		color: var(--text-muted);
	}

	.readout-row {
		font-weight: var(--text-caption-weight);
	}

	@media (prefers-reduced-motion: reduce) {
		.readout {
			transition: none;
		}
	}
</style>
