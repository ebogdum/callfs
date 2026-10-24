<script lang="ts" module>
	export const TOKENS = ['--bytes-color', '--bytes-weight'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatBytes } from '$lib/format/bytes';
	import { t } from '$lib/copy/t';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: number | null;
		precision?: number;
		exactOnHover?: boolean;
	}

	let { value, precision = 1, exactOnHover = true, class: className, ...rest }: Props = $props();

	const formatted = $derived(formatBytes(value, { precision }));
</script>

<span class={['bytes', className]} title={exactOnHover && null !== formatted.exact ? formatted.exact : undefined} {...rest}>
	{formatted.text}{#if null === value}<VisuallyHidden>{t('common.unknown')}</VisuallyHidden>{/if}
</span>

<style>
	.bytes {
		color: var(--bytes-color, inherit);
		font-weight: var(--bytes-weight, inherit);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
</style>
