<script lang="ts" module>
	export const TOKENS = ['--bar-cell-track', '--bar-cell-fill', '--bar-cell-w', '--bar-cell-h', '--bar-cell-gap'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Tone, Unit } from '../types';
	import Bytes from '../atoms/Bytes.svelte';
	import Duration from '../atoms/Duration.svelte';
	import Num from '../atoms/Num.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		value: number;
		max: number;
		tone?: Tone;
		unit?: Unit;
		class?: string;
	}

	let { value, max, tone = 'accent', unit = 'none', class: className, ...rest }: Props = $props();

	const fraction = $derived(0 < max && Number.isFinite(value) ? Math.min(1, Math.max(0, value / max)) : 0);
	const numberUnit = $derived('percent' === unit ? '%' : 'per_s' === unit ? '/s' : undefined);
</script>

<span class={['bar-cell', className]} data-tone={tone} {...rest}>
	<span class="track">
		<span class="fill" style:width={`${fraction * 100}%`}></span>
	</span>
	<span class="value">
		{#if 'ms' === unit || 's' === unit}
			<Duration value={'s' === unit ? value * 1000 : value} style="short" />
		{:else if 'bytes' === unit || 'bytes_per_s' === unit}
			<Bytes {value} exactOnHover={false} />{#if 'bytes_per_s' === unit}/s{/if}
		{:else}
			<Num {value} unit={numberUnit} />
		{/if}
	</span>
</span>

<style>
	.bar-cell {
		--bar-cell-tone: var(--accent);
		display: inline-flex;
		align-items: center;
		gap: var(--bar-cell-gap, var(--space-2));
		max-inline-size: 100%;
		min-inline-size: 0;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
		color: var(--text);
	}

	.bar-cell[data-tone='neutral'] {
		--bar-cell-tone: var(--text-muted);
	}

	.bar-cell[data-tone='ok'] {
		--bar-cell-tone: var(--ok);
	}

	.bar-cell[data-tone='warn'] {
		--bar-cell-tone: var(--warn);
	}

	.bar-cell[data-tone='danger'] {
		--bar-cell-tone: var(--danger);
	}

	.bar-cell[data-tone='info'] {
		--bar-cell-tone: var(--info);
	}

	.track {
		flex: 0 1 var(--bar-cell-w, var(--space-16));
		min-inline-size: 0;
		block-size: var(--bar-cell-h, var(--space-1-5));
		border-radius: var(--radius-full);
		background: var(--bar-cell-track, var(--surface-sunken));
		overflow: hidden;
	}

	.fill {
		display: block;
		block-size: 100%;
		border-radius: var(--radius-full);
		background: var(--bar-cell-fill, var(--bar-cell-tone));
	}

	.value {
		flex: none;
		white-space: nowrap;
	}
</style>
