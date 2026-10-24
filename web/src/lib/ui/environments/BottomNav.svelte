<script lang="ts" module>
	import type { IconName } from '../types';

	export interface BottomNavItem {
		id: string;
		label: string;
		href: string;
		icon: IconName;
	}

	export interface BottomNavGroup {
		id: string;
		label: string;
		items: BottomNavItem[];
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Popover from '../molecules/Popover.svelte';
	import Text from '../atoms/Text.svelte';
	import Icon from '../quarks/Icon.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		items: BottomNavItem[];
		current: string;
		more: BottomNavGroup[];
		class?: string;
	}

	let { items, current, more, class: className, ...rest }: Props = $props();

	const navigationLabel = t('shell.navigation.main');
	const moreLabel = t('common.more');
	const moreActive = $derived(more.some((group) => group.items.some((item) => current === item.id)));
</script>

<nav class={['bottom-nav', className]} aria-label={navigationLabel} {...rest}>
	<ul class="bar">
		{#each items as item (item.id)}
			<li>
				<a
					class="tab"
					href={item.href}
					aria-current={current === item.id ? 'page' : undefined}
					data-state={current === item.id ? 'selected' : undefined}
				>
					<Icon name={item.icon} size="md" />
					<span class="tab-label">{item.label}</span>
				</a>
			</li>
		{/each}
		{#if 0 < more.length}
			<li>
				<Popover label={moreLabel} side="top">
					{#snippet trigger(props)}
						<button type="button" class="tab" data-state={moreActive ? 'selected' : undefined} {...props}>
							<Icon name="more" size="md" />
							<span class="tab-label">{moreLabel}</span>
						</button>
					{/snippet}
					{#snippet content()}
						<div class="more">
							{#each more as group (group.id)}
								<section class="more-group" aria-labelledby={`bottom-nav-group-${group.id}`}>
									<Text as="h2" id={`bottom-nav-group-${group.id}`} size="caption" tone="muted" weight={600}>{group.label}</Text>
									<ul class="more-list">
										{#each group.items as item (item.id)}
											<li>
												<a
													class="more-link"
													href={item.href}
													aria-current={current === item.id ? 'page' : undefined}
													data-state={current === item.id ? 'selected' : undefined}
												>
													<Icon name={item.icon} size="md" />
													<span>{item.label}</span>
												</a>
											</li>
										{/each}
									</ul>
								</section>
							{/each}
						</div>
					{/snippet}
				</Popover>
			</li>
		{/if}
	</ul>
</nav>

<style>
	.bottom-nav {
		position: fixed;
		inset-inline: 0;
		inset-block-end: 0;
		padding-block-end: env(safe-area-inset-bottom);
		background: var(--surface-raised);
		box-shadow: var(--shadow-md);
		color: var(--text-muted);
		z-index: var(--z-topbar);
	}

	.bar {
		display: flex;
		align-items: stretch;
		min-block-size: var(--rail-w);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.bar > li {
		display: flex;
		flex: 1 1 0;
		min-inline-size: 0;
	}

	.tab {
		flex: 1 1 auto;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: var(--space-0-5);
		min-inline-size: var(--touch-target);
		min-block-size: var(--touch-target);
		padding-inline: var(--space-1);
		border: 0;
		background: none;
		color: inherit;
		font: inherit;
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		text-decoration: none;
	}

	.tab[data-state='selected'] {
		color: var(--accent-text);
	}

	.tab:focus-visible,
	.more-link:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.tab-label {
		max-inline-size: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.more {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		padding: var(--space-2);
	}

	.more-group {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
	}

	.more-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.more-link {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		border-radius: var(--radius-md);
		color: var(--text);
		text-decoration: none;
	}

	.more-link[data-state='selected'] {
		background: var(--accent-soft);
		color: var(--accent-text);
	}

	@media (hover: hover) {
		.more-link:hover {
			background: var(--surface-hover);
		}
	}

	@media (min-width: 640px) {
		.bottom-nav {
			display: none;
		}
	}
</style>
