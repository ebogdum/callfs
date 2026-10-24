<script lang="ts">
	import type { HTMLButtonAttributes, MouseEventHandler } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';

	interface Props extends Omit<HTMLButtonAttributes, 'children'> {
		pressed: boolean;
		label: string;
		icon?: IconName;
	}

	let { pressed = $bindable(), label, icon, type = 'button', class: className, onclick, ...rest }: Props = $props();

	const handleClick: MouseEventHandler<HTMLButtonElement> = (event) => {
		pressed = !pressed;
		onclick?.(event);
	};
</script>

<button
	{...rest}
	class={['toggle-button', className]}
	{type}
	aria-pressed={pressed}
	data-state={pressed ? 'selected' : undefined}
	onclick={handleClick}
>
	{#if icon}<Icon name={icon} size="md" />{/if}
	<span class="label">{label}</span>
</button>

<style>
	.toggle-button {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		block-size: var(--control-h);
		min-inline-size: var(--target-min);
		padding-inline: var(--control-px);
		border: var(--hairline) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface-raised);
		color: var(--text);
		font: inherit;
		font-size: var(--text-body-size);
		font-weight: var(--text-body-weight);
		white-space: nowrap;
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
		transition: background var(--dur-micro) var(--ease-out);
	}

	@media (hover: hover) {
		.toggle-button:hover:not(:disabled) {
			background: var(--surface-hover);
		}
	}

	.toggle-button:active:not(:disabled) {
		background: var(--surface-pressed);
	}

	.toggle-button:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.toggle-button[data-state='selected'] {
		border-color: var(--accent-border);
		background: var(--accent-soft);
		color: var(--accent-text);
	}

	.toggle-button:disabled {
		cursor: not-allowed;
		background: var(--surface-sunken);
		border-color: var(--border-subtle);
		color: var(--text-faint);
	}

	@media (pointer: coarse) {
		.toggle-button {
			min-block-size: var(--touch-target);
		}
	}
</style>
