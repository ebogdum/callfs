<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		title: string;
		job?: string;
		breadcrumbs?: Snippet;
		scope?: Snippet;
		primary?: Snippet;
		secondary?: Snippet;
		tabs?: Snippet;
		toolbar?: Snippet;
		status?: Snippet;
		aside?: Snippet;
		children: Snippet;
	}

	let {
		title,
		job,
		breadcrumbs,
		scope,
		primary,
		secondary,
		tabs,
		toolbar,
		status,
		aside,
		children,
		class: className,
		...rest
	}: Props = $props();

	let heading: HTMLHeadingElement | undefined = $state();
	const hasAside = $derived(undefined !== aside);

	$effect(() => {
		title;
		heading?.focus();
	});
</script>

<svelte:head>
	<title>{`${title} · callfs`}</title>
</svelte:head>

<div class={['page-frame', className]} {...rest}>
	<header class="page-head">
		{#if breadcrumbs}<div class="crumbs">{@render breadcrumbs()}</div>{/if}
		<div class="title-row">
			<div class="title-block">
				<h1 bind:this={heading} class="title" tabindex="-1">{title}</h1>
				{#if job}<p class="job">{job}</p>{/if}
				{#if scope}<div class="scope">{@render scope()}</div>{/if}
			</div>
			{#if status || secondary || primary}
				<div class="actions">
					{#if status}<div class="status">{@render status()}</div>{/if}
					{#if secondary}{@render secondary()}{/if}
					{#if primary}{@render primary()}{/if}
				</div>
			{/if}
		</div>
		{#if tabs}<div class="tabs">{@render tabs()}</div>{/if}
	</header>
	{#if toolbar}
		<div class="toolbar">
			{@render toolbar()}
		</div>
	{/if}
	<div class="body" class:with-aside={hasAside}>
		<div class="content">
			{@render children()}
		</div>
		{#if hasAside}
			<aside class="aside">
				{@render aside?.()}
			</aside>
		{/if}
	</div>
</div>

<style>
	.page-frame {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.page-head {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		min-inline-size: 0;
	}

	.crumbs {
		display: flex;
		min-inline-size: 0;
		color: var(--text-muted);
	}

	.title-row {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		justify-content: space-between;
		gap: var(--space-4);
		min-inline-size: 0;
	}

	.title-block {
		display: flex;
		flex: 1 1 calc(var(--space-4) * 20);
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.title {
		margin: 0;
		font-family: var(--font-sans);
		font-size: var(--text-heading-size);
		line-height: var(--text-heading-line);
		font-weight: var(--text-heading-weight);
		letter-spacing: var(--text-heading-tracking);
		color: var(--text);
		text-wrap: balance;
		overflow-wrap: anywhere;
	}

	.title:focus {
		outline: var(--focus-width) solid transparent;
		box-shadow: none;
	}

	.job {
		margin: 0;
		max-inline-size: var(--read-max);
		font-size: var(--text-body-size);
		line-height: var(--text-body-line);
		color: var(--text-muted);
		text-wrap: pretty;
	}

	.scope {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
		padding-block-start: var(--space-1);
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.status {
		display: flex;
		align-items: center;
		color: var(--text-muted);
	}

	.tabs {
		min-inline-size: 0;
		padding-block-start: var(--space-2);
	}

	.toolbar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.body {
		display: grid;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.content {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		min-inline-size: 0;
	}

	.aside {
		min-inline-size: 0;
	}

	@media (min-width: 1280px) {
		.body.with-aside {
			grid-template-columns: minmax(0, 1fr) var(--inspector-w);
			align-items: start;
		}
	}

	@media (max-width: 639px) {
		.page-frame {
			gap: var(--space-5);
		}

		.actions {
			justify-content: flex-start;
		}
	}
</style>
