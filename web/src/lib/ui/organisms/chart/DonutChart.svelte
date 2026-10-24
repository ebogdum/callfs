<script lang="ts" module>
	export interface DonutPart {
		label: string;
		value: number;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatNumber } from '../../../format/number';
	import BarChart, { unitSymbols, type BarDatum } from './BarChart.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		parts: DonutPart[];
		label: string;
	}

	let { parts, label, class: className, ...rest }: Props = $props();

	const maximumSlices = 5;
	const chartWidth = 480;
	const chartHeight = 240;
	const centerX = 240;
	const centerY = 120;
	const ringRadius = 60;
	const labelRadius = 84;
	const circumference = 2 * Math.PI * ringRadius;

	const total = $derived(parts.reduce((sum, part) => sum + part.value, 0));

	const anchorFor = (cosine: number): 'start' | 'middle' | 'end' => {
		if (0.2 < cosine) {
			return 'start';
		}
		return -0.2 > cosine ? 'end' : 'middle';
	};

	const slices = $derived.by(() => {
		let offset = 0;
		return parts.map((part, index) => {
			const share = 0 < total ? part.value / total : 0;
			const start = offset;
			offset += share;
			const midAngle = (start + share / 2) * 2 * Math.PI - Math.PI / 2;
			return {
				key: index,
				label: part.label,
				colorIndex: index + 1,
				dash: `${share * circumference} ${circumference}`,
				dashOffset: -start * circumference,
				labelX: centerX + labelRadius * Math.cos(midAngle),
				labelY: centerY + labelRadius * Math.sin(midAngle),
				anchor: anchorFor(Math.cos(midAngle)),
				text: `${formatNumber(part.value, 'int')} ${part.label} (${formatNumber(share * 100, 'int')}%)`
			};
		});
	});

	const bars: BarDatum[] = $derived(parts.map((part) => ({ label: part.label, value: part.value })));

	const summary = $derived(parts.map((part) => `${part.label}: ${formatNumber(part.value, 'int')}`).join('; '));
</script>

{#if maximumSlices >= parts.length}
	<figure class={['donut-chart', className]} {...rest}>
		<svg class="donut" viewBox="0 0 {chartWidth} {chartHeight}" role="img" aria-label={`${label}. ${summary}`}>
			{#each slices as slice (slice.key)}
				<circle
					class="slice"
					data-slice={slice.label}
					cx={centerX}
					cy={centerY}
					r={ringRadius}
					fill="none"
					stroke-width="24"
					stroke-dasharray={slice.dash}
					stroke-dashoffset={slice.dashOffset}
					transform={`rotate(-90 ${centerX} ${centerY})`}
					style:stroke={`var(--viz-${slice.colorIndex})`}
				/>
				<text class="slice-label" x={slice.labelX} y={slice.labelY} text-anchor={slice.anchor} dominant-baseline="middle">{slice.text}</text>
			{/each}
		</svg>
		<table class="donut-table">
			<thead>
				<tr>
					<th scope="col">{label}</th>
					<th scope="col">{unitSymbols.count}</th>
				</tr>
			</thead>
			<tbody>
				{#each parts as part, index (index)}
					<tr>
						<td>{part.label}</td>
						<td>{formatNumber(part.value, 'int')}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</figure>
{:else}
	<BarChart data={bars} unit="count" {label} class={className} {...rest} />
{/if}

<style>
	.donut-chart {
		display: grid;
		gap: var(--space-3);
		margin: 0;
		min-inline-size: 0;
	}

	.donut {
		inline-size: 100%;
		block-size: auto;
	}

	.slice-label {
		font-family: var(--font-sans);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-variant-numeric: tabular-nums;
		fill: var(--text);
	}

	.donut-table {
		border-collapse: collapse;
		inline-size: 100%;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
	}

	.donut-table th,
	.donut-table td {
		padding: var(--space-1) var(--cell-px);
		text-align: start;
	}

	.donut-table td:last-child,
	.donut-table th:last-child {
		text-align: end;
	}
</style>
