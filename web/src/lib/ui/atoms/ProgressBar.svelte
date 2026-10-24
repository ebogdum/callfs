<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Tone } from '../types';
	import { formatNumber } from '$lib/format/number';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		value?: number;
		max?: number;
		label: string;
		tone?: Tone;
		indeterminate?: boolean;
		showValue?: boolean;
		class?: string;
	}

	let { value, max = 100, label, tone = 'accent', indeterminate = false, showValue = false, class: className, ...rest }: Props = $props();

	const isIndeterminate = $derived(indeterminate || undefined === value);
	const current = $derived(Math.min(max, Math.max(0, value ?? 0)));
	const percent = $derived(0 < max ? (current / max) * 100 : 0);
	const valueText = $derived(`${formatNumber(percent, 'int')} %`);
</script>

<div class={['progress-bar', className]} data-tone={tone} data-indeterminate={isIndeterminate} {...rest}>
	<div
		class="track"
		role="progressbar"
		aria-label={label}
		aria-valuemin={0}
		aria-valuemax={max}
		aria-valuenow={isIndeterminate ? undefined : current}
		aria-valuetext={isIndeterminate ? undefined : valueText}
	>
		<span class="fill" style:inline-size={isIndeterminate ? '100%' : `${percent}%`}></span>
	</div>
	{#if showValue && !isIndeterminate}<span class="value">{valueText}</span>{/if}
</div>

<style>
	.progress-bar {
		--progress-bar-tone: var(--accent);
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.progress-bar[data-tone='neutral'] {
		--progress-bar-tone: var(--text-muted);
	}

	.progress-bar[data-tone='ok'] {
		--progress-bar-tone: var(--ok);
	}

	.progress-bar[data-tone='warn'] {
		--progress-bar-tone: var(--warn);
	}

	.progress-bar[data-tone='danger'] {
		--progress-bar-tone: var(--danger);
	}

	.progress-bar[data-tone='info'] {
		--progress-bar-tone: var(--info);
	}

	.track {
		flex: 1 1 auto;
		min-inline-size: 0;
		block-size: var(--progress-bar-h, var(--space-1-5));
		border-radius: var(--radius-full);
		background: var(--progress-bar-track, var(--surface-sunken));
		overflow: hidden;
	}

	.fill {
		display: block;
		block-size: 100%;
		border-radius: var(--radius-full);
		background: var(--progress-bar-fill, var(--progress-bar-tone));
		transition: inline-size var(--dur-medium) var(--ease-out);
	}

	.progress-bar[data-indeterminate='true'] .fill {
		animation: sweep var(--dur-shimmer) var(--ease-in-out) infinite alternate;
	}

	.value {
		flex: none;
		color: var(--progress-bar-value-color, var(--text-muted));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-variant-numeric: tabular-nums;
	}

	@media (prefers-reduced-motion: reduce) {
		:global(html:not([data-motion='full'])) .progress-bar .fill {
			animation: none;
		}
	}

	:global(html[data-motion='reduced']) .progress-bar .fill,
	:global(html[data-motion='none']) .progress-bar .fill {
		animation: none;
	}

	@keyframes sweep {
		from {
			opacity: 0.25;
		}

		to {
			opacity: 1;
		}
	}
</style>
