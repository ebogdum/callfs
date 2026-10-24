<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Chip } from '../types';
	import Icon from '../quarks/Icon.svelte';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		chip: Chip;
		onremove: (chip: Chip) => void;
		onedit?: (chip: Chip) => void;
	}

	let { chip, onremove, onedit, class: className, ...rest }: Props = $props();

	function handleKeydown(event: KeyboardEvent) {
		if ('Backspace' === event.key) {
			event.preventDefault();
			onremove(chip);
		}
	}
</script>

<span class={['filter-chip', className]} onkeydown={handleKeydown} {...rest}>
	{#if onedit}
		<button
			type="button"
			class="label"
			aria-label={t('u076molecules.chip_edit', { label: chip.label })}
			onclick={() => onedit?.(chip)}>{chip.label}</button
		>
	{:else}
		<span class="label">{chip.label}</span>
	{/if}
	<button
		type="button"
		class="remove"
		aria-label={t('u076molecules.chip_remove', { label: chip.label })}
		onclick={() => onremove(chip)}
	>
		<Icon name="x" size="sm" />
	</button>
</span>

<style>
	.filter-chip {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		max-inline-size: 100%;
		min-block-size: var(--control-h);
		padding-inline: var(--space-2) var(--space-1);
		border: var(--hairline) solid var(--chip-border, var(--accent-border));
		border-radius: var(--chip-radius, var(--radius-xs));
		background: var(--chip-bg, var(--accent-soft));
		color: var(--chip-fg, var(--accent-text));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.label {
		min-inline-size: 0;
		overflow: hidden;
		padding: 0;
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		text-align: start;
		text-overflow: ellipsis;
		white-space: nowrap;
		cursor: pointer;
	}

	.remove {
		display: inline-grid;
		flex: none;
		place-items: center;
		min-inline-size: var(--target-min);
		min-block-size: var(--target-min);
		padding: 0;
		border: 0;
		border-radius: var(--radius-full);
		background: none;
		color: inherit;
		cursor: pointer;
	}

	@media (hover: hover) {
		.remove:hover {
			background: var(--surface-hover);
		}
	}

	.remove:active {
		background: var(--surface-pressed);
	}

	@media (pointer: coarse) {
		.filter-chip {
			min-block-size: var(--touch-target);
		}

		.remove {
			min-inline-size: var(--touch-target);
			min-block-size: var(--touch-target);
		}
	}

	.label:focus-visible,
	.remove:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}
</style>
