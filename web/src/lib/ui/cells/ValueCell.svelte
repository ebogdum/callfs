<script lang="ts" module>
	import type { Tone } from '../types';

	export const TOKENS = ['--value-cell-fg', '--value-cell-empty-fg'] as const;

	export type ValueSource = 'default' | 'file' | 'env' | 'runtime';

	const SOURCE_TONE: Record<ValueSource, Tone> = {
		default: 'neutral',
		file: 'info',
		env: 'warn',
		runtime: 'accent'
	};
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import { STATUS } from '$lib/status/catalog';
	import Badge from '../atoms/Badge.svelte';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: unknown;
		secret?: boolean;
		source?: ValueSource;
	}

	let { value, secret = false, source, class: className, ...rest }: Props = $props();

	const isUnset = $derived(null === value || undefined === value || '' === value);

	const shown = $derived.by(() => {
		if (isUnset) {
			return t('u065cells.not_set');
		}
		if (secret) {
			return t('u065cells.set');
		}
		if ('boolean' === typeof value) {
			return value ? t('u065cells.yes') : t('u065cells.no');
		}
		if ('number' === typeof value) {
			return formatNumber(value, 'decimal');
		}
		if (Array.isArray(value)) {
			return value.map(String).join(', ');
		}
		return String(value);
	});
</script>

<span class={['value-cell', className]} data-state={isUnset ? 'empty' : undefined} {...rest}>
	<MiddleTruncate text={shown} mono />
	{#if source}
		<Badge tone={SOURCE_TONE[source]}>{t(STATUS[`source.${source}`].word)}</Badge>
	{/if}
</span>

<style>
	.value-cell {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		max-inline-size: 100%;
		min-inline-size: 0;
		font-size: var(--table-text);
		color: var(--value-cell-fg, var(--text));
	}

	.value-cell[data-state='empty'] {
		color: var(--value-cell-empty-fg, var(--text-muted));
	}
</style>
