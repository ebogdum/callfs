<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { joinPath, validatePath } from '$lib/format/path';
	import FieldError from '../atoms/FieldError.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';
	import Spinner from '../quarks/Spinner.svelte';
	import Stack from '../quarks/Stack.svelte';

	type SuggestionLookup = { ok: true; value: string[] } | { ok: false };

	interface Props extends Omit<HTMLInputAttributes, 'type' | 'value' | 'size' | 'prefix' | 'onsubmit'> {
		value?: string;
		label: string;
		base?: string;
		suggest?: (prefix: string) => Promise<string[]>;
		onsubmit?: (path: string) => void;
	}

	let {
		value = $bindable(''),
		label,
		base = '/',
		suggest,
		onsubmit,
		onkeydown,
		oninput,
		onfocus,
		onblur,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const fieldId = `${uid}-field`;
	const errorId = `${uid}-error`;
	const helpId = `${uid}-help`;
	const listId = `${uid}-options`;
	const statusId = `${uid}-status`;
	const optionId = (index: number): string => `${uid}-option-${index}`;

	let open = $state(false);
	let loading = $state(false);
	let suggestFailed = $state(false);
	let items = $state<string[]>([]);
	let active = $state(-1);
	let requestCount = 0;

	function resolvePath(text: string, folder: string): string {
		const trimmed = text.trim();
		return '/' === trimmed.charAt(0) ? trimmed : joinPath(folder, trimmed);
	}

	const resolved = $derived(resolvePath(value, base));
	const check = $derived(validatePath(resolved));
	const errorKey = $derived(check.ok ? null : check.error);
	const showSuggestions = $derived(open && undefined !== suggest);
	const listVisible = $derived(showSuggestions && !suggestFailed && 0 < items.length);

	async function loadSuggestions(text: string): Promise<void> {
		if (undefined === suggest) {
			return;
		}
		const current = ++requestCount;
		loading = true;
		const lookup: SuggestionLookup = await suggest(resolvePath(text, base)).then(
			(found): SuggestionLookup => ({ ok: true, value: found }),
			(): SuggestionLookup => ({ ok: false })
		);
		if (current !== requestCount) {
			return;
		}
		loading = false;
		suggestFailed = !lookup.ok;
		items = lookup.ok ? lookup.value : [];
		active = -1;
	}

	function choose(item: string): void {
		value = item;
		active = -1;
		open = true;
		void loadSuggestions(item);
	}

	function handleInput(event: Event & { currentTarget: HTMLInputElement }): void {
		oninput?.(event);
		open = true;
		void loadSuggestions(value);
	}

	function handleFocus(event: FocusEvent & { currentTarget: HTMLInputElement }): void {
		onfocus?.(event);
		open = true;
		void loadSuggestions(value);
	}

	function handleBlur(event: FocusEvent & { currentTarget: HTMLInputElement }): void {
		onblur?.(event);
		open = false;
		active = -1;
	}

	function keepFocus(event: MouseEvent): void {
		event.preventDefault();
	}

	function handleKeydown(event: KeyboardEvent & { currentTarget: EventTarget & HTMLInputElement }): void {
		onkeydown?.(event);
		if ('ArrowDown' === event.key && 0 < items.length) {
			event.preventDefault();
			open = true;
			active = active < items.length - 1 ? active + 1 : 0;
		} else if ('ArrowUp' === event.key && 0 < items.length) {
			event.preventDefault();
			active = 0 < active ? active - 1 : items.length - 1;
		} else if ('Escape' === event.key && open) {
			event.preventDefault();
			open = false;
			active = -1;
		} else if ('Enter' === event.key) {
			const picked = items[active];
			if (listVisible && undefined !== picked) {
				event.preventDefault();
				choose(picked);
			} else if (check.ok) {
				onsubmit?.(resolved);
			}
		}
	}
</script>

<Stack gap="1-5" class={className}>
	<Label for={fieldId}>{label}</Label>
	<Stack gap="1">
		<Input
			id={fieldId}
			role={undefined === suggest ? undefined : 'combobox'}
			aria-autocomplete={undefined === suggest ? undefined : 'list'}
			aria-expanded={undefined === suggest ? undefined : listVisible}
			aria-controls={listVisible ? listId : undefined}
			aria-activedescendant={listVisible && undefined !== items[active] ? optionId(active) : undefined}
			aria-describedby={null === errorKey ? helpId : `${helpId} ${errorId}`}
			autocomplete="off"
			mono
			invalid={null !== errorKey}
			bind:value
			{...rest}
			onkeydown={handleKeydown}
			oninput={handleInput}
			onfocus={handleFocus}
			onblur={handleBlur}
		>
			{#snippet suffix()}
				{#if loading}
					<Spinner size="sm" label={t('common.loading_label')} />
				{/if}
			{/snippet}
		</Input>
		{#if null !== errorKey}
			<FieldError id={errorId} message={t(errorKey)} />
		{/if}
		<HelpText id={helpId}>{t('inputs.path.help')}</HelpText>
		{#if showSuggestions}
			{#if suggestFailed}
				<HelpText id={statusId}>{t('inputs.path.suggest_error')}</HelpText>
			{:else if 0 < items.length}
				<ul id={listId} class="options" role="listbox" aria-label={label}>
					{#each items as item, index (item)}
						<li
							id={optionId(index)}
							class="option"
							role="option"
							aria-selected={active === index}
							data-active={active === index ? 'true' : undefined}
							onmousedown={keepFocus}
							onclick={() => choose(item)}
						>
							{item}
						</li>
					{/each}
				</ul>
			{:else if !loading}
				<HelpText id={statusId}>{t('inputs.path.no_match')}</HelpText>
			{/if}
		{/if}
	</Stack>
</Stack>

<style>
	.options {
		display: flex;
		flex-direction: column;
		gap: var(--path-input-option-gap, var(--space-0-5));
		padding: var(--space-1);
		list-style: none;
		background: var(--path-input-list-bg, var(--surface));
		border: var(--hairline) solid var(--path-input-list-border, var(--border-subtle));
		border-radius: var(--path-input-list-radius, var(--radius-lg));
		box-shadow: var(--shadow-md);
	}

	.option {
		padding-inline: var(--space-2);
		padding-block: var(--space-1);
		border-radius: var(--radius-xs);
		color: var(--path-input-option-fg, var(--text));
		font-family: var(--font-mono);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		overflow-wrap: anywhere;
		cursor: pointer;
	}

	.option[data-active='true'] {
		background: var(--path-input-option-active-bg, var(--surface-selected));
		color: var(--path-input-option-active-fg, var(--text));
	}

	@media (hover: hover) {
		.option:hover {
			background: var(--path-input-option-hover-bg, var(--surface-hover));
		}
	}
</style>
