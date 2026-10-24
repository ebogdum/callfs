<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { tick } from 'svelte';
	import { t } from '$lib/copy/t';
	import Button from '../atoms/Button.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Label from '../atoms/Label.svelte';
	import Tag from '../atoms/Tag.svelte';
	import Icon from '../quarks/Icon.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';
	import {
		completionAt,
		parse,
		removeRange,
		type CompletionOption,
		type ParseResult,
		type QueryError,
		type QueryErrorReason,
		type QueryGrammar,
		type QuerySuggestions,
		type QueryToken
	} from './query-grammar';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'onsubmit'> {
		value?: string;
		grammar: QueryGrammar;
		label: string;
		onsubmit?: (result: ParseResult) => void;
		suggestions?: QuerySuggestions;
		hint?: string;
	}

	let {
		value = $bindable(''),
		grammar,
		label,
		onsubmit,
		suggestions = {},
		hint = t('u095query.examples'),
		class: className,
		...rest
	}: Props = $props();

	const inputId = $props.id();
	const listId = `${inputId}-options`;
	const hintId = `${inputId}-hint`;
	const errorsId = `${inputId}-errors`;

	const reasonKeys: Record<QueryErrorReason, string> = {
		unknown_field: 'u095query.error_unknown_field',
		empty_value: 'u095query.error_empty_value',
		bad_value: 'u095query.error_bad_value',
		bad_size: 'u095query.error_bad_size',
		bad_date: 'u095query.error_bad_date',
		bad_status: 'u095query.error_bad_status',
		bad_range: 'u095query.error_bad_range',
		unterminated_quote: 'u095query.error_unterminated_quote'
	};

	let input: HTMLInputElement | undefined = $state();
	let caret = $state(0);
	let open = $state(false);
	let active = $state(0);

	const result: ParseResult = $derived(parse(value));
	const chips: QueryToken[] = $derived(result.tokens.filter((token) => 'text' !== token.field));
	const completion = $derived(completionAt(value, caret, grammar, suggestions));
	const options: CompletionOption[] = $derived(completion?.options ?? []);
	const listOpen = $derived(open && 0 < options.length);
	const activeOption = $derived(listOpen ? options[active] : undefined);
	const describedBy = $derived(
		0 < result.errors.length ? `${hintId} ${errorsId}` : hintId
	);

	function syncCaret(element: HTMLInputElement) {
		caret = element.selectionStart ?? value.length;
	}

	function handleInput(event: Event & { currentTarget: HTMLInputElement }) {
		syncCaret(event.currentTarget);
		open = true;
		active = 0;
	}

	function applyOption(option: CompletionOption) {
		if (null === completion) {
			return;
		}
		value = `${value.slice(0, completion.start)}${option.insert}${value.slice(completion.end)}`;
		caret = completion.start + option.insert.length;
		active = 0;
		open = true;
		void tick().then(() => input?.setSelectionRange(caret, caret));
	}

	function submit() {
		open = false;
		if (0 < result.errors.length) {
			return;
		}
		onsubmit?.(result);
	}

	function clear() {
		value = '';
		open = false;
		input?.focus();
	}

	function removeChip(token: QueryToken) {
		value = removeRange(value, token.start, token.end);
	}

	function errorReasonText(error: QueryError): string {
		return t(reasonKeys[error.reason]);
	}

	function handleKeydown(event: KeyboardEvent) {
		if ('ArrowDown' === event.key || 'ArrowUp' === event.key) {
			if (0 === options.length) {
				return;
			}
			event.preventDefault();
			open = true;
			const step = 'ArrowDown' === event.key ? 1 : options.length - 1;
			active = (active + step) % options.length;
			return;
		}
		if ('Enter' === event.key) {
			event.preventDefault();
			if (undefined !== activeOption) {
				applyOption(activeOption);
				return;
			}
			submit();
			return;
		}
		if ('Escape' === event.key && listOpen) {
			event.preventDefault();
			open = false;
		}
	}

	function keepFocus(event: MouseEvent) {
		event.preventDefault();
	}
</script>

<div
	class={['query-bar', className]}
	role="search"
	data-state={0 < result.errors.length ? 'error' : undefined}
	{...rest}
