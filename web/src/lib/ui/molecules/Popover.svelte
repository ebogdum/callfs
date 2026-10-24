<script lang="ts" module>
	export type PopoverSide = 'top' | 'right' | 'bottom' | 'left';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { Popover as PopoverPrimitive } from 'bits-ui';

	const POPOVER_OFFSET_PX = 8;
	const POPOVER_COLLISION_PADDING_PX = 8;

	interface Props extends HTMLAttributes<HTMLDivElement> {
		trigger: Snippet<[Record<string, unknown>]>;
		content: Snippet;
		label: string;
		side?: PopoverSide;
		wide?: boolean;
		class?: string;
	}

	let { trigger, content, label, side = 'bottom', wide = false, class: className, ...rest }: Props = $props();
</script>

<PopoverPrimitive.Root>
	<PopoverPrimitive.Trigger>
		{#snippet child({ props })}
			{@render trigger(props)}
		{/snippet}
	</PopoverPrimitive.Trigger>
	<PopoverPrimitive.Portal>
		<PopoverPrimitive.Content
			{side}
			sideOffset={POPOVER_OFFSET_PX}
			collisionPadding={POPOVER_COLLISION_PADDING_PX}
			class={['popover', className]}
			data-wide={wide ? 'true' : undefined}
			role="dialog"
			aria-label={label}
		>
			<div class="popover-body" {...rest}>
				{@render content()}
			</div>
		</PopoverPrimitive.Content>
	</PopoverPrimitive.Portal>
</PopoverPrimitive.Root>

<style>
	:global(.popover) {
		z-index: var(--z-dropdown);
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: calc(var(--space-16) * 2.5);
		max-inline-size: min(calc(var(--space-16) * 5), calc(var(--viewport-w) - var(--space-4)));
		max-block-size: min(calc(var(--space-16) * 6), calc(var(--viewport-h) - var(--space-8)));
		overflow-y: auto;
		padding: var(--space-3);
		color: var(--popover-fg, var(--text));
		background: var(--popover-bg, var(--surface-overlay));
		border: var(--hairline) solid var(--popover-border, var(--border-subtle));
		border-radius: var(--popover-radius, var(--radius-lg));
		box-shadow: var(--shadow-lg);
	}

	:global(.popover[data-wide='true']) {
		max-inline-size: calc(var(--viewport-w) - var(--space-4));
	}
</style>
