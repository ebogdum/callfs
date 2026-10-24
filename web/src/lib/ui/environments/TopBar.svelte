<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class' | 'title'> {
		title: Snippet;
		search: Snippet;
		status: Snippet;
		actions: Snippet;
		account: Snippet;
		class?: string;
	}

	let { title, search, status, actions, account, class: className, ...rest }: Props = $props();
</script>

<header class={['top-bar', className]} {...rest}>
	<div class="title-zone">
		{@render title()}
	</div>
	<div class="search-zone">
		{@render search()}
	</div>
	<div class="end-zone">
		{@render status()}
		<span class="divider" aria-hidden="true"></span>
		{@render actions()}
		<span class="divider" aria-hidden="true"></span>
		{@render account()}
	</div>
</header>

<style>
	.top-bar {
		position: sticky;
		inset-block-start: 0;
		display: grid;
		grid-template-columns: minmax(calc(var(--space-4) * 8), 1fr) minmax(calc(var(--space-4) * 12), calc(var(--space-4) * 28)) auto;
		align-items: center;
		gap: var(--space-4);
		block-size: var(--topbar-h);
		padding-inline: var(--gutter);
		border-block-end: var(--hairline) solid var(--border-subtle);
		background: color-mix(in srgb, var(--bg) 85%, transparent);
		backdrop-filter: saturate(1.4) blur(calc(var(--space-px) * 12));
		z-index: var(--z-topbar);
	}

	.title-zone {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.search-zone {
		display: flex;
		min-inline-size: 0;
	}

	.end-zone {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-2);
		min-inline-size: 0;
	}

	.divider {
		flex: none;
		inline-size: var(--hairline);
		block-size: var(--space-5);
		background: var(--border-subtle);
	}

	@media (max-width: 899px) {
		.top-bar {
			grid-template-columns: minmax(0, 1fr) auto;
		}

		.search-zone {
			display: none;
		}
	}

	@media (max-width: 639px) {
		.top-bar {
			gap: var(--space-2);
		}

		.divider {
			display: none;
		}
	}
</style>
