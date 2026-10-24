<script lang="ts" module>
	export const TOKENS = ['--sparkline-cell-stroke'] as const;

	const WIDTH = 64;
	const HEIGHT = 20;
	const TREND_KEYS = { rising: 'trends.rising', falling: 'trends.falling', flat: 'trends.flat' } as const;
	type Trend = keyof typeof TREND_KEYS;
</script>

<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';
	import type { Tone } from '../types';
	import { t } from '$lib/copy/t';

	interface Props extends Omit<SVGAttributes<SVGSVGElement>, 'class' | 'values'> {
		values: (number | null)[];
		tone?: Tone;
		class?: string;
	}

	let { values, tone = 'accent', class: className, ...rest }: Props = $props();

	const plotted = $derived(values.flatMap((value, index) => ('number' === typeof value && Number.isFinite(value) ? [{ index, value }] : [])));
	const lowest = $derived(0 < plotted.length ? Math.min(...plotted.map(({ value }) => value)) : 0);
	const highest = $derived(0 < plotted.length ? Math.max(...plotted.map(({ value }) => value)) : 0);
	const step = $derived(1 < values.length ? WIDTH / (values.length - 1) : 0);
	const coordinates = $derived(
		plotted.map(({ index, value }) => ({
			x: index * step,
			y: 0 < highest - lowest ? HEIGHT - ((value - lowest) / (highest - lowest)) * HEIGHT : HEIGHT / 2,
		}))
	);
	const polyline = $derived(coordinates.map(({ x, y }) => `${x},${y}`).join(' '));
	const latest = $derived(coordinates.at(-1));
	const trend: Trend = $derived.by(() => {
		const first = plotted.at(0);
		const last = plotted.at(-1);
		if (undefined === first || undefined === last || last.value === first.value) {
			return 'flat';
		}
		return last.value > first.value ? 'rising' : 'falling';
	});
	const summary = $derived(0 < plotted.length ? t('trends.summary', { trend: t(TREND_KEYS[trend]), count: values.length }) : t('trends.empty'));
</script>

<svg
	class={['sparkline-cell', className]}
	data-tone={tone}
	width={WIDTH}
	height={HEIGHT}
	viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
	role="img"
	aria-label={summary}
	{...rest}
>
	{#if 0 < coordinates.length}
		<polyline points={polyline} />
	{/if}
	{#if undefined !== latest}
		<circle cx={latest.x} cy={latest.y} r="1.5" />
	{/if}
</svg>

<style>
	.sparkline-cell {
		--sparkline-tone: var(--accent);
		display: inline-block;
		flex: none;
		overflow: visible;
		vertical-align: middle;
		color: var(--sparkline-cell-stroke, var(--sparkline-tone));
	}

	.sparkline-cell[data-tone='neutral'] {
		--sparkline-tone: var(--text-muted);
	}

	.sparkline-cell[data-tone='ok'] {
		--sparkline-tone: var(--ok);
	}

	.sparkline-cell[data-tone='warn'] {
		--sparkline-tone: var(--warn);
	}

	.sparkline-cell[data-tone='danger'] {
		--sparkline-tone: var(--danger);
	}

	.sparkline-cell[data-tone='info'] {
		--sparkline-tone: var(--info);
	}

	polyline {
		fill: none;
		stroke: currentColor;
		stroke-width: 1.5;
		stroke-linecap: round;
		stroke-linejoin: round;
		vector-effect: non-scaling-stroke;
	}

	circle {
		fill: currentColor;
	}
</style>
