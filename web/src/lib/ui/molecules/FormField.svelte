<script lang="ts" module>
	export type FormFieldLayout = 'stacked' | 'inline';

	export const TOKENS = ['--form-field-gap', '--form-field-inline-gap'] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Label from '../atoms/Label.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import { t } from '$lib/copy/t';

	interface ControlContext {
		id: string;
		describedBy: string | undefined;
		invalid: boolean;
	}

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		label: string;
		for: string;
		help?: string;
		example?: string;
		error?: string;
		optional?: boolean;
		layout?: FormFieldLayout;
		control: Snippet<[ControlContext]>;
		class?: string;
	}

	let {
		label,
		for: htmlFor,
		help,
		example,
		error,
		optional = false,
		layout = 'stacked',
		control,
		class: className,
		...rest
	}: Props = $props();

	const helpId = $derived(`${htmlFor}-help`);
	const exampleId = $derived(`${htmlFor}-example`);
	const errorId = $derived(`${htmlFor}-error`);
	const hasHelp = $derived(undefined !== help && '' !== help);
	const hasExample = $derived(undefined !== example && '' !== example);
	const invalid = $derived(undefined !== error && '' !== error);
	const describedBy = $derived(
		[hasHelp ? helpId : null, hasExample ? exampleId : null, invalid ? errorId : null]
			.filter((part) => null !== part)
			.join(' ') || undefined
	);
</script>

<div class={['form-field', className]} data-layout={layout} {...rest}>
	<div class="label-cell">
		<Label for={htmlFor} {optional}>{label}</Label>
	</div>
	<div class="control-cell">
		{@render control({ id: htmlFor, describedBy, invalid })}
		{#if hasHelp || hasExample}
			<div class="hints">
				{#if hasHelp}<HelpText id={helpId}>{help}</HelpText>{/if}
				{#if hasExample}<HelpText id={exampleId}>{t('form.example', { value: example ?? '' })}</HelpText>{/if}
			</div>
		{/if}
		<div class="error-region" aria-live="polite">
			{#if invalid}<FieldError id={errorId} message={error ?? ''} />{/if}
		</div>
	</div>
</div>

<style>
	.form-field {
		display: grid;
		gap: var(--form-field-gap, var(--space-1-5));
		min-inline-size: 0;
	}

	.label-cell,
	.control-cell {
		min-inline-size: 0;
	}

	.control-cell,
	.hints {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	@media (min-width: 768px) {
		.form-field[data-layout='inline'] {
			grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
			align-items: start;
			gap: var(--form-field-inline-gap, var(--space-3));
		}
	}
</style>
