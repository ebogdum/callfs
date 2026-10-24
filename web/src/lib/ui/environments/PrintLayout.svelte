<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatAbsolute } from '$lib/format/time';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
	}

	let { children, class: className, ...rest }: Props = $props();

	const generatedAt = new Date();

	$effect(() => {
		const root = document.documentElement;
		const previousTheme = root.dataset.theme;
		root.dataset.theme = 'light';
		return () => {
			if (undefined === previousTheme) {
				delete root.dataset.theme;
			} else {
				root.dataset.theme = previousTheme;
			}
		};
	});
</script>

<div class={['print-layout', className]} {...rest}>
	<main class="print-body">
		{@render children()}
	</main>
	<div class="print-footer">
		<time datetime={generatedAt.toISOString()}>{formatAbsolute(generatedAt)}</time>
	</div>
</div>

<style>
	@page {
		size: A4;
		margin: 16mm;
		@bottom-right {
			content: counter(page) ' / ' counter(pages);
		}
	}

	.print-layout {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
		background: var(--surface);
		color: var(--text);
	}

	.print-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
	}

	.print-footer {
		display: flex;
		justify-content: space-between;
		gap: var(--space-2);
		color: var(--text-muted);
	}
</style>
