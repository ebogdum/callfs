<script lang="ts" module>
	export interface ChoiceOption {
		value: string;
		label: string;
		description?: string;
		recommended?: boolean;
		disabledReason?: string;
	}
</script>

<script lang="ts">
	import { RadioGroup as RadioGroupPrimitive } from 'bits-ui';
	import Radio from '../atoms/Radio.svelte';

	interface Props extends Omit<RadioGroupPrimitive.RootProps, 'class' | 'children' | 'child' | 'value' | 'orientation'> {
		value?: string;
		options: ChoiceOption[];
		label: string;
		orientation?: 'vertical' | 'horizontal';
		class?: string;
	}

	let { value = $bindable(''), options, label, orientation = 'vertical', class: className, ...rest }: Props = $props();

	const labelId = $props.id();

	function describe(option: ChoiceOption): string | undefined {
		const parts = [option.description, option.disabledReason].filter((part) => undefined !== part && '' !== part);
		return parts.length > 0 ? parts.join(' ') : undefined;
	}
</script>

<div class={['radio-group-field', className]}>
	<span class="group-label" id={labelId}>{label}</span>
	<RadioGroupPrimitive.Root
		bind:value
		class="radio-group"
		{orientation}
		aria-labelledby={labelId}
		{...rest}
	>
		{#each options as option (option.value)}
			<Radio
				value={option.value}
				label={option.label}
				description={describe(option)}
				recommended={option.recommended}
				disabled={Boolean(option.disabledReason)}
			/>
		{/each}
	</RadioGroupPrimitive.Root>
</div>

<style>
	.radio-group-field {
		display: flex;
		flex-direction: column;
		gap: var(--radio-group-gap, var(--space-1-5));
		min-inline-size: 0;
	}

	.group-label {
		color: var(--radio-group-label-fg, var(--text));
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
		overflow-wrap: anywhere;
	}

	.radio-group-field :global(.radio-group) {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.radio-group-field :global(.radio-group[data-orientation='horizontal']) {
		flex-direction: row;
		flex-wrap: wrap;
		gap: var(--space-4);
	}
</style>
