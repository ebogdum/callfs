<script lang="ts">
	import type { Snippet } from 'svelte';
	import { DEFAULT_OVERSCAN, visibleRange } from './table-state.svelte';

	interface Props {
		count: number;
		rowHeight: number;
		overscan?: number;
		scrollEl: HTMLElement | null;
		render: Snippet<[number]>;
	}

	let { count, rowHeight, overscan = DEFAULT_OVERSCAN, scrollEl, render }: Props = $props();

	let scrollTop = $state(0);
	let viewport = $state(0);

	$effect(() => {
		const element = scrollEl;
		if (null === element) {
			return;
		}
		const measure = () => {
			scrollTop = element.scrollTop;
			viewport = element.clientHeight;
		};
		measure();
		element.addEventListener('scroll', measure, { passive: true });
		return () => {
			element.removeEventListener('scroll', measure);
		};
	});

	const range = $derived(visibleRange(count, rowHeight, overscan, scrollTop, viewport));
	const indices = $derived(Array.from({ length: range.last - range.first }, (_, offset) => range.first + offset));
	const topPx = $derived(range.first * rowHeight);
	const bottomPx = $derived((count - range.last) * rowHeight);
</script>

<tr class="spacer" aria-hidden="true" style:height={`${topPx}px`}></tr>
{#each indices as index (index)}
	{@render render(index)}
{/each}
<tr class="spacer" aria-hidden="true" style:height={`${bottomPx}px`}></tr>
