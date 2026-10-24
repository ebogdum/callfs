<script lang="ts" module>
	export type IconButtonVariant = 'ghost' | 'secondary';
	export type IconButtonSize = 'sm' | 'md' | 'lg';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes, MouseEventHandler } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';
	import Kbd from './Kbd.svelte';
	import Tooltip from './Tooltip.svelte';

	interface Props extends Omit<HTMLButtonAttributes, 'children'> {
		icon: IconName;
		label: string;
		variant?: IconButtonVariant;
		size?: IconButtonSize;
		pressed?: boolean;
		shortcut?: string[];
		badge?: number | string;
		disabledReason?: string;
	}

	let {
		icon,
		label,
		variant = 'ghost',
		size = 'md',
		pressed,
		shortcut,
		badge,
		disabledReason,
		type = 'button',
		class: className,
		onclick,
		...rest
	}: Props = $props();

	const reason = $derived(disabledReason ?? '');
	const blocked = $derived('' !== reason);

	const handleClick: MouseEventHandler<HTMLButtonElement> = (event) => {
		if (blocked) {
			event.preventDefault();
			return;
		}
		onclick?.(event);
	};
</script>

{#snippet tip()}
	{#if blocked}
		{reason}
	{:else}
		<span>{label}</span>
		{#if shortcut}<Kbd keys={shortcut} />{/if}
	{/if}
{/snippet}

<span class="anchor">
	<Tooltip content={tip}>
		<button
			{...rest}
			class={['icon-button', className]}
			{type}
			aria-label={label}
			aria-pressed={undefined === pressed ? undefined : pressed}
			aria-disabled={blocked ? 'true' : undefined}
			data-variant={variant}
			data-size={size}
			data-state={pressed ? 'selected' : undefined}
			onclick={handleClick}
		>
			<Icon name={icon} size={'sm' === size ? 'sm' : 'lg' === size ? 'lg' : 'md'} />
		</button>
	</Tooltip>
	{#if undefined !== badge}<span class="badge">{badge}</span>{/if}
</span>

<style>
	.anchor {
		position: relative;
		display: inline-flex;
	}

	.icon-button {
		display: inline-grid;
		place-items: center;
		flex: none;
		inline-size: var(--control-h);
		block-size: var(--control-h);
		min-inline-size: var(--target-min);
		min-block-size: var(--target-min);
		padding: 0;
		border: var(--hairline) solid transparent;
		border-radius: var(--radius-md);
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
		transition: background var(--dur-micro) var(--ease-out);
	}

	.icon-button[data-size='sm'] {
		inline-size: calc(var(--control-h) - var(--space-1));
		block-size: calc(var(--control-h) - var(--space-1));
	}

	.icon-button[data-size='lg'] {
		inline-size: calc(var(--control-h) + var(--space-2));
		block-size: calc(var(--control-h) + var(--space-2));
	}

	.icon-button[data-variant='secondary'] {
		background: var(--surface-raised);
		border-color: var(--border-control);
		color: var(--text);
	}

	.icon-button[data-state='selected'] {
		background: var(--surface-selected);
		color: var(--accent-text);
	}

	@media (hover: hover) {
		.icon-button:hover:not([aria-disabled='true']) {
			background: var(--surface-hover);
			color: var(--text);
		}
	}

	.icon-button:active:not([aria-disabled='true']) {
		background: var(--surface-pressed);
	}

	.icon-button:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.icon-button[aria-disabled='true'] {
		cursor: not-allowed;
		background: var(--surface-sunken);
		color: var(--text-faint);
	}

	.badge {
		position: absolute;
		inset-block-start: calc(var(--space-1) * -1);
		inset-inline-end: calc(var(--space-1) * -1);
		display: inline-grid;
		place-items: center;
		min-inline-size: var(--space-4);
		block-size: var(--space-4);
		padding-inline: var(--space-0-5);
		border-radius: var(--radius-full);
		background: var(--accent);
		color: var(--text-on-accent);
		font-size: var(--text-caption-size);
		font-weight: var(--text-caption-weight);
		line-height: 1;
		box-shadow: 0 0 0 var(--hairline) var(--surface);
		pointer-events: none;
	}

	@media (pointer: coarse) {
		.icon-button {
			min-inline-size: var(--touch-target);
			min-block-size: var(--touch-target);
		}
	}
</style>
