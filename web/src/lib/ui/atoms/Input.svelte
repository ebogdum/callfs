<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLInputAttributes, HTMLInputTypeAttribute } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import type { Size } from '../types';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends Omit<HTMLInputAttributes, 'size' | 'type' | 'value' | 'prefix'> {
		value?: string;
		type?: HTMLInputTypeAttribute;
		size?: Size;
		invalid?: boolean;
		mono?: boolean;
		clearable?: boolean;
		prefix?: Snippet;
		suffix?: Snippet;
	}

	let {
		value = $bindable(''),
		type = 'text',
		size = 'md',
		invalid = false,
		mono = false,
		clearable = false,
		prefix,
		suffix,
		class: className,
		oninput,
		...rest
	}: Props = $props();

	let field: HTMLInputElement | undefined = $state();

	function handleInput(event: Event & { currentTarget: HTMLInputElement }): void {
		value = event.currentTarget.value;
		oninput?.(event);
	}

	function clear(): void {
		value = '';
		field?.focus();
	}
</script>

<span
	class={['control', className]}
	data-size={size}
	data-invalid={invalid ? 'true' : undefined}
	data-mono={mono ? 'true' : undefined}
>
	{#if prefix}<span class="addon">{@render prefix()}</span>{/if}
	<input
		bind:this={field}
		class="field"
		{type}
		value={value}
		aria-invalid={invalid ? 'true' : undefined}
		{...rest}
		oninput={handleInput}
	/>
	{#if clearable && '' !== value}
		<button type="button" class="clear" aria-label={t('common.clear')} onclick={clear}>
			<Icon name="x" size="sm" />
		</button>
	{/if}
	{#if suffix}<span class="addon">{@render suffix()}</span>{/if}
</span>

<style>
	.control {
		display: flex;
		align-items: stretch;
		min-inline-size: 0;
		block-size: var(--control-h);
		border: var(--hairline) solid var(--input-border, var(--border-control));
		border-radius: var(--input-radius, var(--radius-md));
		background: var(--input-bg, var(--surface));
		box-shadow: var(--shadow-xs);
		transition: border-color var(--dur-micro) var(--ease-out);
		color: var(--input-fg, var(--text));
		overflow: hidden;
	}

	.control:has(.field:focus-visible) {
		border-color: var(--accent-border, var(--accent));
		box-shadow: var(--focus-ring);
	}

	.control[data-invalid='true'] {
		border-color: var(--danger);
	}

	.control:has(.field:disabled) {
		border-color: var(--border-subtle);
		color: var(--text-faint);
	}

	.control[data-size='sm'] .field {
		font-size: var(--text-sm-size);
	}

	.control[data-mono='true'] .field {
		font-family: var(--font-mono);
	}

	.field {
		flex: 1 1 auto;
		min-inline-size: 0;
		inline-size: 100%;
		padding-inline: var(--control-px);
		border: 0;
		background: transparent;
		color: inherit;
		font-family: var(--font-sans);
		font-size: var(--text-input);
		line-height: var(--text-body-line);
		outline: var(--focus-width) solid transparent;
	}

	.field::placeholder {
		color: var(--text-faint);
	}

	.field:disabled {
		cursor: not-allowed;
	}

	.addon {
		display: inline-flex;
		flex: none;
		align-items: center;
		padding-inline: var(--space-2);
		background: transparent;
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		white-space: nowrap;
	}

	.addon:first-child {
		padding-inline-start: var(--control-px);
		padding-inline-end: 0;
	}

	.addon:last-child {
		padding-inline-start: 0;
		padding-inline-end: var(--space-1-5);
	}

	.addon:first-child + .field {
		padding-inline-start: var(--space-2);
	}

	@media (hover: hover) {
		.control:hover:not(:has(.field:disabled, .field:focus-visible)) {
			border-color: var(--input-border-hover, var(--text-faint));
		}
	}

	.clear {
		display: inline-flex;
		flex: none;
		align-items: center;
		justify-content: center;
		min-inline-size: var(--target-min);
		min-block-size: var(--target-min);
		align-self: center;
		padding: 0;
		border: 0;
		border-radius: var(--radius-xs);
		background: transparent;
		color: var(--text-muted);
		cursor: pointer;
	}

	.clear:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.clear:active {
		background: var(--surface-pressed);
	}

	@media (hover: hover) {
		.clear:hover {
			background: var(--surface-hover);
			color: var(--text);
		}
	}
</style>
