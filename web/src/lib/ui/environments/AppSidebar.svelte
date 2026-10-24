<script lang="ts" module>
	import type { IconName } from '../types';

	export interface SidebarItem {
		id: string;
		label: string;
		href: string;
		icon: IconName;
	}

	export interface SidebarGroup {
		id: string;
		label: string;
		items: SidebarItem[];
	}
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import IconButton from '../atoms/IconButton.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';
	import Icon from '../quarks/Icon.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		groups: SidebarGroup[];
		current: string;
		collapsed?: boolean;
		home?: string;
		footer: Snippet<[boolean]>;
		class?: string;
	}

	let {
		groups,
		current,
		collapsed = $bindable(false),
		home = '/',
		footer,
		class: className,
		...rest
	}: Props = $props();

	const navigationLabel = t('shell.navigation.main');
	const toggleLabel = $derived(collapsed ? t('shell.sidebar.expand') : t('shell.sidebar.collapse'));
</script>

{#snippet entry(item: SidebarItem)}
	<a
		class="entry"
		href={item.href}
		aria-current={current === item.id ? 'page' : undefined}
		data-state={current === item.id ? 'selected' : undefined}
	>
		<span class="entry-icon"><Icon name={item.icon} size="md" /></span>
		{#if collapsed}
			<VisuallyHidden>{item.label}</VisuallyHidden>
		{:else}
			<span class="entry-label">{item.label}</span>
		{/if}
	</a>
{/snippet}

<nav
	class={['app-sidebar', className]}
	data-state={collapsed ? 'collapsed' : 'expanded'}
	aria-label={navigationLabel}
	{...rest}
>
	<div class="head">
		<a class="brand" href={home} aria-label={t('shell.brand.home')}>
			<span class="mark" aria-hidden="true"><Icon name="hard-drive" size="md" /></span>
			{#if false === collapsed}
				<span class="wordmark">{t('shell.brand.name')}</span>
			{/if}
		</a>
		<IconButton
			icon="sidebar"
			size="sm"
			label={toggleLabel}
			shortcut={['[']}
			aria-expanded={!collapsed}
			onclick={() => (collapsed = !collapsed)}
		/>
	</div>
	<div class="body">
		{#each groups as group (group.id)}
			<section class="group" aria-labelledby={`sidebar-group-${group.id}`}>
				<h2 class={['group-label', collapsed && 'hidden-label']} id={`sidebar-group-${group.id}`}>{group.label}</h2>
				<ul class="list">
					{#each group.items as item (item.id)}
						<li>
							{#if collapsed}
								<Tooltip content={item.label} side="right">{@render entry(item)}</Tooltip>
							{:else}
								{@render entry(item)}
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	</div>
	<div class="footer">{@render footer(collapsed)}</div>
</nav>

<style>
	.app-sidebar {
		position: sticky;
		inset-block-start: 0;
		display: flex;
		flex-direction: column;
		inline-size: var(--sidebar-w);
		block-size: 100dvh;
		color: var(--text);
		z-index: var(--z-sidebar);
	}

	.app-sidebar[data-state='collapsed'] {
		inline-size: var(--rail-w);
	}

	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
		block-size: var(--topbar-h);
		padding-inline: var(--space-4) var(--space-3);
		flex: none;
	}

	.app-sidebar[data-state='collapsed'] .head {
		flex-direction: column;
		justify-content: center;
		block-size: auto;
		padding-block: var(--space-3);
		padding-inline: var(--space-2);
		gap: var(--space-2);
	}

	.brand {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
		border-radius: var(--radius-md);
		color: var(--text);
		text-decoration: none;
	}

	.mark {
		display: grid;
		place-items: center;
		flex: none;
		inline-size: var(--space-6);
		block-size: var(--space-6);
		border-radius: var(--radius-sm);
		background: var(--accent);
		color: var(--text-on-accent);
		box-shadow: var(--highlight-top);
	}

	.wordmark {
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-weight);
		letter-spacing: var(--text-title-tracking);
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		flex: 1 1 auto;
		min-block-size: 0;
		overflow-y: auto;
		padding-block: var(--space-2) var(--space-4);
		padding-inline: var(--space-3);
	}

	.app-sidebar[data-state='collapsed'] .body {
		padding-inline: var(--space-2);
	}

	.group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.group-label {
		margin: 0;
		padding-inline: var(--space-2);
		padding-block-end: var(--space-1);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: 600;
		letter-spacing: var(--text-caption-tracking);
		color: var(--text-faint);
	}

	.hidden-label {
		position: absolute;
		inline-size: var(--space-px);
		block-size: var(--space-px);
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.list {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.entry {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		color: var(--text-muted);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: 500;
		text-decoration: none;
		transition:
			background-color var(--dur-micro) var(--ease-out),
			color var(--dur-micro) var(--ease-out);
	}

	.entry-icon {
		display: inline-flex;
		color: var(--text-faint);
	}

	.app-sidebar[data-state='collapsed'] .entry {
		justify-content: center;
		inline-size: var(--space-10);
		margin-inline: auto;
		padding-inline: 0;
	}

	.entry[data-state='selected'] {
		background: var(--surface-raised);
		color: var(--text);
		box-shadow:
			0 0 0 var(--hairline) var(--border-subtle),
			var(--shadow-xs);
	}

	.entry[data-state='selected'] .entry-icon {
		color: var(--accent);
	}

	.entry:active {
		background: var(--surface-pressed);
	}

	.entry:focus-visible,
	.brand:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	@media (hover: hover) {
		.entry:hover:not([data-state='selected']) {
			background: var(--surface-hover);
			color: var(--text);
		}
	}

	@media (pointer: coarse) {
		.entry {
			min-block-size: var(--touch-target);
		}
	}

	.entry-label {
		min-inline-size: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.footer {
		flex: none;
		padding: var(--space-3);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.app-sidebar[data-state='collapsed'] .footer {
		display: flex;
		justify-content: center;
		padding-inline: var(--space-2);
	}

	@media (max-width: 639px) {
		.app-sidebar {
			display: none;
		}
	}
</style>
