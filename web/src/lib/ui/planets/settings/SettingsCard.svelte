<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
		title: string;
		description?: string;
		tone?: 'default' | 'danger';
		flush?: boolean;
		actions?: Snippet;
		footer?: Snippet;
		children: Snippet;
	}

	let {
		title,
		description,
		tone = 'default',
		flush = false,
		actions,
		footer,
		children,
		class: className,
		...rest
	}: Props = $props();

	const headingId = $props.id();
</script>

<section class={['settings-card', className]} class:danger={'danger' === tone} aria-labelledby={headingId} {...rest}>
	<header class="head">
		<div class="title-block">
			<h2 class="title" id={headingId}>{title}</h2>
			{#if description}<p class="description">{description}</p>{/if}
		</div>
		{#if actions}<div class="actions">{@render actions()}</div>{/if}
	</header>
	<div class="body" class:flush>
		{@render children()}
	</div>
	{#if footer}<footer class="foot">{@render footer()}</footer>{/if}
</section>

<style>
	.settings-card {
		display: flex;
		flex-direction: column;
		min-inline-size: 0;
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface);
		box-shadow: var(--shadow-xs);
	}

	.settings-card.danger {
		border-color: var(--danger-border);
	}

	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		justify-content: space-between;
		gap: var(--space-3);
		padding: var(--card-pad);
		padding-block-end: var(--space-4);
	}

	.title-block {
		display: flex;
		flex: 1 1 calc(var(--space-4) * 16);
		flex-direction: column;
		gap: var(--space-1);
		min-inline-size: 0;
	}

	.title {
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
		font-weight: var(--text-title-sm-weight);
		letter-spacing: var(--text-title-sm-tracking);
		color: var(--text);
	}

	.danger .title {
		color: var(--danger-text);
	}

	.description {
		max-inline-size: var(--read-max);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		color: var(--text-muted);
		text-wrap: pretty;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		min-inline-size: 0;
		padding: 0 var(--card-pad) var(--card-pad);
	}

	.body.flush {
		gap: 0;
		padding: 0;
		border-block-start: var(--hairline) solid var(--border-subtle);
	}

	.foot {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
		padding: var(--space-3) var(--card-pad);
		border-block-start: var(--hairline) solid var(--border-subtle);
		background: var(--bg-subtle);
		border-end-start-radius: var(--radius-lg);
		border-end-end-radius: var(--radius-lg);
	}
</style>
