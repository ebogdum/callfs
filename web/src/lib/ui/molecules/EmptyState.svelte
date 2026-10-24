<script lang="ts" module>
	export type EmptyStateVariant = 'first-use' | 'no-results' | 'cleared' | 'no-permission' | 'not-available';

	export type EmptyStatePrimary = { label: string; onclick: () => void } | { label: string; href: string };

	export const TOKENS = [
		'--empty-state-pad',
		'--empty-state-gap',
		'--empty-state-art-size',
		'--empty-state-art-fg'
	] as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Button from '../atoms/Button.svelte';
	import Link from '../atoms/Link.svelte';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		variant: EmptyStateVariant;
		icon: IconName;
		title: string;
		body?: string;
		compact?: boolean;
		primary?: EmptyStatePrimary;
		secondary?: { label: string; href: string } | { label: string; onclick: () => void; icon?: IconName };
		example?: Snippet;
	}

	let { variant, icon, title, body, compact = false, primary, secondary, example, class: className, ...rest }: Props = $props();
</script>

<div {...rest} class={['empty-state', className]} data-variant={variant} data-compact={compact ? 'true' : undefined}>
	{#if !compact}
		<span class="art"><Icon name={icon} size="xl" /></span>
	{/if}
	<div class="copy">
		{#if compact}
			<p class="title">{title}</p>
		{:else}
			<h2 class="title">{title}</h2>
		{/if}
		{#if undefined !== body}
			<p class="body">{body}</p>
		{/if}
	</div>
	{#if undefined !== example}<div class="example">{@render example()}</div>{/if}
	{#if undefined !== primary || undefined !== secondary}
		<div class="actions">
			{#if undefined !== primary}
				{#if 'href' in primary}
					<a class="primary-link" href={primary.href}>{primary.label}</a>
				{:else}
					<Button variant="primary" onclick={primary.onclick}>{primary.label}</Button>
				{/if}
			{/if}
			{#if undefined !== secondary}
				{#if 'href' in secondary}
					<Link href={secondary.href}>{secondary.label}</Link>
				{:else}
					<Button variant="secondary" iconStart={secondary.icon} onclick={secondary.onclick}>{secondary.label}</Button>
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	.empty-state {
		display: grid;
		justify-items: center;
		align-content: center;
		gap: var(--empty-state-gap, var(--space-4));
		max-inline-size: var(--read-max);
		margin-inline: auto;
		padding: var(--empty-state-pad, var(--space-8));
		text-align: center;
		color: var(--text);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		letter-spacing: var(--text-body-tracking);
	}

	.empty-state[data-compact='true'] {
		justify-items: start;
		align-content: start;
		gap: var(--space-2);
		max-inline-size: none;
		margin-inline: 0;
		padding: var(--empty-state-pad, var(--space-2) 0);
		text-align: start;
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.empty-state[data-compact='true'] .copy {
		justify-items: start;
		gap: var(--space-0-5);
	}

	.empty-state[data-compact='true'] .title {
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-control-weight);
		letter-spacing: normal;
	}

	.empty-state[data-compact='true'] .actions {
		justify-content: flex-start;
		padding-block-start: 0;
	}

	.art {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		inline-size: var(--empty-state-art-size, var(--space-12));
		block-size: var(--empty-state-art-size, var(--space-12));
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		box-shadow: var(--shadow-sm);
		color: var(--empty-state-art-fg, var(--text-muted));
	}

	.empty-state[data-variant='no-permission'] .art,
	.empty-state[data-variant='not-available'] .art {
		background: var(--surface-sunken);
		box-shadow: none;
	}

	.copy {
		display: grid;
		justify-items: center;
		gap: var(--space-1-5);
	}

	.title {
		margin: 0;
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		text-wrap: balance;
	}

	.body {
		margin: 0;
		color: var(--text-muted);
		text-wrap: pretty;
		max-inline-size: calc(var(--ch) * 48);
	}

	.example {
		justify-self: center;
		max-inline-size: 100%;
		color: var(--text-muted);
		text-align: start;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		padding-block-start: var(--space-1);
	}

	.primary-link {
		display: inline-flex;
		align-items: center;
		block-size: var(--control-h);
		padding-inline: var(--control-px);
		border-radius: var(--radius-md);
		background: var(--button-primary-bg, var(--accent));
		box-shadow: var(--shadow-xs);
		color: var(--button-primary-fg, var(--text-on-accent));
		font-weight: var(--text-control-weight);
		text-decoration: none;
		outline: var(--focus-width) solid transparent;
	}

	@media (hover: hover) {
		.primary-link:hover {
			background: var(--accent-hover);
		}
	}

	.primary-link:focus-visible {
		box-shadow: var(--focus-ring);
	}
</style>
