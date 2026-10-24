<script lang="ts" module>
	export interface SelectOption {
		value: string;
		label: string;
		disabled?: boolean;
	}
</script>

<script lang="ts">
	import type { HTMLSelectAttributes } from 'svelte/elements';
	import type { Size } from '../types';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends Omit<HTMLSelectAttributes, 'class' | 'value' | 'size'> {
		value?: string;
		options: SelectOption[];
		size?: Size;
		class?: string;
	}

	let { value = $bindable(''), options, size = 'md', class: className = '', ...rest }: Props = $props();
</script>

<span class={['select', className]} data-size={size}>
	<select {...rest} bind:value class="control">
		{#each options as option (option.value)}
			<option value={option.value} disabled={option.disabled}>{option.label}</option>
		{/each}
	</select>
	<span class="chevron">
		<Icon name="chevron-down" />
	</span>
</span>

<style>
	.select {
		position: relative;
		display: inline-flex;
		align-items: center;
		min-inline-size: 0;
		max-inline-size: 100%;
	}

	.control {
		appearance: none;
		box-sizing: border-box;
		inline-size: 100%;
		min-inline-size: 0;
		block-size: var(--select-h, var(--control-h));
		padding-block: 0;
		padding-inline-start: var(--control-px);
		padding-inline-end: calc(var(--control-px) * 2 + var(--icon-size));
		font-family: var(--font-sans);
		font-size: var(--text-input);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		color: var(--select-fg, var(--text));
		background: var(--select-bg, var(--surface));
		border: var(--hairline) solid var(--select-border, var(--border-control));
		border-radius: var(--select-radius, var(--radius-md));
		text-overflow: ellipsis;
		cursor: pointer;
	}

	.select[data-size='sm'] .control {
		font-size: var(--text-sm-size);
	}

	.select[data-size='lg'] .control {
		block-size: var(--space-12);
	}

	@media (hover: hover) {
		.control:hover:not(:disabled) {
			border-color: var(--border-strong);
		}
	}

	.control:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.control[aria-invalid='true'] {
		border-color: var(--danger);
	}

	.control:disabled {
		background: var(--surface-sunken);
		color: var(--text-faint);
		cursor: not-allowed;
	}

	.chevron {
		position: absolute;
		inset-inline-end: var(--control-px);
		display: inline-flex;
		color: var(--text-muted);
		pointer-events: none;
	}
</style>
