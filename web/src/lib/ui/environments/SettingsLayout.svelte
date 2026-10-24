<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import SectionRail from './SectionRail.svelte';
	import type { SectionNavGroup } from './SectionNav.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		nav: SectionNavGroup[];
		current: string;
		search: string;
		children: Snippet;
		saveBar?: Snippet;
	}

	let {
		nav,
		current,
		search = $bindable(),
		children,
		saveBar,
		class: className,
		...rest
	}: Props = $props();
</script>

{#snippet searchBox()}
	<label class="search">
		<span class="search-label">{t('u111dashboard.settings.search_label')}</span>
		<span class="search-icon" aria-hidden="true"><Icon name="search" size="sm" /></span>
		<input
			class="search-input"
			type="search"
			autocomplete="off"
			placeholder={t('u111dashboard.settings.search_placeholder')}
			bind:value={search}
		/>
	</label>
{/snippet}

<div class={['settings-layout', className]} {...rest}>
	<SectionRail groups={nav} {current} label={t('u111dashboard.settings.nav_label')} header={searchBox} />
	<div class="main">
		<div class="content">
			{@render children()}
		</div>
		{#if saveBar}
			<div class="save-bar">
				{@render saveBar()}
			</div>
		{/if}
	</div>
</div>

<style>
	.settings-layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: var(--space-6);
		min-inline-size: 0;
	}

	@media (min-width: 840px) {
		.settings-layout {
			grid-template-columns: calc(var(--space-4) * 14) minmax(0, 1fr);
			align-items: start;
			gap: var(--space-10);
		}
	}

	.search {
		position: relative;
		display: flex;
		align-items: center;
	}

	.search-label {
		position: absolute;
		inline-size: var(--space-px);
		block-size: var(--space-px);
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.search-icon {
		position: absolute;
		inset-inline-start: var(--space-2);
		display: inline-flex;
		color: var(--text-faint);
		pointer-events: none;
	}

	.search-input {
		inline-size: 100%;
		min-block-size: var(--control-h);
		padding-inline: var(--space-8) var(--control-px);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-md);
		background: var(--surface);
		color: var(--text);
		font: inherit;
		font-size: var(--text-body-size);
		outline: var(--focus-width) solid transparent;
	}

	.search-input::placeholder {
		color: var(--text-faint);
	}

	.search-input:focus-visible {
		border-color: var(--accent-border);
		box-shadow: var(--focus-ring);
	}

	.main {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.content {
		display: flex;
		flex-direction: column;
		gap: var(--space-8);
		max-inline-size: calc(var(--space-4) * 52);
		min-inline-size: 0;
	}

	.save-bar {
		position: sticky;
		inset-block-end: var(--space-4);
		z-index: var(--z-sticky);
		padding: var(--space-3) var(--space-4);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		box-shadow: var(--shadow-md);
	}
</style>
