<script lang="ts" module>
	export type SkeletonShape = 'text' | 'rect' | 'circle';
	export const TOKENS = ['--skeleton-base', '--skeleton-shine', '--skeleton-radius', '--skeleton-line-h'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class'> {
		shape?: SkeletonShape;
		lines?: number;
		width?: string;
		height?: string;
		class?: string;
	}

	let { shape = 'text', lines = 1, width, height, class: className, ...rest }: Props = $props();

	const LENGTH_PATTERN = /^(?:\d{1,3}(?:\.\d+)?%|var\(--[a-z0-9-]+\))$/;

	const lengthOrUndefined = (length: string | undefined): string | undefined =>
		undefined !== length && LENGTH_PATTERN.test(length) ? length : undefined;

	const lineCount = $derived(Math.max(1, Math.floor(lines)));
	const lineIndexes = $derived(Array.from({ length: lineCount }, (_, index) => index));
	const resolvedWidth = $derived(lengthOrUndefined(width));
	const resolvedHeight = $derived(lengthOrUndefined(height));

	const widthForLine = (index: number): string => {
		if (undefined !== resolvedWidth) {
			return resolvedWidth;
		}
		return 1 < lineCount && lineCount - 1 === index ? '60%' : '100%';
	};
</script>

<div class={['skeleton', className]} data-shape={shape} aria-hidden="true" {...rest}>
	{#if 'text' === shape}
		{#each lineIndexes as index (index)}
			<span class="line" style:inline-size={widthForLine(index)} style:block-size={resolvedHeight}></span>
		{/each}
	{:else}
		<span class="block" style:inline-size={resolvedWidth} style:block-size={resolvedHeight}></span>
	{/if}
</div>

<style>
	.skeleton {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: var(--space-2);
	}

	.line,
	.block {
		display: block;
		flex: none;
		max-inline-size: 100%;
		background-color: var(--skeleton-base, var(--surface-sunken));
		background-image: linear-gradient(90deg, transparent 0%, var(--skeleton-shine, var(--surface-hover)) 50%, transparent 100%);
		background-size: 200% 100%;
		background-repeat: no-repeat;
		border-radius: var(--skeleton-radius, var(--radius-xs));
		animation: shimmer var(--dur-shimmer) linear infinite;
	}

	.line {
		inline-size: 100%;
		block-size: var(--skeleton-line-h, var(--space-4));
	}

	.block {
		inline-size: 100%;
		block-size: var(--space-16);
		border-radius: var(--skeleton-radius, var(--radius-md));
	}

	.skeleton[data-shape='circle'] .block {
		inline-size: var(--space-10);
		block-size: auto;
		aspect-ratio: 1 / 1;
		border-radius: var(--radius-full);
	}

	@media (prefers-reduced-motion: reduce) {
		:global(html:not([data-motion='full'])) .line,
		:global(html:not([data-motion='full'])) .block {
			animation: none;
		}
	}

	:global(html[data-motion='reduced']) .line,
	:global(html[data-motion='reduced']) .block,
	:global(html[data-motion='none']) .line,
	:global(html[data-motion='none']) .block {
		animation: none;
	}

	@keyframes shimmer {
		from {
			background-position: 100% 0;
		}

		to {
			background-position: -100% 0;
		}
	}
</style>
