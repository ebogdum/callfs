<script lang="ts" module>
	export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
	export type ButtonSize = 'sm' | 'md' | 'lg';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes, MouseEventHandler } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import Spinner from '../quarks/Spinner.svelte';
	import type { IconName } from '../types';
	import Kbd from './Kbd.svelte';
	import Tooltip from './Tooltip.svelte';

	interface Props extends HTMLButtonAttributes {
		variant?: ButtonVariant;
		size?: ButtonSize;
		loading?: boolean;
		iconStart?: IconName;
		iconEnd?: IconName;
		shortcut?: string[];
		full?: boolean;
		disabledReason?: string;
		children: Snippet;
	}

	let {
		variant = 'secondary',
		size = 'md',
		loading = false,
		iconStart,
		iconEnd,
		shortcut,
		full = false,
		disabledReason,
		type = 'button',
		class: className,
		onclick,
		children,
		...rest
	}: Props = $props();

	const reason = $derived(disabledReason ?? '');
	const blocked = $derived('' !== reason);

	const handleClick: MouseEventHandler<HTMLButtonElement> = (event) => {
		if (blocked || loading) {
			event.preventDefault();
			return;
		}
		onclick?.(event);
	};
</script>

{#snippet control()}
	<button
		{...rest}
		class={['button', className]}
		{type}
		data-variant={variant}
		data-size={size}
		data-full={full ? '' : undefined}
		data-state={loading ? 'loading' : undefined}
		aria-busy={loading ? 'true' : undefined}
		aria-disabled={blocked ? 'true' : undefined}
		onclick={handleClick}
	>
		<span class="content">
			{#if iconStart}<Icon name={iconStart} size={'sm' === size ? 'sm' : 'md'} />{/if}
			<span class="label">{@render children()}</span>
			{#if iconEnd}<Icon name={iconEnd} size={'sm' === size ? 'sm' : 'md'} />{/if}
			{#if shortcut && 'sm' !== size}<Kbd keys={shortcut} />{/if}
		</span>
		{#if loading}
			<span class="spinner-slot"><Spinner size="sm" /></span>
		{/if}
	</button>
{/snippet}

{#if blocked}
	<Tooltip content={reason}>
		{@render control()}
	</Tooltip>
{:else}
	{@render control()}
{/if}

<style>
	.button {
		position: relative;
		display: inline-grid;
		place-items: center;
		font: inherit;
		font-weight: var(--text-control-weight);
		font-size: var(--text-body-size);
		line-height: 1;
		min-inline-size: var(--target-min);
		block-size: var(--control-h);
		padding-inline: var(--control-px);
		border: var(--hairline) solid transparent;
		border-radius: var(--button-radius, var(--radius-md));
		background: transparent;
		color: var(--text);
		cursor: pointer;
		white-space: nowrap;
		transition:
			background var(--dur-micro) var(--ease-out),
			border-color var(--dur-micro) var(--ease-out),
			transform var(--dur-micro) var(--ease-out);
		outline: var(--focus-width) solid transparent;
	}

	.button[data-size='sm'] {
		block-size: calc(var(--control-h) - var(--space-1));
		padding-inline: var(--space-3);
		font-size: var(--text-sm-size);
	}

	.button[data-size='sm'] .content {
		gap: var(--space-1-5);
	}

	.button[data-size='lg'] {
		block-size: calc(var(--control-h) + var(--space-2));
	}

	.button[data-full] {
		inline-size: 100%;
	}

	.button[data-variant='primary'] {
		background: var(--accent);
		color: var(--text-on-accent);
	}

	.button[data-variant='secondary'] {
		background: var(--surface-raised);
		border-color: var(--border-control);
		box-shadow: var(--shadow-xs);
	}

	.button[data-variant='primary'],
	.button[data-variant='danger'] {
		box-shadow: var(--shadow-xs);
	}

	.button[data-variant='ghost'] {
		background: transparent;
	}

	.button[data-variant='danger'] {
		background: var(--danger);
		color: var(--text-on-accent);
	}

	@media (hover: hover) {
		.button[data-variant='primary']:hover:not([aria-disabled='true'], [aria-busy='true']) {
			background: var(--accent-hover);
		}

		.button[data-variant='secondary']:hover:not([aria-disabled='true'], [aria-busy='true']),
		.button[data-variant='ghost']:hover:not([aria-disabled='true'], [aria-busy='true']) {
			background: var(--surface-hover);
		}

		.button[data-variant='danger']:hover:not([aria-disabled='true'], [aria-busy='true']) {
			background: var(--danger-soft);
			color: var(--danger-text);
		}
	}

	.button:active:not([aria-disabled='true'], [aria-busy='true']) {
		background: var(--surface-pressed);
	}

	.button[data-variant='primary']:active:not([aria-disabled='true'], [aria-busy='true']) {
		background: var(--accent-pressed);
	}

	.button:active:not([aria-disabled='true'], [aria-busy='true']) {
		transform: scale(0.98);
	}

	.button[aria-pressed='true']:not([aria-disabled='true']) {
		border-color: var(--accent-border);
		background: var(--accent-soft);
		color: var(--accent-text);
	}

	@media (hover: hover) {
		.button[aria-pressed='true']:hover:not([aria-disabled='true'], [aria-busy='true']) {
			background: var(--accent-soft-hover);
		}
	}

	.button:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.button[aria-disabled='true'],
	.button[aria-busy='true'] {
		cursor: not-allowed;
	}

	.button[aria-disabled='true'] {
		background: var(--surface-sunken);
		border-color: var(--border-subtle);
		color: var(--text-faint);
	}

	.button[aria-disabled='true']:not(:focus-visible) {
		box-shadow: none;
	}

	.button[data-variant='ghost'][aria-disabled='true'] {
		background: transparent;
		border-color: transparent;
	}

	.button[aria-busy='true'] {
		cursor: progress;
	}

	.content {
		grid-area: 1 / 1;
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.button[data-state='loading'] .content {
		opacity: 0;
	}

	.spinner-slot {
		grid-area: 1 / 1;
		display: inline-flex;
	}

	.label {
		white-space: nowrap;
	}

	@media (pointer: coarse) {
		.button {
			min-block-size: var(--touch-target);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.button,
		.button:active {
			transform: none;
		}
	}

	:global(html[data-motion='reduced']) .button:active,
	:global(html[data-motion='none']) .button:active {
		transform: none;
	}
</style>
