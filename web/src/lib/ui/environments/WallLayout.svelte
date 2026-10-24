<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import Grid from '../quarks/Grid.svelte';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		children: Snippet;
	}

	const FONT_SCALE = '1.5';
	const IDLE_MS = 3000;

	let { children, class: className, ...rest }: Props = $props();

	let idle = $state(false);
	let idleTimer: number | undefined;

	function arm(): void {
		idle = false;
		window.clearTimeout(idleTimer);
		idleTimer = window.setTimeout(() => {
			idle = true;
		}, IDLE_MS);
	}

	$effect(() => {
		const root = document.documentElement;
		const previousContrast = root.dataset.contrast;
		const previousTheme = root.dataset.theme;
		root.dataset.theme = 'dark';
		const previousScale = root.style.getPropertyValue('--font-scale');
		root.dataset.contrast = 'more';
		root.style.setProperty('--font-scale', FONT_SCALE);
		return () => {
			if (undefined === previousTheme) {
				delete root.dataset.theme;
			} else {
				root.dataset.theme = previousTheme;
			}
			if (undefined === previousContrast) {
				delete root.dataset.contrast;
			} else {
				root.dataset.contrast = previousContrast;
			}
			root.style.setProperty('--font-scale', previousScale);
		};
	});

	$effect(() => {
		arm();
		return () => {
			window.clearTimeout(idleTimer);
		};
	});
</script>

<svelte:window onpointermove={arm} onkeydown={arm} />

<div class={['wall-layout', className]} data-state={idle ? 'idle' : 'active'} {...rest}>
	<Grid min="24" gap="6" class="wall-grid">
		{@render children()}
	</Grid>
</div>

<style>
	.wall-layout {
		min-block-size: 100dvh;
		padding: var(--space-6);
		background: var(--surface);
		color: var(--text);
	}

	.wall-layout :global(.wall-grid) {
		grid-template-columns: minmax(0, 1fr);
		min-block-size: calc(100dvh - 2 * var(--space-6));
		align-content: start;
	}

	.wall-layout[data-state='idle'] {
		cursor: none;
	}
</style>
