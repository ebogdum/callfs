<script lang="ts" module>
	export const TOKENS = ['--enum-field-gap'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import HelpText from '../atoms/HelpText.svelte';
	import Select from '../atoms/Select.svelte';
	import Combobox from './Combobox.svelte';
	import FormField from './FormField.svelte';
	import RadioGroup, { type ChoiceOption } from './RadioGroup.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		value?: string;
		options: ChoiceOption[];
		label: string;
		help?: string;
		class?: string;
	}

	let { value = $bindable(''), options, label, help, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const helpId = `${uid}-help`;
	const controlId = `${uid}-control`;
	const hasHelp = $derived(undefined !== help && '' !== help);
	const mode = $derived(5 >= options.length ? 'radio' : 15 >= options.length ? 'select' : 'combobox');
	const selectOptions = $derived(
		options.map((option) => ({
			value: option.value,
			label: option.label,
			disabled: Boolean(option.disabledReason)
		}))
	);

	function readSelection(): string[] {
		return '' === value ? [] : [value];
	}

	function writeSelection(next: string[]): void {
		const chosen = next.at(-1) ?? '';
		if (chosen !== value) {
			value = chosen;
		}
	}
</script>

<div class={['enum-field', className]} {...rest}>
	{#if 'radio' === mode}
		<RadioGroup bind:value {options} {label} aria-describedby={hasHelp ? helpId : undefined} />
		{#if hasHelp}<HelpText id={helpId}>{help}</HelpText>{/if}
	{:else if 'select' === mode}
		<FormField {label} for={controlId} {help}>
			{#snippet control({ id, describedBy })}
				<Select bind:value {id} aria-describedby={describedBy} options={selectOptions} />
			{/snippet}
		</FormField>
	{:else}
		<Combobox bind:value={readSelection, writeSelection} items={options} {label} />
		{#if hasHelp}<HelpText id={helpId}>{help}</HelpText>{/if}
	{/if}
</div>

<style>
	.enum-field {
		display: flex;
		flex-direction: column;
		gap: var(--enum-field-gap, var(--space-1));
		min-inline-size: 0;
	}
</style>
