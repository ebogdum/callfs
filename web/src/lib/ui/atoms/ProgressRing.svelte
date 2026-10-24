<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Size } from '../types';
	import { formatNumber } from '$lib/format/number';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		value: number;
		max?: number;
		label: string;
		size?: Size;
		class?: string;
	}

	let { value, max = 100, label, size = 'md', class: className, ...rest }: Props = $props();

	const current = $derived(Math.min(max, Math.max(0, value)));
	const percent = $derived(0 < max ? (current / max) * 100 : 0);
	const valueText = $derived(`${formatNumber(percent, 'int')} %`);
</script>

<span
	class={['progress-ring', className]}
	data-size={size}
	role="progressbar"
	aria-label={label}
	aria-valuemin={0}
	aria-valuemax={max}
	aria-valuenow={current}
	aria-valuetext={valueText}
	{...rest}
>
	<svg viewBox="0 0 36 36" aria-hidden="true">
		<circle class="track" cx="18" cy="18" r="16" pathLength="100" />
		<circle class="fill" cx="18" cy="18" r="16" pathLength="100" stroke-dasharray="100" style:stroke-dashoffset={100 - percent} />
	</svg>
</span>

<style>
	.progress-ring {
		--progress-ring-box: var(--space-8);
		display: inline-flex;
		flex: none;
		inline-size: var(--progress-ring-size, var(--progress-ring-box));
		block-size: var(--progress-ring-size, var(--progress-ring-box));
	}

	.progress-ring[data-size='sm'] {
		--progress-ring-box: var(--space-5);
	}

	.progress-ring[data-size='lg'] {
		--progress-ring-box: var(--space-12);
	}

	svg {
		inline-size: 100%;
		block-size: 100%;
		overflow: visible;
		transform: rotate(-90deg);
	}

	circle {
		fill: none;
		stroke-width: 3;
	}

	.track {
		stroke: var(--progress-ring-track, var(--surface-sunken));
	}

	.fill {
		stroke: var(--progress-ring-color, var(--accent));
		transition: stroke-dashoffset var(--dur-medium) var(--ease-out);
	}
</style>
