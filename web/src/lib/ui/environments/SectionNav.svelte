<script lang="ts" module>
	import type { IconName } from '../types';

	export interface SectionNavItem {
		id: string;
		label: string;
		href: string;
		icon: IconName;
		badge?: string;
	}

	export interface SectionNavGroup {
		label: string;
		items: SectionNavItem[];
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends HTMLAttributes<HTMLElement> {
		groups: SectionNavGroup[];
		current: string;
		label: string;
	}

	let { groups, current, label, class: className, ...rest }: Props = $props();
</script>

<nav class={['section-nav', className]} aria-label={label} {...rest}>
	{#each groups as group (group.label)}
		<div class="group">
			<p class="caption">{group.label}</p>
			<ul class="items">
				{#each group.items as item (item.id)}
					<li>
						<a
							class="item"
							href={item.href}
							aria-current={item.id === current ? 'page' : undefined}
							data-state={item.id === current ? 'selected' : undefined}
						>
							<span class="item-icon"><Icon name={item.icon} /></span>
							<span class="item-label">{item.label}</span>
							{#if item.badge}<span class="badge">{item.badge}</span>{/if}
						</a>
					</li>
				{/each}
			</ul>
		</div>
	{/each}
</nav>

<style>
	.section-nav {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		min-inline-size: 0;
	}

	.group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}

	.caption {
		margin: 0;
		padding-inline: var(--space-2);
		padding-block-end: var(--space-1);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: 600;
		letter-spacing: var(--text-caption-tracking);
		color: var(--text-faint);
	}

	.items {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.item {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		font-weight: 500;
		color: var(--text-muted);
		text-decoration: none;
		outline: var(--focus-width) solid transparent;
		transition:
			background-color var(--dur-micro) var(--ease-out),
			color var(--dur-micro) var(--ease-out);
	}

	.item-icon {
		display: inline-flex;
		color: var(--text-faint);
	}

	@media (hover: hover) {
		.item:hover:not([data-state='selected']) {
			background: var(--surface-hover);
			color: var(--text);
		}
	}

	.item:active {
		background: var(--surface-pressed);
	}

	.item[data-state='selected'] {
		background: var(--accent-soft);
		color: var(--accent-text);
	}

	.item[data-state='selected'] .item-icon {
		color: var(--accent-text);
	}

	.item:focus-visible {
		box-shadow: var(--focus-ring);
	}

	@media (pointer: coarse) {
		.item {
			min-block-size: var(--touch-target);
		}
	}

	.item-label {
		flex: 1 1 auto;
		min-inline-size: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.badge {
		flex: none;
		padding-inline: var(--space-1-5);
		border-radius: var(--radius-full);
		background: var(--surface-sunken);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		color: var(--text-faint);
	}
</style>
