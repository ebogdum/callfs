<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		axis?: 'y' | 'x' | 'both';
		label?: string;
		children: Snippet;
	}

	let { axis = 'y', label, class: className, children, ...rest }: Props = $props();

	let element: HTMLDivElement | undefined = $state();
	let scrollable = $state(false);

	$effect(() => {
		const node = element;
		if (undefined === node) {
			return;
		}
		const vertical = 'y' === axis || 'both' === axis;
		const horizontal = 'x' === axis || 'both' === axis;
		const measure = () => {
			scrollable =
				(vertical && node.scrollHeight > node.clientHeight) ||
				(horizontal && node.scrollWidth > node.clientWidth);
		};
		measure();
		const observer = 'function' === typeof ResizeObserver ? new ResizeObserver(measure) : undefined;
		observer?.observe(node);
		if (null !== node.firstElementChild) {
			observer?.observe(node.firstElementChild);
		}
		return () => observer?.disconnect();
	});
</script>

<div
	bind:this={element}
	class={['scroll', className]}
	data-axis={axis}
	tabindex={scrollable ? 0 : undefined}
	role={scrollable ? 'region' : undefined}
	aria-label={scrollable ? label : undefined}
	{...rest}
>
	{@render children()}
</div>

<style>
	.scroll {
		min-inline-size: 0;
		background:
			linear-gradient(var(--surface), var(--surface)) top / 100% var(--space-4) no-repeat local,
			linear-gradient(var(--surface), var(--surface)) bottom / 100% var(--space-4) no-repeat local,
			linear-gradient(color-mix(in srgb, var(--text) 12%, transparent), transparent) top / 100%
				var(--space-2) no-repeat scroll,
			linear-gradient(transparent, color-mix(in srgb, var(--text) 12%, transparent)) bottom / 100%
				var(--space-2) no-repeat scroll;
	}

	.scroll[data-axis='y'] {
		overflow-y: auto;
		overflow-x: hidden;
	}

	.scroll[data-axis='x'] {
		overflow-x: auto;
		overflow-y: hidden;
	}

	.scroll[data-axis='both'] {
		overflow: auto;
	}

	.scroll:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}
</style>
