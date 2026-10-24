<script lang="ts" module>
	export const TOKENS = ['--spinner-color', '--spinner-size'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import VisuallyHidden from './VisuallyHidden.svelte';
	import { holdMinimum } from './motion';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class'> {
		size?: 'sm' | 'md' | 'lg';
		label?: string;
		delay?: number;
		class?: string;
	}

	let { size = 'md', label, delay = 150, class: className = '', ...rest }: Props = $props();

	let visible = $state(false);
	let shownAt = 0;

	$effect(() => {
		const timer = setTimeout(() => {
			shownAt = Date.now();
			visible = true;
		}, delay);
		return () => clearTimeout(timer);
	});
</script>

{#if visible}
	<span
		class="spinner {className}"
		data-size={size}
		role={label ? 'status' : undefined}
		aria-hidden={label ? undefined : 'true'}
		out:holdMinimum|global={{ shownAt }}
		{...rest}
	>
		<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9" /></svg>
		{#if label}<VisuallyHidden>{label}</VisuallyHidden>{/if}
	</span>
{/if}

<style>
	.spinner {
		--spinner-box: var(--space-5);
		display: inline-flex;
		flex: none;
		inline-size: var(--spinner-size, var(--spinner-box));
		block-size: var(--spinner-size, var(--spinner-box));
		color: var(--spinner-color, currentColor);
		animation: spin var(--dur-shimmer) linear infinite;
	}

	.spinner[data-size='sm'] {
		--spinner-box: var(--space-4);
	}

	.spinner[data-size='lg'] {
		--spinner-box: var(--space-6);
	}

	.spinner circle {
		fill: none;
		stroke: currentColor;
		stroke-width: 2;
		stroke-linecap: round;
		stroke-dasharray: 42 18;
	}

	@media (prefers-reduced-motion: reduce) {
		:global(html:not([data-motion='full'])) .spinner {
			animation: none;
		}
	}

	:global(html[data-motion='reduced']) .spinner,
	:global(html[data-motion='none']) .spinner {
		animation: none;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
