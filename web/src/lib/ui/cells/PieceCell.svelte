<script lang="ts" module>
	export type PieceKind = 'data' | 'spare';
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import type { StatusKey } from '$lib/status/catalog';
	import StatusPill from '../atoms/StatusPill.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		index: number;
		kind: PieceKind;
		state: StatusKey;
	}

	let { index, kind, state, class: className, ...rest }: Props = $props();

	const KIND_KEY: Record<PieceKind, string> = {
		data: 'u065cells.piece_data',
		spare: 'u065cells.piece_spare'
	};
</script>

<span class={['piece-cell', className]} {...rest}>
	<span class="title">{t('u065cells.piece_title', { index: formatNumber(index), kind: t(KIND_KEY[kind]) })}</span>
	<StatusPill status={state} size="sm" />
</span>

<style>
	.piece-cell {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		max-inline-size: 100%;
		min-inline-size: 0;
		font-size: var(--table-text);
		color: var(--piece-cell-fg, var(--text));
	}

	.title {
		white-space: nowrap;
	}
</style>
