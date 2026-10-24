<script lang="ts" module>
	export interface ChartTable {
		columns: string[];
		rows: string[][];
	}

	const X_TICK_SPACING = 80;
	const Y_TICK_SPACING = 40;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatNumber } from '$lib/format/number';
	import { TICK_GAP, type Scale } from './scale';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		width: number;
		height: number;
		xScale: Scale;
		yScale: Scale;
		label: string;
		summary: string;
		table: ChartTable;
		children: Snippet;
		yTitle?: string;
		formatX?: (value: number) => string;
		formatY?: (value: number) => string;
		describedby?: string;
		showXTicks?: boolean;
		showYTicks?: boolean;
		showAxis?: boolean;
	}

	const defaultTick = (value: number): string => formatNumber(value, 'compact');

	let {
		width,
		height,
		xScale,
		yScale,
		label,
		summary,
		table,
		children,
		yTitle = '',
		formatX = defaultTick,
		formatY = defaultTick,
		describedby,
		showXTicks = true,
		showYTicks = true,
		showAxis = true,
		class: className,
		...rest
	}: Props = $props();

	const plot = $derived({
		left: xScale.range[0],
		right: xScale.range[1],
		top: yScale.range[1],
		bottom: yScale.range[0]
	});
	const CHAR_WIDTH_PX = 7;
	const anchorFor = (x: number, text: string): 'start' | 'middle' | 'end' => {
		const half = (text.length * CHAR_WIDTH_PX) / 2;
		if (x + half > width) {
			return 'end';
		}
		if (x - half < 0) {
			return 'start';
		}
		return 'middle';
	};
	const xTicks = $derived(
		xScale
			.ticks(Math.max(2, Math.floor((plot.right - plot.left) / X_TICK_SPACING)))
			.map((tick) => ({ tick, text: formatX(tick) }))
			.filter((entry, index, list) => 0 === index || list[index - 1].text !== entry.text)
			.map((entry) => ({ ...entry, x: xScale.apply(entry.tick), anchor: anchorFor(xScale.apply(entry.tick), entry.text) }))
	);
	const yTicks = $derived(yScale.ticks(Math.max(2, Math.floor((plot.bottom - plot.top) / Y_TICK_SPACING))));
</script>

<div class={['chart-frame', className]} {...rest}>
	{#if '' !== yTitle}
		<p class="y-title">{yTitle}</p>
	{/if}
	<svg
		class="plot"
		role="img"
		tabindex="0"
		aria-label={summary}
		aria-describedby={describedby}
		aria-keyshortcuts="ArrowLeft ArrowRight"
		{width}
		{height}
		viewBox={`0 0 ${width} ${height}`}
	>
		{#if showYTicks}{#each yTicks as tick (tick)}
			<line class="gridline" x1={plot.left} x2={plot.right} y1={yScale.apply(tick)} y2={yScale.apply(tick)} />
			<text class="tick" x={plot.left - TICK_GAP} y={yScale.apply(tick)} text-anchor="end" dominant-baseline="middle"
				>{formatY(tick)}</text
			>
		{/each}
		{/if}{#if showXTicks}{#each xTicks as entry (entry.tick)}
			<text
				class="tick"
				x={entry.x}
				y={plot.bottom + TICK_GAP}
				text-anchor={entry.anchor}
				dominant-baseline="hanging">{entry.text}</text
			>
		{/each}
		{/if}
		{#if showAxis}
			<line class="axis" x1={plot.left} x2={plot.right} y1={plot.bottom} y2={plot.bottom} />
		{/if}
		{@render children()}
	</svg>
	<div class="sr-table">
		<table>
			<caption>{label}</caption>
			<thead>
				<tr>
					{#each table.columns as column, index (index)}
						<th scope="col">{column}</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each table.rows as row, rowIndex (rowIndex)}
					<tr>
						{#each row as cell, cellIndex (cellIndex)}
							{#if 0 === cellIndex}
								<th scope="row">{cell}</th>
							{:else}
								<td>{cell}</td>
							{/if}
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>

<style>
	.chart-frame {
		position: relative;
		display: grid;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.y-title {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
	}

	.sr-table {
		position: absolute;
		inline-size: var(--hairline);
		block-size: var(--hairline);
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.plot {
		display: block;
		inline-size: 100%;
		block-size: auto;
		overflow: visible;
		outline: var(--focus-width) solid transparent;
		border-radius: var(--radius-xs);
	}

	.plot:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.gridline {
		stroke: var(--border-subtle);
		stroke-width: var(--hairline);
	}

	.axis {
		stroke: var(--border);
		stroke-width: var(--hairline);
	}

	.tick {
		fill: var(--text-muted);
		font-family: var(--font-sans);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
		letter-spacing: var(--text-caption-tracking);
		font-variant-numeric: tabular-nums;
	}
</style>
