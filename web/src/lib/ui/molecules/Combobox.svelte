<script lang="ts" module>
	export const TOKENS = ['--combobox-bg', '--combobox-fg', '--combobox-border', '--combobox-radius'] as const;

	export interface ComboboxItem {
		value: string;
		label: string;
		disabled?: boolean;
	}
</script>

<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { Combobox as ComboboxPrimitive } from 'bits-ui';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Icon from '../quarks/Icon.svelte';

	const VISIBLE_CHIP_LIMIT = 2;
	const COMBOBOX_OFFSET_PX = 4;
	const COMBOBOX_COLLISION_PADDING_PX = 8;

	interface Props
		extends Pick<
			HTMLInputAttributes,
			'id' | 'class' | 'name' | 'placeholder' | 'disabled' | 'required' | 'readonly' | 'autocomplete' | 'aria-describedby' | 'aria-invalid'
		> {
		value?: string[];
		items?: ComboboxItem[];
		search?: (query: string) => Promise<ComboboxItem[]>;
		label: string;
		multiple?: boolean;
		creatable?: boolean;
	}

	let {
		value = $bindable([]),
		items = [],
		search,
		label,
		multiple = false,
		creatable = false,
		id,
		class: className,
		...rest
	}: Props = $props();

	const generatedId = $props.id();
	const inputId = $derived(id ?? generatedId);

	let open = $state(false);
	let query = $state('');
	let editing = $state(false);
	let picked = $state<ComboboxItem[]>([]);
	let remoteItems = $state<ComboboxItem[]>([]);
	let remoteStatus = $state<'idle' | 'loading' | 'ready' | 'error'>('idle');
	let requestCount = 0;

	const filterText = $derived(multiple || editing ? query : '');
	const normalizedFilter = $derived(filterText.trim().toLowerCase());

	const staticOptions = $derived(items.filter((item) => item.label.toLowerCase().includes(normalizedFilter)));
	const options = $derived(undefined === search ? staticOptions : remoteItems);
	const listStatus = $derived(undefined === search ? 'ready' : remoteStatus);

	const createText = $derived(filterText.trim());
	const offerCreate = $derived(
		creatable &&
			'' !== createText &&
			!options.some((option) => option.label.toLowerCase() === createText.toLowerCase())
	);

	$effect(() => {
		if (undefined === search) {
			return;
		}
		const text = filterText;
		requestCount += 1;
		const current = requestCount;
		remoteStatus = 'loading';
		search(text).then(
			(results) => {
				if (current === requestCount) {
					remoteItems = results;
					remoteStatus = 'ready';
				}
			},
			() => {
				if (current === requestCount) {
					remoteStatus = 'error';
				}
			}
		);
	});

	function resolveItem(itemValue: string): ComboboxItem {
		return (
			options.find((item) => item.value === itemValue) ??
			picked.find((item) => item.value === itemValue) ?? { value: itemValue, label: itemValue }
		);
	}

	const chips = $derived(value.map(resolveItem));
	const selectedLabel = $derived(0 < value.length ? resolveItem(value[0]).label : '');
	const shownText = $derived(!multiple && !editing ? selectedLabel : query);

	function commitSelection(next: string[]) {
		const selection = multiple ? next : next.slice(-1);
		picked = selection.map(resolveItem);
		value = selection;
		query = '';
		if (!multiple) {
			editing = false;
			open = false;
		}
	}

	function removeChip(itemValue: string) {
		commitSelection(value.filter((entry) => entry !== itemValue));
	}

	function readInput(event: Event) {
		const target = event.currentTarget;
		if (!(target instanceof HTMLInputElement)) {
			return;
		}
		query = target.value;
		editing = true;
		open = true;
	}

	function readKey(event: KeyboardEvent) {
		if ('Backspace' === event.key && multiple && '' === query && 0 < value.length) {
			commitSelection(value.slice(0, -1));
		}
	}
</script>

