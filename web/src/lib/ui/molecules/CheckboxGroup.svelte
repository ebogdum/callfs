<script lang="ts" module>
	export interface CheckboxOption {
		value: string;
		label: string;
		description?: string;
		disabledReason?: string;
	}

	export const TOKENS = ['--checkbox-group-gap', '--checkbox-group-label-fg'] as const;
</script>

<script lang="ts">
	import type { HTMLFieldsetAttributes } from 'svelte/elements';
	import Checkbox from '../atoms/Checkbox.svelte';

	interface Props extends Omit<HTMLFieldsetAttributes, 'class'> {
		values?: string[];
		options: CheckboxOption[];
		label: string;
		class?: string;
	}

	let { values = $bindable([]), options, label, class: className, ...rest }: Props = $props();

	function setOption(optionValue: string, checked: boolean): void {
		const others = values.filter((entry) => optionValue !== entry);
		values = checked ? [...others, optionValue] : others;
	}
</script>

<fieldset class={['checkbox-group', className]} {...rest}>
	<legend class="group-label">{label}</legend>
	<div class="options">
		{#each options as option (option.value)}
			<Checkbox
				bind:checked={() => values.includes(option.value), (checked) => setOption(option.value, checked)}
				label={option.label}
				description={option.description}
				disabledReason={option.disabledReason}
			/>
		{/each}
	</div>
</fieldset>

<style>
	.checkbox-group {
		display: flex;
		flex-direction: column;
		gap: var(--checkbox-group-gap, var(--space-1-5));
		min-inline-size: 0;
		border: 0;
		padding: 0;
		margin: 0;
	}

	.group-label {
		padding: 0;
		color: var(--checkbox-group-label-fg, var(--text));
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: var(--text-body-weight);
		letter-spacing: var(--text-body-tracking);
		overflow-wrap: anywhere;
	}

	.options {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: 0;
	}
</style>
