<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Icon from '$lib/ui/quarks/Icon.svelte';
	import type { IconName } from '$lib/ui/types';

	interface Props extends HTMLAttributes<HTMLElement> {
		title: string;
		description?: string;
		icon?: IconName;
		tone?: 'neutral' | 'warn' | 'danger' | 'ok';
		meta?: Snippet;
		footer?: Snippet;
		children?: Snippet;
	}

	let { title, description, icon, tone = 'neutral', meta, footer, children, class: className, ...rest }: Props = $props();

	const uid = $props.id();
</script>

<section {...rest} class={['card', className]} data-tone={tone} aria-labelledby={`${uid}-title`}>
	<header class="head">
		{#if icon}
			<span class="tile" aria-hidden="true"><Icon name={icon} size="md" /></span>
		{/if}
		<div class="titles">
			<h2 class="title" id={`${uid}-title`}>{title}</h2>
			{#if description}
				<p class="description">{description}</p>
			{/if}
		</div>
		{#if meta}
			<div class="meta">{@render meta()}</div>
		{/if}
	</header>
	{#if children}
		<div class="body">{@render children()}</div>
	{/if}
	{#if footer}
		<footer class="foot">{@render footer()}</footer>
	{/if}
</section>

<style>
	.card {
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
		background: var(--surface-raised);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-xs);
		overflow: hidden;
		container-type: inline-size;
	}

	.head {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: start;
		column-gap: var(--space-3);
		row-gap: var(--space-2);
		padding: var(--card-pad);
		padding-block-end: var(--space-4);
	}

	.tile {
		display: inline-grid;
		place-items: center;
		flex: none;
		inline-size: var(--space-10);
		block-size: var(--space-10);
		border-radius: var(--radius-md);
		background: var(--surface-sunken);
		border: var(--hairline) solid var(--border-subtle);
		color: var(--text-muted);
	}

	.card[data-tone='warn'] .tile {
		background: var(--warn-soft);
		border-color: var(--warn-border);
		color: var(--warn-text);
	}

	.card[data-tone='danger'] .tile {
		background: var(--danger-soft);
		border-color: var(--danger-border);
		color: var(--danger-text);
	}

	.card[data-tone='ok'] .tile {
		background: var(--ok-soft);
		border-color: var(--ok-border);
		color: var(--ok-text);
	}

	.titles {
		display: grid;
		gap: var(--space-1);
		min-inline-size: 0;
		align-self: center;
	}

	.head:not(:has(.tile)) {
		grid-template-columns: minmax(0, 1fr) auto;
	}

	.title {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		color: var(--text);
	}

	.description {
		margin: 0;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
	}

	.meta {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		align-items: center;
		gap: var(--space-2);
	}

	@container (max-width: 30rem) {
		.head {
			grid-template-columns: auto minmax(0, 1fr);
		}

		.meta {
			grid-column: 2;
			justify-content: flex-start;
		}

		.head:not(:has(.tile)) {
			grid-template-columns: minmax(0, 1fr);
		}

		.head:not(:has(.tile)) .meta {
			grid-column: 1;
		}
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		flex: 1 1 auto;
		min-inline-size: 0;
		padding-inline: var(--card-pad);
		padding-block-end: var(--card-pad);
	}

	.foot {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		padding-block: var(--space-3);
		padding-inline: var(--card-pad);
		border-block-start: var(--hairline) solid var(--border-subtle);
		background: var(--bg-subtle);
	}
</style>
