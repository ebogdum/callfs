<script lang="ts" module>
	interface PathSegment {
		text: string;
		marked: boolean;
	}

	const escapeRegExp = (text: string): string => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

	const segmentsOf = (path: string, highlight: string): PathSegment[] => {
		if ('' === highlight) {
			return [{ text: path, marked: false }];
		}
		const needle = highlight.toLowerCase();
		return path
			.split(new RegExp(`(${escapeRegExp(highlight)})`, 'i'))
			.filter((text) => '' !== text)
			.map((text) => ({ text, marked: needle === text.toLowerCase() }));
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import Link from '../atoms/Link.svelte';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		path: string;
		href?: string;
		highlight?: string;
	}

	let { path, href, highlight = '', class: className, ...rest }: Props = $props();

	const segments = $derived(segmentsOf(path, highlight));
	const hasMatch = $derived(segments.some((segment) => segment.marked));
</script>

{#snippet pathText()}
	{#if hasMatch}
		<span class="highlighted" title={path}>
			{#each segments as segment, index (index)}{#if segment.marked}<mark>{segment.text}</mark>{:else}{segment.text}{/if}{/each}
		</span>
	{:else}
		<MiddleTruncate text={path} mono />
	{/if}
{/snippet}

<span class={['path-cell', className]} {...rest}>
	{#if href}
		<Link {href}>{@render pathText()}</Link>
	{:else}
		{@render pathText()}
	{/if}
</span>

<style>
	.path-cell {
		display: block;
		min-inline-size: 0;
		max-inline-size: 100%;
		font-family: var(--font-mono);
		font-size: var(--table-text);
		color: var(--path-cell-fg, var(--text));
	}

	.highlighted {
		display: block;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	mark {
		background: var(--path-cell-mark-bg, var(--accent-soft));
		color: var(--path-cell-mark-fg, var(--accent-text));
		border-radius: var(--radius-xs);
	}
</style>
