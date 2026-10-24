<script lang="ts">
	import { DropdownMenu } from 'bits-ui';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import IconButton from '../atoms/IconButton.svelte';
	import Kbd from '../atoms/Kbd.svelte';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName, MenuItem } from '../types';

	type MenuRow = { item: MenuItem; separated: boolean };
	type IconMenuItem = MenuItem & { icon: IconName };

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		inline?: MenuItem[];
		menu: MenuItem[];
		label: string;
		class?: string;
	}

	let { inline = [], menu, label, class: className, ...rest }: Props = $props();

	let menuOpen = $state(false);

	const inlineItems = $derived(
		inline.filter((entry): entry is IconMenuItem => undefined !== entry.icon)
	);

	const orderedRows = (items: MenuItem[]): MenuRow[] => {
		const rows: MenuRow[] = [];
		let previous: MenuItem | undefined;
		const ordered = [
			...items.filter((entry) => 'danger' !== entry.tone),
			...items.filter((entry) => 'danger' === entry.tone)
		];
		for (const item of ordered) {
			rows.push({
				item,
				separated:
					undefined !== previous &&
					(previous.group !== item.group || ('danger' === item.tone && 'danger' !== previous.tone))
			});
			previous = item;
		}
		return rows;
	};

	const menuRows = $derived(orderedRows(menu));

	const openOnShiftF10 = (event: KeyboardEvent) => {
		if ('F10' === event.key && event.shiftKey) {
			event.preventDefault();
			menuOpen = true;
		}
	};
</script>

{#snippet entryBody(entry: MenuItem)}
	{#if entry.icon}<Icon name={entry.icon} size="sm" />{/if}
	<span class="entry-label">{entry.label}</span>
	{#if entry.shortcut}<Kbd keys={entry.shortcut} />{/if}
	{#if entry.disabledReason}<span class="reason">{entry.disabledReason}</span>{/if}
{/snippet}

{#snippet entries(rows: MenuRow[])}
	{#each rows as row (row.item.id)}
		{#if row.separated}<DropdownMenu.Separator class="separator" />{/if}
		{#if undefined !== row.item.children}
			<DropdownMenu.Sub>
				<DropdownMenu.SubTrigger class="menu-item" textValue={row.item.label}>
					{@render entryBody(row.item)}
				</DropdownMenu.SubTrigger>
				<DropdownMenu.Portal>
					<DropdownMenu.SubContent class="menu">
						{@render entries(orderedRows(row.item.children))}
					</DropdownMenu.SubContent>
				</DropdownMenu.Portal>
			</DropdownMenu.Sub>
		{:else if undefined === row.item.checked}
			<DropdownMenu.Item
				class="menu-item"
				textValue={row.item.label}
				data-tone={row.item.tone ?? 'default'}
				disabled={'' !== (row.item.disabledReason ?? '')}
				onSelect={() => row.item.onselect()}
			>
				{@render entryBody(row.item)}
			</DropdownMenu.Item>
		{:else}
			<DropdownMenu.CheckboxItem
				class="menu-item"
				textValue={row.item.label}
				data-tone={row.item.tone ?? 'default'}
				checked={row.item.checked}
				disabled={'' !== (row.item.disabledReason ?? '')}
				onCheckedChange={() => row.item.onselect()}
			>
				{@render entryBody(row.item)}
			</DropdownMenu.CheckboxItem>
		{/if}
	{/each}
{/snippet}

<div class={['actions-cell', className]} {...rest}>
	{#if 0 < inlineItems.length}
		<div class="inline-actions">
			{#each inlineItems as entry (entry.id)}
				<IconButton
					icon={entry.icon}
					label={entry.label}
					size="sm"
					shortcut={entry.shortcut}
					disabledReason={entry.disabledReason}
					onkeydowncapture={openOnShiftF10}
					onclick={() => entry.onselect()}
				/>
			{/each}
		</div>
	{/if}
	<DropdownMenu.Root bind:open={menuOpen}>
		<DropdownMenu.Trigger>
			{#snippet child({ props })}
				<IconButton
					{...props}
					icon="more-vertical"
					label={`${t('common.more')} ${label}`}
					size="sm"
					onkeydowncapture={openOnShiftF10}
				/>
			{/snippet}
		</DropdownMenu.Trigger>
		<DropdownMenu.Portal>
			<DropdownMenu.Content class="menu" sideOffset={4} align="end" aria-label={label}>
				{@render entries(menuRows)}
			</DropdownMenu.Content>
		</DropdownMenu.Portal>
	</DropdownMenu.Root>
</div>

<style>
	.actions-cell {
		display: inline-flex;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
	}

	.inline-actions {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
	}

	@media (hover: hover) {
		.inline-actions {
			opacity: 0;
			transition: opacity var(--dur-micro) var(--ease-out);
		}

		.actions-cell:hover .inline-actions,
		.actions-cell:focus-within .inline-actions {
			opacity: 1;
		}
	}

	:global(.menu) {
		z-index: var(--z-dropdown);
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: calc(var(--control-h) * 5);
		max-inline-size: calc(var(--control-h) * 10);
		padding: var(--space-1);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-overlay);
		box-shadow: var(--shadow-md);
	}

	:global(.menu-item) {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		border-radius: var(--radius-sm);
		color: var(--text);
		font-size: var(--text-sm-size);
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	:global(.menu-item[data-highlighted]) {
		background: var(--surface-hover);
	}

	:global(.menu-item:focus-visible) {
		box-shadow: var(--focus-ring);
	}

	:global(.menu-item[data-tone='danger']) {
		color: var(--danger-text);
	}

	:global(.menu-item[data-disabled]) {
		color: var(--text-faint);
		cursor: not-allowed;
	}

	:global(.separator) {
		block-size: var(--hairline);
		background: var(--border-subtle);
	}

	.entry-label {
		flex: 1;
		min-inline-size: 0;
	}

	.reason {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
	}

	@media (pointer: coarse) {
		:global(.menu-item) {
			min-block-size: var(--touch-target);
		}
	}
</style>
