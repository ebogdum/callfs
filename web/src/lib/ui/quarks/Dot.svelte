<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Tone } from '../types';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		tone: Tone;
		pulse?: boolean;
	}

	let { tone, pulse = false, class: className, ...rest }: Props = $props();
</script>

<span class={['dot', className]} data-tone={tone} data-pulse={pulse} aria-hidden="true" {...rest}></span>

<style>
	.dot {
		display: inline-block;
		flex: none;
		inline-size: var(--space-2);
		block-size: var(--space-2);
		border-radius: var(--radius-full);
		background: var(--text-muted);
	}

	.dot[data-tone='accent'] {
		background: var(--accent);
	}

	.dot[data-tone='ok'] {
		background: var(--ok);
	}

	.dot[data-tone='warn'] {
		background: var(--warn);
	}

	.dot[data-tone='danger'] {
		background: var(--danger);
	}

	.dot[data-tone='info'] {
		background: var(--info);
	}

	.dot[data-pulse='true'] {
		animation: pulse var(--dur-shimmer) var(--ease-in-out) infinite;
	}

	@keyframes pulse {
		50% {
			opacity: 0.4;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dot[data-pulse='true'] {
			animation: none;
		}
	}

	:global([data-motion='reduced']) .dot[data-pulse='true'] {
		animation: none;
	}
</style>
