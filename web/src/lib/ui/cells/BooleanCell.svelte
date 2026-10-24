<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';

	interface Props extends Omit<HTMLAttributes<HTMLSpanElement>, 'class' | 'children'> {
		value: boolean | null;
		trueLabel: string;
		falseLabel: string;
		class?: string;
	}

	let { value, trueLabel, falseLabel, class: className, ...rest }: Props = $props();

	const valueState = $derived(true === value ? 'true' : false === value ? 'false' : 'unknown');
	const glyph = $derived<IconName>(true === value ? 'check' : false === value ? 'x' : 'minus');
	const word = $derived(true === value ? trueLabel : false === value ? falseLabel : t('common.unknown'));
</script>

<span class={['boolean-cell', className]} data-value={valueState} {...rest}>
	<Icon name={glyph} size="sm" />
	<span class="word">{word}</span>
</span>

<style>
	.boolean-cell {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		white-space: nowrap;
		color: var(--text);
	}

	.boolean-cell[data-value='false'],
	.boolean-cell[data-value='unknown'] {
		color: var(--text-muted);
	}
</style>
