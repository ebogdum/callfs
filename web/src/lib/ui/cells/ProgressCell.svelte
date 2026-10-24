<script lang="ts" module>
	export const TOKENS = ['--progress-cell-gap', '--progress-cell-w', '--progress-cell-text'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Tone } from '../types';
	import type { StatusKey } from '$lib/status/catalog';
	import { STATUS } from '$lib/status/catalog';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatNumber } from '$lib/format/number';
	import ProgressBar from '../atoms/ProgressBar.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		done: number;
		total: number | null;
		state: StatusKey;
		bytesPerSecond?: number | null;
		class?: string;
	}

	let { done, total, state, bytesPerSecond, class: className, ...rest }: Props = $props();

	const entry = $derived(STATUS[state]);
	const tone: Tone = $derived('progress' === entry.tone ? 'accent' : 'muted' === entry.tone ? 'neutral' : entry.tone);
	const countText = $derived(null === total ? '' : `${formatNumber(done, 'int')} ${t('number.of')} ${formatNumber(total, 'int')}`);
	const speed = $derived(Number.isFinite(bytesPerSecond) ? `${formatBytes(bytesPerSecond ?? null).text}/s` : null);
</script>

<div class={['progress-cell', className]} data-tone={tone} data-mode={null === total ? 'bytes' : 'count'} {...rest}>
	<span class="bar">
		<ProgressBar
			value={null === total ? undefined : done}
			max={null === total ? 100 : total}
			label={t(entry.word)}
			{tone}
			indeterminate={null === total}
		/>
	</span>
	{#if null === total}
		<span class="bytes">{formatBytes(done).text}</span>
		{#if null !== speed}<span class="speed">{speed}</span>{/if}
	{:else}
		<span class="count">{countText}</span>
	{/if}
</div>

<style>
	.progress-cell {
		--progress-cell-tone: var(--accent);
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--progress-cell-gap, var(--space-2));
		max-inline-size: 100%;
		min-inline-size: 0;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
		color: var(--progress-cell-text, var(--text));
	}

	.progress-cell[data-tone='neutral'] {
		--progress-cell-tone: var(--text-muted);
	}

	.progress-cell[data-tone='ok'] {
		--progress-cell-tone: var(--ok);
	}

	.progress-cell[data-tone='warn'] {
		--progress-cell-tone: var(--warn);
	}

	.progress-cell[data-tone='danger'] {
		--progress-cell-tone: var(--danger);
	}

	.progress-cell[data-tone='info'] {
		--progress-cell-tone: var(--info);
	}

	.bar {
		flex: 0 1 var(--progress-cell-w, var(--space-16));
		min-inline-size: 0;
	}

	.bytes,
	.count {
		flex: none;
		white-space: nowrap;
	}

	.speed {
		flex: none;
		white-space: nowrap;
		color: var(--text-muted);
	}
</style>
