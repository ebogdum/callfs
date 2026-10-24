<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Cluster from '../quarks/Cluster.svelte';
	import Icon from '../quarks/Icon.svelte';
	import type { IconName } from '../types';

	export const TOKENS = ['--storage-cell-fg', '--storage-cell-font-size'] as const;

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
	}

	interface Placement {
		icon: IconName;
		word: string;
	}

	const PLACEMENTS: Record<string, Placement> = {
		localfs: { icon: 'hard-drive', word: 'placement.storage.localfs' },
		s3: { icon: 'cloud', word: 'placement.storage.s3' },
		erasure: { icon: 'split', word: 'placement.storage.erasure' },
	};
	const OTHER_PLACEMENT: Placement = { icon: 'server', word: 'placement.storage.other' };

	let { value, class: className, ...rest }: Props = $props();

	const placement = $derived(Object.hasOwn(PLACEMENTS, value) ? PLACEMENTS[value] : OTHER_PLACEMENT);
</script>

<span class={['storage-cell', className]} data-kind={value} {...rest}>
	<Cluster as="span" gap="2" align="center" wrap={false}>
		<Icon name={placement.icon} size="sm" />
		<span>{t(placement.word)}</span>
	</Cluster>
</span>

<style>
	.storage-cell {
		display: inline-flex;
		align-items: center;
		min-inline-size: 0;
		max-inline-size: 100%;
		color: var(--storage-cell-fg, var(--text));
		font-size: var(--storage-cell-font-size, var(--table-text, var(--text-sm)));
		white-space: nowrap;
	}
</style>
