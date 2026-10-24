<script lang="ts">
	import type { Snippet } from 'svelte';
	import { MediaQuery } from 'svelte/reactivity';
	import Icon from '../quarks/Icon.svelte';
	import SectionNav from './SectionNav.svelte';
	import type { SectionNavGroup } from './SectionNav.svelte';

	interface Props {
		groups: SectionNavGroup[];
		current: string;
		label: string;
		header?: Snippet;
	}

	let { groups, current, label, header }: Props = $props();

	const narrow = new MediaQuery('max-width: 839px');
	let navOpen = $state(false);
	const currentItem = $derived(groups.flatMap((group) => group.items).find((item) => current === item.id));

	$effect(() => {
		void current;
		navOpen = false;
	});
</script>

{#snippet sideContent()}
	{#if header}{@render header()}{/if}
	<SectionNav {groups} {current} {label} />
{/snippet}

{#if narrow.current}
	<details class="fold" bind:open={navOpen}>
		<summary class="fold-summary">
			<span class="fold-label">
				<span class="fold-caption">{label}</span>
				<span class="fold-current">
					{#if undefined !== currentItem}<Icon name={currentItem.icon} size="sm" />{/if}
					<span>{currentItem?.label ?? label}</span>
				</span>
			</span>
			<span class="fold-chevron" aria-hidden="true"><Icon name="chevron-down" size="sm" /></span>
		</summary>
		<div class="side fold-body">
			{@render sideContent()}
		</div>
	</details>
{:else}
	<aside class="side">
		{@render sideContent()}
	</aside>
{/if}

<style>
	@media (min-width: 840px) {
		.side {
			position: sticky;
			inset-block-start: calc(var(--topbar-h) + var(--space-8));
			max-block-size: calc(100dvh - var(--topbar-h) - var(--space-12));
			overflow-y: auto;
		}
	}

	.side {
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
		min-inline-size: 0;
		padding: var(--space-0-5);
	}

	.fold {
		min-inline-size: 0;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface);
	}

	.fold-summary {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		min-block-size: var(--target-min);
		padding: var(--space-2) var(--space-4);
		border-radius: var(--radius-lg);
		list-style: none;
		cursor: pointer;
		outline: var(--focus-width) solid transparent;
	}

	.fold-summary::-webkit-details-marker {
		display: none;
	}

	.fold-summary:focus-visible {
		box-shadow: var(--focus-ring);
	}

	.fold-label {
		display: flex;
		flex-direction: column;
		gap: var(--space-0-5);
		min-inline-size: 0;
	}

	.fold-caption {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
		font-weight: var(--text-caption-weight);
	}

	.fold-current {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		color: var(--text);
		font-weight: var(--text-control-weight);
	}

	.fold-chevron {
		display: inline-flex;
		color: var(--text-muted);
		transition: rotate var(--dur-micro) var(--ease-in-out);
	}

	.fold[open] .fold-chevron {
		rotate: 180deg;
	}

	.fold-body {
		padding: var(--space-2) var(--space-3) var(--space-4);
		border-block-start: var(--hairline) solid var(--border-subtle);
	}
</style>
