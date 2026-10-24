<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { DASH } from '$lib/format/number';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string | null;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

{#if null === value || '' === value}
	<span class={['ip-cell', className]} data-state="empty" {...rest}>
		{DASH}
		<VisuallyHidden>{t('u065cells.unknown')}</VisuallyHidden>
	</span>
{:else}
	<span class={['ip-cell', className]} {...rest}>
		<MiddleTruncate text={value} mono />
	</span>
{/if}

<style>
	.ip-cell {
		display: inline-flex;
		align-items: center;
		max-inline-size: 100%;
		min-inline-size: 0;
		font-size: var(--table-text);
		font-variant-numeric: tabular-nums;
		color: var(--ip-cell-fg, var(--text));
	}

	.ip-cell[data-state='empty'] {
		color: var(--ip-cell-empty-fg, var(--text-muted));
	}
</style>
