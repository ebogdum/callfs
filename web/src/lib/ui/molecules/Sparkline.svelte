<script lang="ts" module>
	export type SparklineHeight = 'sm' | 'md';
	export const TOKENS = ['--sparkline-stroke', '--sparkline-w', '--sparkline-h'] as const;

	const VIEW_WIDTH = 64;
	const VIEW_HEIGHT = 20;
	const INSET = 1;
	const CIRCLE_RADIUS = 1.5;
</script>

<script lang="ts">
	import type { SVGAttributes } from 'svelte/elements';
	import type { Tone } from '../types';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';

	interface Props extends Omit<SVGAttributes<SVGSVGElement>, 'class' | 'values'> {
		values: (number | null)[];
		tone?: Tone;
		height?: SparklineHeight;
		class?: string;
	}

	interface Point {
		x: number;
		y: number;
	}

	let { values, tone = 'accent', height = 'sm', class: className, ...rest }: Props = $props();

	const present = $derived(values.filter((amount): amount is number => 'number' === typeof amount && Number.isFinite(amount)));
	const low = $derived(0 === present.length ? 0 : Math.min(...present));
	const high = $derived(0 === present.length ? 0 : Math.max(...present));
	const first = $derived(present.at(0) ?? 0);
	const last = $derived(present.at(-1) ?? 0);
	const range = $derived(high - low);

	const trendKey = $derived(
		first < last ? 'u077molecules.trend_rising' : last < first ? 'u077molecules.trend_falling' : 'u077molecules.trend_flat'
	);

	const summary = $derived(
		0 === present.length
			? t('u077molecules.no_points')
			: t('u077molecules.spark_summary', {
					trend: t(trendKey),
					last: formatNumber(last, 'decimal'),
					low: formatNumber(low, 'decimal'),
					high: formatNumber(high, 'decimal'),
				})
	);

	const pointAt = (index: number, amount: number): Point => ({
		x: values.length < 2 ? VIEW_WIDTH / 2 : (index * VIEW_WIDTH) / (values.length - 1),
		y: 0 === range ? VIEW_HEIGHT / 2 : VIEW_HEIGHT - INSET - ((amount - low) / range) * (VIEW_HEIGHT - 2 * INSET),
	});

	const segments = $derived.by(() => {
		const runs: Point[][] = [];
		let run: Point[] = [];
		values.forEach((amount, index) => {
			if ('number' !== typeof amount || !Number.isFinite(amount)) {
				if (0 < run.length) {
					runs.push(run);
				}
				run = [];
				return;
			}
			run.push(pointAt(index, amount));
		});
		if (0 < run.length) {
			runs.push(run);
		}
		return runs;
	});
</script>

<svg
	class={['sparkline', className]}
	data-tone={tone}
	data-height={height}
	viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`}
	preserveAspectRatio="none"
	role="img"
	aria-label={summary}
	{...rest}
>
	{#each segments as run, index (index)}
		{#if 1 < run.length}
			<polyline points={run.map((point) => `${point.x},${point.y}`).join(' ')} />
		{:else}
			<circle cx={run[0].x} cy={run[0].y} r={CIRCLE_RADIUS} />
		{/if}
	{/each}
</svg>

<style>
	.sparkline {
		--sparkline-tone: var(--accent);
		display: block;
		inline-size: var(--sparkline-w, var(--space-16));
		block-size: var(--sparkline-h, var(--space-5));
		max-inline-size: 100%;
		overflow: visible;
	}

	.sparkline[data-height='md'] {
		block-size: var(--sparkline-h, var(--space-6));
	}

	.sparkline[data-tone='neutral'] {
		--sparkline-tone: var(--text-muted);
	}

	.sparkline[data-tone='ok'] {
		--sparkline-tone: var(--ok);
	}

	.sparkline[data-tone='warn'] {
		--sparkline-tone: var(--warn);
	}

	.sparkline[data-tone='danger'] {
		--sparkline-tone: var(--danger);
	}

	.sparkline[data-tone='info'] {
		--sparkline-tone: var(--info);
	}

	polyline {
		fill: none;
		stroke: var(--sparkline-stroke, var(--sparkline-tone));
		stroke-width: var(--space-0-5);
		stroke-linecap: round;
		stroke-linejoin: round;
		vector-effect: non-scaling-stroke;
	}

	circle {
		fill: var(--sparkline-stroke, var(--sparkline-tone));
	}
</style>
