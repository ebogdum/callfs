<script lang="ts" module>
	export type DurationDisplay = 'long' | 'short';
	export const TOKENS = ['--duration-color'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { formatDuration, parseGoDuration } from '$lib/format/duration';
	import { t } from '$lib/copy/t';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: number | string;
		style?: DurationDisplay;
	}

	let { value, style = 'long', class: className, ...rest }: Props = $props();

	const milliseconds = $derived.by(() => {
		if ('number' === typeof value) {
			return value;
		}
		const parsed = parseGoDuration(value);
		return parsed.ok ? parsed.value : null;
	});
	const text = $derived(formatDuration(milliseconds, style));
	const longText = $derived(formatDuration(milliseconds, 'long'));
</script>

<span class={['duration', className]} title={'short' === style ? longText : undefined} {...rest}>
	{text}
	{#if null === milliseconds || !Number.isFinite(milliseconds)}<VisuallyHidden>{t('common.unknown')}</VisuallyHidden>{/if}
</span>

<style>
	.duration {
		color: var(--duration-color, inherit);
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}
</style>
