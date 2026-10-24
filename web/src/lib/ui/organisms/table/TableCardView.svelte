<script lang="ts" generics="T">
	import type { HTMLAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';
	import { t } from '$lib/copy/t';
	import Button from '$lib/ui/atoms/Button.svelte';
	import Skeleton from '$lib/ui/atoms/Skeleton.svelte';
	import ActionsCell from '$lib/ui/cells/ActionsCell.svelte';
	import CheckboxCell from '$lib/ui/cells/CheckboxCell.svelte';
	import ErrorState from '$lib/ui/molecules/ErrorState.svelte';
	import InlineAlert from '$lib/ui/molecules/InlineAlert.svelte';
	import VisuallyHidden from '$lib/ui/quarks/VisuallyHidden.svelte';
	import type { ColumnDef, MenuItem, QueryStatus, Selection, UiError } from '$lib/ui/types';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		rows: T[];
		columns: ColumnDef<T>[];
		rowKey: (row: T) => string;
		label: string;
		status: QueryStatus;
		error?: UiError;
		selection?: Selection;
		onopen?: (row: T) => void;
		rowMenu?: (row: T) => MenuItem[];
		rowInline?: (row: T) => MenuItem[];
		empty: Snippet;
		footer?: Snippet;
		class?: string;
	}

	let {
		rows,
		columns,
		rowKey,
		label,
		status,
		error,
		selection = $bindable({ mode: 'none', ids: new Set<string>(), allMatching: false }),
		onopen,
		rowMenu,
		rowInline,
		empty,
		footer,
		class: className,
		...rest
	}: Props = $props();

	const skeletonRows = [0, 1, 2];

	const FALLBACK_CARD_FIELDS = 4;
	const cardColumns = $derived.by(() => {
		const shown = columns.filter((column) => true !== column.hidden);
		const primary = shown.filter((column) => 1 === column.priority);
		if (0 < primary.length) {
			return primary;
		}
		return shown.filter((column) => undefined === column.priority || 2 >= column.priority).slice(0, FALLBACK_CARD_FIELDS);
	});
	const selectable = $derived('none' !== selection.mode);

	const isSelected = (row: T): boolean => selection.ids.has(rowKey(row));

	const setSelected = (row: T, checked: boolean) => {
		const ids = 'single' === selection.mode ? new Set<string>() : new Set(selection.ids);
		if (checked) {
			ids.add(rowKey(row));
		} else {
			ids.delete(rowKey(row));
		}
		selection = { mode: selection.mode, ids, allMatching: false };
	};
</script>

<div class={['table-cards', className]} {...rest}>
	{#if 'error' === status}
		{#if undefined === error}
			<InlineAlert tone="danger" title={t('u091datatable.error_title')} body={t('u091datatable.error_body')} />
		{:else}
			<ErrorState {error} variant="region" />
		{/if}
	{/if}
	{#if 'partial' === status}
		<InlineAlert tone="warn" title={t('u091datatable.partial_title')} body={t('u091datatable.partial_body')} />
	{/if}
	{#if 'loading' === status && 0 === rows.length}
		<div class="loading" aria-busy="true">
			<VisuallyHidden>{t('u091datatable.loading')}</VisuallyHidden>
			{#each skeletonRows as skeletonRow (skeletonRow)}
				<div class="card">
					<Skeleton lines={3} />
				</div>
			{/each}
		</div>
	{:else if 0 === rows.length}
		{#if 'error' !== status}
			{@render empty()}
		{/if}
	{:else}
		<ul class="cards" aria-label={label}>
			{#each rows as row, index (rowKey(row))}
				<li class="card" data-state={selectable && isSelected(row) ? 'selected' : undefined}>
					{#if selectable}
						<CheckboxCell
							checked={isSelected(row)}
							label={t('u091datatable.row_select', { position: index + 1 })}
							onchange={(checked) => setSelected(row, checked)}
						/>
					{/if}
					<dl class="fields">
						{#each cardColumns as column, columnIndex (column.id)}
							<div class={['field', 0 === columnIndex && 'lead']}>
								{#if 0 === columnIndex}
									<dt><VisuallyHidden>{column.header}</VisuallyHidden></dt>
								{:else}
									<dt>{column.header}</dt>
								{/if}
								<dd>{@render column.cell(row)}</dd>
							</div>
						{/each}
					</dl>
					{#if undefined !== onopen || undefined !== rowMenu || undefined !== rowInline}
						<div class="card-actions">
							{#if undefined !== onopen}
								<Button variant="ghost" size="sm" onclick={() => onopen?.(row)}>{t('u091datatable.open')}</Button>
							{/if}
							{#if undefined !== rowMenu || undefined !== rowInline}
								<ActionsCell
									inline={rowInline?.(row) ?? []}
									menu={rowMenu?.(row) ?? []}
									label={t('u091datatable.row_label', { position: index + 1 })}
								/>
							{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
	{@render footer?.()}
</div>

<style>
	.table-cards {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	.loading,
	.cards {
		display: grid;
		gap: var(--space-3);
		margin: 0;
		padding: 0;
		list-style: none;
		grid-template-columns: 1fr;
	}

	.card {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr) auto;
		align-items: center;
		gap: var(--space-3);
		padding: var(--space-3) var(--space-4);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-raised);
		color: var(--text);
	}

	.card[data-state='selected'] {
		border-color: var(--accent-border);
		background: var(--surface-selected);
	}

	.card > .fields:first-child {
		grid-column: 1 / 3;
	}

	.loading .card {
		display: block;
	}

	.fields {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1) var(--space-3);
		min-inline-size: 0;
		margin: 0;
	}

	.field {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: var(--space-1);
		min-inline-size: 0;
		max-inline-size: 100%;
	}

	.field.lead {
		flex: 1 0 100%;
		font-weight: var(--text-control-weight);
	}

	.field.lead dd {
		font-size: var(--text-body-size);
	}

	dt {
		margin: 0;
		color: var(--text-muted);
		font-size: var(--text-caption-size);
		font-weight: var(--text-caption-weight);
	}

	dd {
		margin: 0;
		min-inline-size: 0;
		max-inline-size: 100%;
		overflow-wrap: anywhere;
		font-size: var(--text-sm-size);
	}

	.card-actions {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: var(--space-1);
	}
</style>
