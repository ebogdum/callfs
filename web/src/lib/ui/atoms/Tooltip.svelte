<script lang="ts" module>
	export type TooltipSide = 'top' | 'right' | 'bottom' | 'left';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { Tooltip as TooltipPrimitive } from 'bits-ui';

	const TOOLTIP_OFFSET_PX = 8;

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		content: string | Snippet;
		side?: TooltipSide;
		delay?: number;
		children: Snippet;
	}

	let { content, side = 'top', delay = 500, class: className, children, ...rest }: Props = $props();

	let open = $state(false);
</script>

<TooltipPrimitive.Provider delayDuration={delay}>
	<TooltipPrimitive.Root bind:open>
		<TooltipPrimitive.Trigger tabindex={-1}>
			{#snippet child({ props })}
				<span
					class={['tooltip-trigger', className]}
					{...props}
					{...rest}
					onfocusin={() => (open = true)}
					onfocusout={() => (open = false)}
				>
					{@render children()}
				</span>
			{/snippet}
		</TooltipPrimitive.Trigger>
		<TooltipPrimitive.Content {side} sideOffset={TOOLTIP_OFFSET_PX} class="tooltip" role="tooltip">
			{#if 'string' === typeof content}
				{content}
			{:else}
				{@render content()}
			{/if}
		</TooltipPrimitive.Content>
	</TooltipPrimitive.Root>
</TooltipPrimitive.Provider>

<style>
	.tooltip-trigger {
		display: inline-flex;
		max-inline-size: 100%;
	}

	:global(.tooltip) {
		z-index: var(--z-tooltip);
		max-inline-size: min(calc(var(--ch) * 32), calc(var(--viewport-w) - var(--space-4)));
		padding-block: var(--space-1);
		padding-inline: var(--space-2);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--tooltip-radius, var(--radius-sm));
		background: var(--tooltip-bg, var(--surface-overlay));
		color: var(--tooltip-fg, var(--text));
		font-size: var(--text-sm-size);
		line-height: 1.4;
		box-shadow: var(--shadow-md);
		overflow-wrap: anywhere;
	}
</style>