<div class={['combobox', className]}>
	<label class="combobox-label" for={inputId}>{label}</label>
	<ComboboxPrimitive.Root type="multiple" {value} bind:open inputValue={shownText} onValueChange={commitSelection}>
		<div class="combobox-control">
			{#if multiple}
				{#each chips.slice(0, VISIBLE_CHIP_LIMIT) as chip (chip.value)}
					<button type="button" class="combobox-chip" onclick={() => removeChip(chip.value)}>{chip.label}</button>
				{/each}
				{#if VISIBLE_CHIP_LIMIT < chips.length}
					<span class="combobox-more">+{formatNumber(chips.length - VISIBLE_CHIP_LIMIT)}</span>
				{/if}
			{/if}
			<ComboboxPrimitive.Input
				id={inputId}
				class="combobox-input"
				{...rest}
				oninput={readInput}
				onkeydown={readKey}
			/>
			<ComboboxPrimitive.Trigger class="combobox-trigger" aria-label={t('common.show_options')}>
				<Icon name="chevron-down" size="sm" />
			</ComboboxPrimitive.Trigger>
		</div>
		<ComboboxPrimitive.Portal>
			<ComboboxPrimitive.Content
				class="combobox-list"
				aria-label={label}
				sideOffset={COMBOBOX_OFFSET_PX}
				collisionPadding={COMBOBOX_COLLISION_PADDING_PX}
			>
				{#if 'loading' === listStatus}
					<div class="combobox-status" role="status">{t('common.loading_label')}</div>
				{:else if 'error' === listStatus}
					<div class="combobox-status" role="alert">{t('errors.network.title')}</div>
				{:else}
					{#each options as option (option.value)}
						<ComboboxPrimitive.Item value={option.value} label={option.label} disabled={option.disabled} class="combobox-option">
							{#snippet children({ selected })}
								<span class="combobox-check">
									{#if selected}
										<Icon name="check" />
									{/if}
								</span>
								<span class="combobox-option-label">{option.label}</span>
							{/snippet}
						</ComboboxPrimitive.Item>
					{/each}
					{#if offerCreate}
						<ComboboxPrimitive.Item value={createText} label={createText} class="combobox-option">
							{#snippet children()}
								<span class="combobox-check">
									<Icon name="plus" />
								</span>
								<span class="combobox-option-label">{createText}</span>
							{/snippet}
						</ComboboxPrimitive.Item>
					{/if}
					{#if 0 === options.length && !offerCreate}
						<div class="combobox-status" role="status">{t('common.no_matches')}</div>
					{/if}
				{/if}
			</ComboboxPrimitive.Content>
		</ComboboxPrimitive.Portal>
	</ComboboxPrimitive.Root>
</div>

<style>
	.combobox {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
		min-inline-size: 0;
		max-inline-size: 100%;
	}

	.combobox-label {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-body-weight);
		color: var(--text);
	}

	.combobox-control {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-1);
		box-sizing: border-box;
		min-inline-size: 0;
		max-inline-size: 100%;
		min-block-size: var(--control-h);
		padding-block: var(--space-1);
		padding-inline: var(--control-px);
		color: var(--combobox-fg, var(--text));
		background: var(--combobox-bg, var(--surface));
		border: var(--hairline) solid var(--combobox-border, var(--border-control));
		border-radius: var(--combobox-radius, var(--radius-md));
		font-size: var(--text-input);
		outline: var(--focus-width) solid transparent;
	}

	.combobox-control:focus-within {
		box-shadow: var(--focus-ring);
	}

	:global(.combobox-trigger) {
		display: inline-flex;
		align-items: center;
		padding: 0;
		border: 0;
		background: none;
		color: var(--text-muted);
		cursor: pointer;
	}

	:global(.combobox-input) {
		flex: 1 1 calc(var(--ch) * 8);
		min-inline-size: calc(var(--ch) * 8);
		block-size: var(--space-6);
		padding: 0;
		border: 0;
		background: transparent;
		color: inherit;
		font: inherit;
		outline: var(--focus-width) solid transparent;
	}

	.combobox-chip {
		display: inline-flex;
		align-items: center;
		max-inline-size: 100%;
		min-block-size: var(--space-6);
		padding-inline: var(--space-2);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--accent-text);
		background: var(--accent-soft);
		border: var(--hairline) solid var(--accent-border);
		border-radius: var(--radius-sm);
		overflow-wrap: anywhere;
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	.combobox-chip:focus-visible {
		box-shadow: var(--focus-ring);
	}

	@media (pointer: coarse) {
		.combobox-chip {
			min-block-size: var(--touch-target);
		}
	}

	.combobox-more {
		padding-inline: var(--space-1);
		font-size: var(--text-sm-size);
		color: var(--text-muted);
	}

	:global(.combobox-list) {
		z-index: var(--z-dropdown);
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: calc(var(--space-16) * 2.5);
		max-inline-size: min(calc(var(--space-16) * 5), calc(var(--viewport-w) - var(--space-4)));
		max-block-size: min(calc(var(--space-16) * 4), calc(var(--viewport-h) * 0.6));
		overflow-y: auto;
		padding: var(--space-1);
		color: var(--combobox-fg, var(--text));
		background: var(--combobox-bg, var(--surface-overlay));
		border: var(--hairline) solid var(--combobox-border, var(--border-subtle));
		border-radius: var(--combobox-radius, var(--radius-lg));
		box-shadow: var(--shadow-lg);
	}

	:global(.combobox-option) {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		box-sizing: border-box;
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: inherit;
		cursor: default;
		outline: var(--focus-width) solid transparent;
	}

	:global(.combobox-option[data-highlighted]) {
		background: var(--surface-hover);
	}

	:global(.combobox-option[data-disabled]) {
		color: var(--text-faint);
		cursor: not-allowed;
	}

	:global(.combobox-check) {
		display: inline-flex;
		flex: 0 0 var(--icon-size);
		inline-size: var(--icon-size);
	}

	:global(.combobox-option-label) {
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	:global(.combobox-status) {
		display: flex;
		align-items: center;
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		font-size: var(--text-sm-size);
		color: var(--text-muted);
	}
</style>
