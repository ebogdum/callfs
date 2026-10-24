<script lang="ts">
	import { Switch } from 'bits-ui';
	import type { ComponentProps } from 'svelte';
	import Spinner from '../quarks/Spinner.svelte';

	interface Props
		extends Omit<
			ComponentProps<typeof Switch.Root>,
			'checked' | 'onCheckedChange' | 'class' | 'children' | 'child' | 'onchange'
		> {
		checked: boolean;
		label: string;
		description?: string;
		saving?: boolean;
		onchange?: (checked: boolean) => void;
		class?: string;
	}

	let { checked = $bindable(), label, description, saving = false, onchange, class: className, ...rest }: Props = $props();

	const id = $props.id();
	const descriptionId = `${id}-description`;

	const hasDescription = $derived(undefined !== description && '' !== description);
</script>

<div class={['switch-field', className]}>
	<span class="text">
		<label class="label" for={id}>{label}</label>
		{#if hasDescription}<span class="description" id={descriptionId}>{description}</span>{/if}
	</span>
	<Switch.Root
		{id}
		bind:checked
		class="switch"
		aria-busy={saving}
		aria-describedby={hasDescription ? descriptionId : undefined}
		onCheckedChange={(next: boolean) => onchange?.(next)}
		{...rest}
	>
		<Switch.Thumb class="switch-thumb" />
	</Switch.Root>
	{#if saving}<Spinner size="sm" />{/if}
</div>

<style>
	.switch-field {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-block-size: var(--target-min);
		min-inline-size: 0;
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

	.description {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
	}

	.switch-field :global(.switch) {
		--switch-track-w: calc(var(--space-8) + var(--space-1));
		display: inline-flex;
		flex: none;
		align-items: center;
		box-sizing: border-box;
		inline-size: var(--switch-track-w);
		block-size: var(--space-5);
		padding: var(--space-0-5);
		border: 0;
		border-radius: var(--radius-full);
		background: var(--switch-track, var(--border-control));
		cursor: pointer;
		transition: background-color var(--dur-small) var(--ease-standard);
	}

	.switch-field :global(.switch[data-state='checked']) {
		background: var(--switch-track-on, var(--accent));
	}

	.switch-field :global(.switch:disabled) {
		cursor: not-allowed;
		background: var(--text-faint);
	}

	.switch-field :global(.switch:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.switch-field :global(.switch:not(:disabled):hover) {
			background: var(--accent-hover);
		}

		.switch-field :global(.switch[data-state='unchecked']:not(:disabled):hover) {
			background: var(--text-faint);
		}
	}

	.switch-field :global(.switch-thumb) {
		inline-size: var(--space-4);
		block-size: var(--space-4);
		border-radius: var(--radius-full);
		background: var(--switch-thumb, var(--text-on-accent));
		box-shadow: var(--shadow-sm);
		transition: transform var(--dur-small) var(--ease-standard);
	}

	.switch-field :global(.switch[data-state='checked'] > .switch-thumb) {
		transform: translateX(calc(var(--switch-track-w) - var(--space-4) - var(--space-1)));
	}

	.switch-field:has(:disabled) .label {
		color: var(--text-faint);
		cursor: not-allowed;
	}
</style>
