<script lang="ts" module>
	export const TOKENS = ['--key-cell-fg'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import CopyButton from '../atoms/CopyButton.svelte';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

<span class={['key-cell', className]} {...rest}>
	<MiddleTruncate text={value} mono />
	<CopyButton {value} label={t('u065cells.copy_key')} />
</span>

<style>
	.key-cell {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		max-inline-size: 100%;
		min-inline-size: 0;
		font-size: var(--table-text);
		color: var(--key-cell-fg, var(--text));
	}
</style>
