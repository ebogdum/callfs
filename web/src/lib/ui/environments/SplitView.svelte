<script lang="ts" module>
	export type SplitMode = 'docked' | 'drawer' | 'replace';
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { BREAKPOINTS } from '../quarks/tokens/breakpoints';
	import ResizeHandle from '../molecules/ResizeHandle.svelte';
	import Drawer from '../organisms/Drawer.svelte';

	const DETAIL_MIN = 360;
	const DETAIL_MAX = 480;

	interface Props extends HTMLAttributes<HTMLDivElement> {
		list: Snippet;
		detail: Snippet;
		detailOpen?: boolean;
		detailWidth?: number;
		label: string;
	}

	let {
		list,
		detail,
		detailOpen = $bindable(false),
		detailWidth = $bindable(400),
		label,
		class: className,
		...rest
	}: Props = $props();

	let docked = $state(false);
	let sideBySide = $state(false);

	$effect(() => {
		if ('function' !== typeof window.matchMedia) {
			return;
		}
		const wide = window.matchMedia(`(min-width: ${BREAKPOINTS.xl}px)`);
		const split = window.matchMedia(`(min-width: ${BREAKPOINTS.split}px)`);
		const sync = () => {
			docked = wide.matches;
			sideBySide = split.matches;
		};
		sync();
		wide.addEventListener('change', sync);
		split.addEventListener('change', sync);
		return () => {
			wide.removeEventListener('change', sync);
			split.removeEventListener('change', sync);
		};
	});

	const mode: SplitMode = $derived(docked ? 'docked' : sideBySide ? 'drawer' : 'replace');
	const listCovered = $derived('replace' === mode && detailOpen);

	function closeOnEscape(event: KeyboardEvent): void {
		if ('Escape' === event.key) {
			detailOpen = false;
		}
	}
</script>

<div class={['split', mode, className]} data-mode={mode} {...rest}>
	<div class="list" class:covered={listCovered} role="region" aria-label={label} inert={listCovered}>
		{@render list()}
	</div>
	{#if 'docked' === mode && detailOpen}
		<ResizeHandle orientation="vertical" bind:value={detailWidth} min={DETAIL_MIN} max={DETAIL_MAX} {label} />
		<section class="detail" style:inline-size={`${detailWidth}px`}>
			{@render detail()}
		</section>
	{:else if 'replace' === mode && detailOpen}
		<section class="detail covering" onkeydown={closeOnEscape}>
			{@render detail()}
		</section>
	{:else if 'drawer' === mode}
		<Drawer bind:open={detailOpen} title={label}>
			{@render detail()}
		</Drawer>
	{/if}
</div>

<style>
	.split {
		display: block;
		min-inline-size: 0;
	}

	.split.docked {
		display: flex;
		align-items: stretch;
		gap: var(--space-2);
	}

	.split.replace {
		display: grid;
	}

	.split.replace > * {
		grid-area: 1 / 1;
	}

	.list {
		min-inline-size: 0;
	}

	.split.docked > .list {
		flex: 1 1 0;
	}

	.list.covered {
		visibility: hidden;
	}

	.detail {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		box-sizing: border-box;
		min-inline-size: 0;
		background: var(--bg);
	}

	.split.docked > .detail {
		flex: 0 0 auto;
	}

	.detail.covering {
		inline-size: 100%;
	}
</style>
