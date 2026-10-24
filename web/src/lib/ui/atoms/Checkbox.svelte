<script lang="ts">
	import { Checkbox } from 'bits-ui';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends Omit<HTMLButtonAttributes, 'class' | 'children'> {
		checked: boolean;
		indeterminate?: boolean;
		label?: string;
		description?: string;
		disabledReason?: string;
		class?: string;
	}

	let {
		checked = $bindable(),
		indeterminate = $bindable(false),
		label,
		description,
		disabledReason,
		class: className,
		...rest
	}: Props = $props();

	const id = $props.id();
	const descriptionId = `${id}-description`;
	const reasonId = `${id}-reason`;

	const hasLabel = $derived(undefined !== label && '' !== label);
	const hasDescription = $derived(undefined !== description && '' !== description);
	const hasReason = $derived(undefined !== disabledReason && '' !== disabledReason);
	const describedBy = $derived(
		[hasDescription ? descriptionId : null, hasReason ? reasonId : null].filter((part) => null !== part).join(' ') ||
			undefined
	);
</script>

<div class={['checkbox-field', className]}>
	<Checkbox.Root
		{id}
		bind:checked
		bind:indeterminate
		class="checkbox"
		disabled={hasReason}
		aria-describedby={describedBy}
		{...(rest as Checkbox.RootProps)}
	>
		{#snippet children({ checked: marked, indeterminate: mixed })}
			{#if mixed}
				<Icon name="minus" size="xs" />
			{:else if marked}
				<Icon name="check" size="xs" />
			{/if}
		{/snippet}
	</Checkbox.Root>
	{#if hasLabel || hasDescription || hasReason}
		<span class="text">
			{#if hasLabel}<label class="label" for={id}>{label}</label>{/if}
			{#if hasDescription}<span class="description" id={descriptionId}>{description}</span>{/if}
			{#if hasReason}<span class="reason" id={reasonId}>{disabledReason}</span>{/if}
		</span>
	{/if}
</div>

<style>
	.checkbox-field {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		min-block-size: var(--target-min);
		min-inline-size: 0;
	}

	.checkbox-field:not(:has(.description, .reason)) {
		align-items: center;
	}

	.checkbox-field:has(.description, .reason) :global(.checkbox) {
		translate: 0 calc((var(--text-body-line) - var(--checkbox-box, var(--space-4))) / 2);
	}

	.checkbox-field :global(.checkbox) {
		display: inline-grid;
		place-items: center;
		flex: none;
		box-sizing: border-box;
		inline-size: var(--checkbox-box, var(--space-4));
		block-size: var(--checkbox-box, var(--space-4));
		padding: 0;
		border: var(--hairline) solid var(--checkbox-border, var(--border-control));
		border-radius: var(--checkbox-radius, var(--radius-xs));
		background: var(--checkbox-bg, var(--surface));
		color: var(--checkbox-mark, var(--text-on-accent));
		transition:
			background-color var(--dur-small) var(--ease-standard),
			border-color var(--dur-small) var(--ease-standard);
	}

	.checkbox-field :global(.checkbox[data-state='checked']),
	.checkbox-field :global(.checkbox[data-state='indeterminate']) {
		background: var(--checkbox-accent, var(--accent));
		border-color: var(--checkbox-accent, var(--accent));
	}

	.checkbox-field :global(.checkbox:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.checkbox-field :global(.checkbox:disabled) {
		cursor: not-allowed;
		border-color: var(--text-faint);
	}

	.checkbox-field :global(.checkbox:not(:disabled)) {
		cursor: pointer;
	}

	@media (hover: hover) {
		.checkbox-field :global(.checkbox:not(:disabled):hover) {
			border-color: var(--accent);
		}
	}

	.checkbox-field :global(.checkbox:not(:disabled):active) {
		background: var(--surface-pressed);
	}

	.checkbox-field :global(.checkbox[data-state='checked']:not(:disabled):active),
	.checkbox-field :global(.checkbox[data-state='indeterminate']:not(:disabled):active) {
		background: var(--accent-pressed);
	}

	@media (pointer: coarse) {
		.checkbox-field :global(.checkbox) {
			--checkbox-box: var(--space-6);
		}
	}

	:global(html[data-density='comfortable']) .checkbox-field {
		--checkbox-box: var(--space-5);
	}

	.text {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.label {
		color: var(--text);
		font-size: var(--text-body-size);
		cursor: pointer;
	}

	.checkbox-field:has(:disabled) .label {
		color: var(--text-faint);
		cursor: not-allowed;
	}

	.description,
	.reason {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
	}
</style>
