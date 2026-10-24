<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Bytes from '$lib/ui/atoms/Bytes.svelte';
	import Button from '$lib/ui/atoms/Button.svelte';
	import IconButton from '$lib/ui/atoms/IconButton.svelte';
	import Num from '$lib/ui/atoms/Num.svelte';
	import Menu from '$lib/ui/molecules/Menu.svelte';
	import Hairline from '$lib/ui/quarks/Hairline.svelte';
	import type { MenuItem } from '$lib/ui/types';

	const VISIBLE_ACTION_COUNT = 3;

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		count: number;
		bytes?: number | null;
		allMatching: boolean;
		total?: number;
		actions: MenuItem[];
		onselectall: () => void;
		onclear: () => void;
		class?: string;
	}

	let {
		count,
		bytes,
		allMatching,
		total,
		actions,
		onselectall,
		onclear,
		class: className,
		...rest
	}: Props = $props();

	const visibleActions = $derived(actions.slice(0, VISIBLE_ACTION_COUNT));
	const overflowActions = $derived(actions.slice(VISIBLE_ACTION_COUNT));
	const shownCount = $derived(allMatching && undefined !== total ? total : count);
	const canSelectAll = $derived(!allMatching && undefined !== total && count < total);
</script>

{#if 1 <= count}
	<div class={['bulk-bar', className]} role="region" aria-label={t('u091datatable.bulk_label')} {...rest}>
		<div class="summary">
			<span class="count">
				<Num value={shownCount} />
				{allMatching ? t('u091datatable.matching_selected') : t('u091datatable.selected')}
			</span>
			{#if 'number' === typeof bytes}
				<Hairline orientation="vertical" />
				<Bytes value={bytes} />
			{/if}
		</div>
		<div class="actions">
			{#each visibleActions as action, index (action.id)}
				<Button
					variant={0 === index ? 'primary' : 'secondary'}
					iconStart={action.icon}
					disabledReason={action.disabledReason}
					onclick={() => action.onselect()}>{action.label}</Button
				>
			{/each}
			{#if canSelectAll}
				<Button variant="ghost" onclick={onselectall}>
					{t('u091datatable.select_all', { total: formatNumber(total ?? count) })}
				</Button>
			{/if}
			{#if 0 < overflowActions.length}
				<Menu items={overflowActions} label={t('u091datatable.more_actions')}>
					{#snippet trigger(props)}
						<IconButton {...props} icon="more" label={t('u091datatable.more_actions')} />
					{/snippet}
				</Menu>
			{/if}
			<Button variant="ghost" iconStart="x" onclick={onclear}>{t('u091datatable.clear')}</Button>
		</div>
	</div>
{/if}

<style>
	.bulk-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-overlay);
		box-shadow: var(--shadow-md);
		color: var(--text);
		font-size: var(--text-sm-size);
	}

	.summary,
	.actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}

	.count {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
	}
</style>
