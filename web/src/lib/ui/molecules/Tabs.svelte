<script lang="ts" module>
	export interface TabItem {
		id: string;
		label: string;
		count?: number;
		href?: string;
	}
</script>

<script lang="ts">
	import { Tabs } from 'bits-ui';
	import Badge from '../atoms/Badge.svelte';
	import { formatNumber } from '$lib/format/number';

	interface Props extends Omit<Tabs.RootProps, 'value' | 'children' | 'child'> {
		value?: string;
		items: TabItem[];
		label: string;
		size?: 'md' | 'sm';
	}

	let { value = $bindable(''), items, label, size = 'md', class: className = '', ...rest }: Props = $props();
</script>

<Tabs.Root bind:value class="tabs {className}" data-size={size} {...rest}>
	<Tabs.List aria-label={label} class="list">
		{#each items as item (item.id)}
			<Tabs.Trigger value={item.id} class="tab">
				{#snippet child({ props })}
					{#if undefined !== item.href}
						<a {...props} href={item.href}>
							<span class="text">{item.label}</span>
							{#if undefined !== item.count}<Badge tone="neutral">{formatNumber(item.count, 'int')}</Badge>{/if}
						</a>
					{:else}
						<button {...props} type="button">
							<span class="text">{item.label}</span>
							{#if undefined !== item.count}<Badge tone="neutral">{formatNumber(item.count, 'int')}</Badge>{/if}
						</button>
					{/if}
				{/snippet}
			</Tabs.Trigger>
		{/each}
	</Tabs.List>
</Tabs.Root>

<style>
	:global(.tabs) {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	:global(.tabs .list) {
		display: flex;
		align-items: stretch;
		gap: var(--space-1);
		max-inline-size: 100%;
		overflow-x: auto;
		scrollbar-width: none;
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	:global(.tabs .tab) {
		display: inline-flex;
		align-items: center;
		flex: none;
		gap: var(--space-2);
		min-block-size: calc(var(--control-h) + var(--space-2));
		padding-inline: var(--space-3);
		border: 0;
		border-radius: var(--radius-sm) var(--radius-sm) 0 0;
		background: transparent;
		color: var(--text-muted);
		font: inherit;
		font-weight: var(--text-control-weight);
		text-decoration: none;
		cursor: pointer;
		transition: color var(--dur-micro) var(--ease-in-out), box-shadow var(--dur-micro) var(--ease-in-out);
	}

	:global(.tabs[data-size='sm'] .list) {
		gap: 0;
	}

	:global(.tabs[data-size='sm'] .tab) {
		gap: var(--space-1-5);
		min-block-size: var(--control-h);
		padding-inline: var(--space-2);
		font-size: var(--text-sm-size);
	}

	@media (max-width: 639px) {
		:global(.tabs .list) {
			gap: 0;
		}

		:global(.tabs .tab) {
			gap: var(--space-1-5);
			padding-inline: var(--space-2);
			font-size: var(--text-sm-size);
		}
	}

	:global(.tabs .tab > a),
	:global(.tabs .tab > button) {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: inherit;
		font: inherit;
		text-decoration: none;
	}

	@media (hover: hover) {
		:global(.tabs .tab:hover) {
			color: var(--text);
			background: var(--surface-hover);
		}
	}

	:global(.tabs .tab[data-state='active']) {
		color: var(--text);
		box-shadow: inset 0 calc(-1 * var(--focus-width)) 0 var(--accent);
	}

	:global(.tabs .tab:focus-visible) {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}

	.text {
		white-space: nowrap;
	}
</style>
