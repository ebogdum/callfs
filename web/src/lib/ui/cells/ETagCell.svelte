<script lang="ts" module>
	export const TOKENS = ['--etag-cell-fg', '--etag-cell-empty-fg'] as const;

	const VISIBLE_CHARACTERS = 12;
	const ELLIPSIS = '…';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import CopyButton from '../atoms/CopyButton.svelte';
	import StatusGlyph from '../quarks/StatusGlyph.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
	}

	let { value, class: className, ...rest }: Props = $props();

	const shown = $derived(
		VISIBLE_CHARACTERS < value.length ? `${value.slice(0, VISIBLE_CHARACTERS)}${ELLIPSIS}` : value
	);
</script>

{#if '' === value}
	<span class={['etag-cell', className]} data-tone="warn" {...rest}>
		<StatusGlyph tone="warn" size="sm" />
		<span class="empty">{t('cells.etag.not_set')}</span>
	</span>
{:else}
	<span class={['etag-cell', className]} {...rest}>
		<span class="value" title={value}>{shown}</span>
		<CopyButton {value} label={t('cells.etag.copy')} />
	</span>
{/if}

<style>
	.etag-cell {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		max-inline-size: 100%;
		font-size: var(--table-text);
		color: var(--etag-cell-fg, var(--text));
	}

	.value {
		font-family: var(--font-mono);
		white-space: nowrap;
	}

	.etag-cell[data-tone='warn'] {
		gap: var(--space-1-5);
		color: var(--etag-cell-empty-fg, var(--warn-text));
	}
</style>
