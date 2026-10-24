<script lang="ts" module>
	export const TOKENS = ['--time-cell-color'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import RelTime from '../atoms/RelTime.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string | null;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

<span class={['time-cell', className]} {...rest}>
	{#if null === value}
		<span aria-hidden="true">—</span><VisuallyHidden>{t('common.unknown')}</VisuallyHidden>
	{:else}
		<RelTime at={value} />
	{/if}
</span>

<style>
	.time-cell {
		display: block;
		font-size: var(--table-text);
		white-space: nowrap;
		color: var(--time-cell-color, inherit);
	}
</style>