>
	<Stack gap="2">
		<Label for={inputId}>{label}</Label>
		<Cluster gap="2" align="start">
			<div class="field">
				<span class="glyph"><Icon name="search" /></span>
				<input
					bind:this={input}
					id={inputId}
					class="input"
					type="search"
					role="combobox"
					autocomplete="off"
					aria-autocomplete="list"
					aria-expanded={listOpen}
					aria-controls={listId}
					aria-activedescendant={undefined !== activeOption ? `${listId}-${active}` : undefined}
					aria-invalid={0 < result.errors.length ? 'true' : 'false'}
					aria-describedby={describedBy}
					bind:value
					oninput={handleInput}
					onclick={(event) => syncCaret(event.currentTarget)}
					onkeyup={(event) => syncCaret(event.currentTarget)}
					onkeydown={handleKeydown}
					onblur={() => (open = false)}
				/>
				{#if listOpen}
					<ul id={listId} class="options" role="listbox" aria-label={t('u095query.suggestions')}>
						{#each options as option, index (option.insert)}
							<li
								id={`${listId}-${index}`}
								class="option"
								role="option"
								aria-selected={index === active}
								data-state={index === active ? 'selected' : undefined}
								onmousedown={keepFocus}
								onclick={() => applyOption(option)}
							>
								<span class="option-label">{option.label}</span>
								{#if '' !== option.hint}
									<span class="option-hint">{option.hint}</span>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</div>
			<Button variant="primary" iconStart="search" onclick={submit}>{t('u095query.submit')}</Button>
			{#if '' !== value}
				<Button variant="ghost" onclick={clear}>{t('u095query.clear')}</Button>
			{/if}
		</Cluster>
		<HelpText id={hintId}>{hint}</HelpText>
		{#if 0 < chips.length}
			<Cluster gap="2" align="center">
				{#each chips as token (token.start)}
					<Tag label={token.raw} removable onremove={() => removeChip(token)} />
				{/each}
			</Cluster>
		{/if}
		{#if 0 < result.errors.length}
			<div id={errorsId} class="errors">
				<Stack gap="1">
					<span class="errors-title">{t('u095query.errors_title')}</span>
					<ul class="error-list">
						{#each result.errors as error (error.start)}
							<li class="error-item">
								<code class="raw">{error.raw}</code>
								<span>{errorReasonText(error)}</span>
							</li>
						{/each}
					</ul>
				</Stack>
			</div>
		{/if}
	</Stack>
</div>

<style>
	.query-bar {
		inline-size: 100%;
		min-inline-size: 0;
	}

	.field {
		position: relative;
		flex: 1 1 calc(var(--space-4) * 16);
		max-inline-size: var(--form-max);
		min-inline-size: 0;
		display: flex;
		align-items: center;
	}

	.glyph {
		position: absolute;
		inset-inline-start: var(--control-px);
		display: inline-flex;
		color: var(--text-muted);
		pointer-events: none;
	}

	.input {
		inline-size: 100%;
		block-size: var(--control-h);
		padding-inline: var(--space-8) var(--control-px);
		border: var(--hairline) solid var(--border-control);
		border-radius: var(--radius-md);
		background: var(--surface);
		box-shadow: var(--shadow-xs);
		color: var(--text);
		font: inherit;
	}

	.input::placeholder {
		color: var(--text-faint);
	}

	.input:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.input[aria-invalid='true'] {
		border-color: var(--danger);
	}

	.options {
		position: absolute;
		inset-block-start: calc(100% + var(--space-1));
		inset-inline: 0;
		z-index: var(--z-dropdown);
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		margin: 0;
		padding: var(--space-1);
		max-block-size: calc(var(--viewport-h) * 0.6);
		overflow-y: auto;
		list-style: none;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-overlay);
		box-shadow: var(--shadow-md);
	}

	.option {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		padding-block: var(--space-1);
		border-radius: var(--radius-sm);
		cursor: pointer;
	}

	@media (hover: hover) {
		.option:hover {
			background: var(--surface-hover);
		}
	}

	.option[data-state='selected'] {
		background: var(--surface-selected);
	}

	.option-label {
		font-family: var(--font-mono, inherit);
	}

	.option-hint {
		color: var(--text-muted);
	}

	.errors {
		padding: var(--space-2) var(--space-3);
		border: var(--hairline) solid var(--danger-border);
		border-radius: var(--radius-md);
		background: var(--danger-soft);
		color: var(--danger-text);
	}

	.errors-title {
		font-weight: 600;
	}

	.error-list {
		margin: 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.error-item {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-2);
	}

	.raw {
		font-family: var(--font-mono, inherit);
		text-decoration: underline wavy var(--danger);
	}
</style>
