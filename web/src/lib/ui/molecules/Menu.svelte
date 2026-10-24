<script lang="ts" module>
	export const TOKENS = ['--menu-bg', '--menu-fg', '--menu-border', '--menu-radius'] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import { DropdownMenu } from 'bits-ui';
	import type { MenuItem } from '../types';
	import Icon from '../quarks/Icon.svelte';
	import Kbd from '../atoms/Kbd.svelte';

	const MENU_SIDE_OFFSET_PX = 4;
	const MENU_COLLISION_PADDING_PX = 8;

	interface Props {
		trigger: Snippet<[Record<string, unknown>]>;
		items: MenuItem[];
		label: string;
		class?: string;
	}

	let { trigger, items, label, class: className }: Props = $props();

	function sectionMenuItems(source: MenuItem[]): MenuItem[][] {
		const groups = new Map<string, MenuItem[]>();
		const danger: MenuItem[] = [];
		for (const item of source) {
			if ('danger' === item.tone) {
				danger.push(item);
				continue;
			}
			const key = item.group ?? '';
			const bucket = groups.get(key) ?? [];
			bucket.push(item);
			groups.set(key, bucket);
		}
		const sections = [...groups.values()];
		if (0 < danger.length) {
			sections.push(danger);
		}
		return sections;
	}

	const sections = $derived(sectionMenuItems(items));

	const isDisabled = (item: MenuItem): boolean => Boolean(item.disabledReason);
</script>

{#snippet itemBody(item: MenuItem)}
	{#if item.checked}
		<Icon name="check" />
	{:else if item.icon}
		<Icon name={item.icon} />
	{/if}
	<span class="menu-item-label">
		{item.label}
		{#if isDisabled(item)}
			<span class="menu-item-reason">{item.disabledReason}</span>
		{/if}
	</span>
	{#if item.shortcut}
		<Kbd keys={item.shortcut} />
	{/if}
	{#if item.children && 0 < item.children.length}
		<Icon name="chevron-right" />
	{/if}
{/snippet}

{#snippet entry(item: MenuItem)}
	{#if item.children && 0 < item.children.length}
		<DropdownMenu.Sub>
			<DropdownMenu.SubTrigger class="menu-item" data-tone={item.tone ?? 'default'} textValue={item.label} disabled={isDisabled(item)}>
				{@render itemBody(item)}
			</DropdownMenu.SubTrigger>
			<DropdownMenu.Portal>
				<DropdownMenu.SubContent class="menu" collisionPadding={MENU_COLLISION_PADDING_PX}>
					{#each item.children as child (child.id)}
						{@render entry(child)}
					{/each}
				</DropdownMenu.SubContent>
			</DropdownMenu.Portal>
		</DropdownMenu.Sub>
	{:else if undefined !== item.checked}
		<DropdownMenu.CheckboxItem
			class="menu-item"
			data-tone={item.tone ?? 'default'}
			checked={item.checked}
			textValue={item.label}
			disabled={isDisabled(item)}
			onCheckedChange={() => item.onselect()}
		>
			{@render itemBody(item)}
		</DropdownMenu.CheckboxItem>
	{:else}
		<DropdownMenu.Item
			class="menu-item"
			data-tone={item.tone ?? 'default'}
			textValue={item.label}
			disabled={isDisabled(item)}
			onSelect={() => item.onselect()}
		>
			{@render itemBody(item)}
		</DropdownMenu.Item>
	{/if}
{/snippet}

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			{@render trigger(props)}
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Portal>
		<DropdownMenu.Content
			class={className ? `menu ${className}` : 'menu'}
			aria-label={label}
			sideOffset={MENU_SIDE_OFFSET_PX}
			collisionPadding={MENU_COLLISION_PADDING_PX}
		>
			{#each sections as section, sectionIndex (sectionIndex)}
				{#if 0 < sectionIndex}
					<DropdownMenu.Separator class="menu-separator" />
				{/if}
				{#each section as item (item.id)}
					{@render entry(item)}
				{/each}
			{/each}
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>

<style>
	:global(.menu) {
		z-index: var(--z-dropdown);
		box-sizing: border-box;
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: calc(var(--space-16) * 2.5);
		max-inline-size: min(calc(var(--space-16) * 5), calc(var(--viewport-w) - var(--space-4)));
		max-block-size: min(calc(var(--space-16) * 6), calc(var(--viewport-h) - var(--space-8)));
		overflow-y: auto;
		padding: var(--space-1);
		color: var(--menu-fg, var(--text));
		background: var(--menu-bg, var(--surface-overlay));
		border: var(--hairline) solid var(--menu-border, var(--border-subtle));
		border-radius: var(--menu-radius, var(--radius-lg));
		box-shadow: var(--shadow-lg);
	}

	:global(.menu-item) {
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
		user-select: none;
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

	:global(.menu-item[data-tone='danger'][data-highlighted]) {
		background: var(--danger-soft);
	}

	:global(.menu-item[data-disabled]) {
		color: var(--text-faint);
		cursor: not-allowed;
	}

	:global(.menu-item-label) {
		display: flex;
		flex: 1 1 auto;
		flex-direction: column;
		min-inline-size: 0;
		overflow-wrap: anywhere;
	}

	:global(.menu-item-reason) {
		font-size: var(--text-caption-size);
		color: var(--text-muted);
	}

	:global(.menu-separator) {
		block-size: var(--hairline);
		background: var(--border-subtle);
	}
</style>
