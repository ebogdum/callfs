<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { modeToSymbolic } from '$lib/format/mode';
	import VisuallyHidden from '../quarks/VisuallyHidden.svelte';

	export const TOKENS = ['--mode-cell-fg', '--mode-cell-font-size'] as const;

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
	}

	let { value, class: className, ...rest }: Props = $props();

	const symbolic = $derived(modeToSymbolic(value));
	const symbolicText = $derived(symbolic.ok ? symbolic.value : t('placement.mode.invalid'));
</script>

<span
	class={['mode-cell', className]}
	title={symbolicText}
	{...rest}
>
	<span class="digits">{value}</span>
	<VisuallyHidden>{symbolicText}</VisuallyHidden>
</span>

<style>
	.mode-cell {
		display: inline-flex;
		align-items: center;
		min-inline-size: 0;
		max-inline-size: 100%;
		color: var(--mode-cell-fg, var(--text));
		font-family: var(--font-mono);
		font-size: var(--mode-cell-font-size, var(--table-text, var(--text-sm)));
		white-space: nowrap;
	}
</style>
