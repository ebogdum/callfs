<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import Avatar from '../atoms/Avatar.svelte';
	import Badge from '../atoms/Badge.svelte';
	import MiddleTruncate from '../atoms/MiddleTruncate.svelte';
	import Tooltip from '../atoms/Tooltip.svelte';
	import Cluster from '../quarks/Cluster.svelte';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		value: string;
	}

	let { value, class: className, ...rest }: Props = $props();
</script>

<span
	class={['owner-cell', className]}
	data-state={'' === value ? 'empty' : 'set'}
	{...rest}
>
	{#if '' === value}
		<Tooltip content={t('owner.empty_help')}>
			<Badge tone="warn" icon="warn">{t('status.file.no_owner')}</Badge>
		</Tooltip>
	{:else}
		<Cluster as="span" gap="2" align="center" wrap={false}>
			<Avatar name={value} />
			<MiddleTruncate text={value} mono />
		</Cluster>
	{/if}
</span>

<style>
	.owner-cell {
		display: inline-flex;
		align-items: center;
		min-inline-size: 0;
		max-inline-size: 100%;
		color: var(--owner-cell-fg, var(--text));
		font-size: var(--owner-cell-font-size, var(--table-text, var(--text-sm)));
		white-space: nowrap;
	}
</style>
