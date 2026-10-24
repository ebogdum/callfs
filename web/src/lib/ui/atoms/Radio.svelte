<script lang="ts" module>
	export const TOKENS = ['--radio-size', '--radio-border', '--radio-bg', '--radio-accent'] as const;
</script>

<script lang="ts">
	import { RadioGroup } from 'bits-ui';
	import { t } from '$lib/copy/t';

	interface Props extends Omit<RadioGroup.ItemProps, 'class' | 'children' | 'child' | 'value'> {
		value: string;
		label: string;
		description?: string;
		recommended?: boolean;
		class?: string;
	}

	let { value, label, description, recommended = false, class: className, ...rest }: Props = $props();

	const id = $props.id();
	const descriptionId = `${id}-description`;

	const hasDescription = $derived(undefined !== description && '' !== description);
</script>

<div class={['radio-field', className]}>
	<RadioGroup.Item
		{id}
		{value}
		class="radio"
		aria-describedby={hasDescription ? descriptionId : undefined}
		{...rest}
	/>
	<span class="text">
		<span class="line">
			<label class="label" for={id}>{label}</label>
			{#if recommended}<span class="recommended">{t('common.recommended')}</span>{/if}
		</span>
		{#if hasDescription}<span class="description" id={descriptionId}>{description}</span>{/if}
	</span>
</div>

<style>
	.radio-field {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		min-block-size: var(--target-min);
		min-inline-size: 0;
	}

	.radio-field :global(.radio) {
		display: inline-grid;
		place-items: center;
		flex: none;
		box-sizing: border-box;
		inline-size: var(--radio-size, var(--space-4));
		block-size: var(--radio-size, var(--space-4));
		padding: 0;
		border: var(--hairline) solid var(--radio-border, var(--border-control));
		border-radius: var(--radius-full);
		background: var(--radio-bg, var(--surface));
		transition: border-color var(--dur-small) var(--ease-standard);
	}

	.radio-field :global(.radio)::after {
		content: '';
		inline-size: var(--space-2);
		block-size: var(--space-2);
		border-radius: var(--radius-full);
		background: var(--radio-accent, var(--accent));
		transform: scale(0);
		transition: transform var(--dur-small) var(--ease-standard);
	}

	.radio-field :global(.radio[data-state='checked'])::after {
		transform: scale(1);
	}

	.radio-field :global(.radio[data-state='checked']) {
		border-color: var(--radio-accent, var(--accent));
	}

	.radio-field :global(.radio:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.radio-field :global(.radio:disabled) {
		cursor: not-allowed;
		border-color: var(--text-faint);
	}

	.radio-field :global(.radio:not(:disabled)) {
		cursor: pointer;
	}

	@media (hover: hover) {
		.radio-field :global(.radio:not(:disabled):hover) {
			border-color: var(--accent);
		}
	}

	.radio-field :global(.radio:not(:disabled):active) {
		background: var(--surface-pressed);
	}

	@media (pointer: coarse) {
		.radio-field :global(.radio) {
			--radio-size: var(--space-6);
		}
	}

	:global(html[data-density='comfortable']) .radio-field {
		--radio-size: var(--space-5);
	}

	.text {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.line {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}

	.label {
		color: var(--text);
		font-size: var(--text-body-size);
		cursor: pointer;
	}

	.radio-field:has(:disabled) .label {
		color: var(--text-faint);
		cursor: not-allowed;
	}

	.recommended {
		padding-block: var(--space-0-5);
		padding-inline: var(--space-2);
		border: var(--hairline) solid var(--accent-border);
		border-radius: var(--radius-full);
		background: var(--accent-soft);
		color: var(--accent-text);
		font-size: var(--text-sm-size);
	}

	.description {
		color: var(--text-muted);
		font-size: var(--text-sm-size);
	}
</style>
