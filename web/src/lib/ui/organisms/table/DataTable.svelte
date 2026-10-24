<script lang="ts" generics="T">
	import type { Snippet } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatNumber } from '$lib/format/number';
	import Button from '../../atoms/Button.svelte';
	import Skeleton from '../../atoms/Skeleton.svelte';
	import ActionsCell from '../../cells/ActionsCell.svelte';
	import CheckboxCell from '../../cells/CheckboxCell.svelte';
	import ErrorState from '../../molecules/ErrorState.svelte';
	import InlineAlert from '../../molecules/InlineAlert.svelte';
	import ResizeHandle from '../../molecules/ResizeHandle.svelte';
	import SortHeader from '../../molecules/SortHeader.svelte';
	import { announce } from '../../quarks/announce';
	import VisuallyHidden from '../../quarks/VisuallyHidden.svelte';
	import { BREAKPOINTS } from '../../quarks/tokens/breakpoints';
	import type { ColumnDef, Density, MenuItem, QueryStatus, Selection, SortState, UiError } from '../../types';
	import TableCardView from './TableCardView.svelte';
	import TableToolbar from './TableToolbar.svelte';
	import VirtualRows from './VirtualRows.svelte';
	import {
		DEFAULT_COLUMN_WIDTH_PX,
		MAX_COLUMN_WIDTH_PX,
		MIN_COLUMN_WIDTH_PX,
		TableCursor,
		VIRTUAL_THRESHOLD,
		clearSelection,
		clampIndex,
		deselectPage,
		destinationFor,
		emptySelection,
		isCoarsePointer,
		isPageSelected,
		orderedVisibleColumns,
		rowHeightPx,
		selectAllMatching,
		selectPage,
		selectRange,
		toggleRow
	} from './table-state.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'class' | 'children'> {
		rows: T[];
		columns: ColumnDef<T>[];
		rowKey: (row: T) => string;
		rowLabel?: (row: T) => string;
		label: string;
		status: QueryStatus;
		error?: UiError;
		sort?: SortState | null;
		selection?: Selection;
		density?: Density;
		virtual?: 'auto' | boolean;
		total?: number | null;
		totalIsEstimate?: boolean;
		hasMore?: boolean;
		onloadmore?: () => void;
		onopen?: (row: T) => void;
		onretry?: () => void;
		onexport?: (format: 'csv' | 'json') => void;
		rowMenu?: (row: T) => MenuItem[];
		rowInline?: (row: T) => MenuItem[];
		empty: Snippet;
		toolbar?: Snippet;
		search?: Snippet;
		filters?: Snippet;
		footer?: Snippet;
		stickyHeader?: boolean;
		freezeFirst?: boolean;
		resizable?: boolean;
		controls?: boolean;
		caption?: string;
		class?: string;
	}

	let {
		rows,
		columns,
		rowKey,
		rowLabel,
		label,
		status,
		error,
		sort = $bindable(null),
		selection = $bindable(emptySelection()),
		density = $bindable('default'),
		virtual = 'auto',
		total = null,
		totalIsEstimate = false,
		hasMore = false,
		onloadmore,
		onopen,
		onretry,
		onexport,
		rowMenu,
		rowInline,
		empty,
		toolbar,
		search,
		filters,
		footer,
		stickyHeader = true,
		freezeFirst = true,
		resizable = true,
		controls = true,
		caption,
		class: className,
		...rest
	}: Props = $props();

	const SKELETON_ROWS = [0, 1, 2, 3, 4, 5];

	const cursor = new TableCursor();
	let frameEl: HTMLDivElement | null = $state(null);
	let tableEl: HTMLTableElement | null = $state(null);
	let visible: string[] = $state([]);
	let order: string[] = $state([]);
	let widthOverrides: Record<string, number> = $state({});
	let narrow = $state(false);
	let layoutSignature = '';

	$effect.pre(() => {
		const ids = columns.map((column) => column.id);
		const signature = ids.join('|');
		if (layoutSignature === signature) {
			return;
		}
		layoutSignature = signature;
		order = ids;
		visible = columns.filter((column) => !column.hidden).map((column) => column.id);
	});

	$effect(() => {
		if ('undefined' === typeof window || 'function' !== typeof window.matchMedia) {
			return;
		}
		const query = window.matchMedia(`(max-width: ${BREAKPOINTS.sm - 1}px)`);
		const sync = () => {
			narrow = query.matches;
		};
		sync();
		query.addEventListener('change', sync);
		return () => {
			query.removeEventListener('change', sync);
		};
	});

	const keys: string[] = $derived(rows.map((row) => rowKey(row)));
	const selectable = $derived('none' !== selection.mode);
	const visibleColumns = $derived(orderedVisibleColumns(columns, order, visible));
	const hasRowActions = $derived(undefined !== rowMenu || undefined !== rowInline);
	const columnCount = $derived(visibleColumns.length + (selectable ? 1 : 0) + (hasRowActions ? 1 : 0));
	const useVirtual = $derived('auto' === virtual ? VIRTUAL_THRESHOLD < rows.length : virtual);
	const rowHeight = $derived(rowHeightPx(density, isCoarsePointer()));
	const pageSelected = $derived(isPageSelected(selection, keys));
	const selectAllOffer = $derived(
		!selection.allMatching && pageSelected && null !== total && undefined !== total && rows.length < total
	);
	const activeRow = $derived(clampIndex(cursor.active, rows.length));
	const showSkeleton = $derived(0 === rows.length && 'loading' === status);
	const showError = $derived(0 === rows.length && 'error' === status);
	const showEmpty = $derived(0 === rows.length && 'loading' !== status && 'error' !== status);
	const errorView: UiError = $derived(
		error ?? { title: t('datatable.errorTitle'), body: t('datatable.errorBody'), retryable: true }
	);

	const footerText = $derived.by(() => {
		if (0 === rows.length) {
			return '';
		}
		if (hasMore) {
			return t('datatable.shownMore', { shown: formatNumber(rows.length) });
		}
		if (null === total || undefined === total) {
			return t('datatable.shown', { count: formatNumber(rows.length) });
		}
		const range = { from: formatNumber(1), to: formatNumber(rows.length), total: formatNumber(total) };
		return totalIsEstimate ? t('datatable.range.estimate', range) : t('tablecontrols.pagination.range', range);
	});

	const selectionText = (next: Selection): string =>
		next.allMatching
			? t('datatable.allMatching', { count: formatNumber(total ?? rows.length) })
			: t('datatable.selectedCount', { count: next.ids.size });

	const columnLabel = (key: string): string => columns.find((column) => column.sortKey === key)?.header ?? key;

	const ariaSort = (column: ColumnDef<T>): 'ascending' | 'descending' | 'none' | undefined => {
		if (undefined === column.sortKey) {
			return undefined;
		}
		if (null === sort || sort.key !== column.sortKey) {
			return 'none';
		}
		return 'asc' === sort.dir ? 'ascending' : 'descending';
	};

	const widthOf = (column: ColumnDef<T>): number =>
		widthOverrides[column.id] ?? column.width ?? DEFAULT_COLUMN_WIDTH_PX;

	const setWidth = (id: string, next: number) => {
		widthOverrides[id] = next;
	};

	const changeSort = (next: SortState | null) => {
		const previous = sort;
		sort = next;
		if (null === next) {
			announce(t('tablecontrols.sort.cleared', { label: columnLabel(previous?.key ?? '') }), 'polite');
			return;
		}
		announce(
			t('tablecontrols.sort.announce', {
				label: columnLabel(next.key),
				direction:
					'asc' === next.dir ? t('tablecontrols.sort.ascending') : t('tablecontrols.sort.descending')
			}),
			'polite'
		);
	};

	const applySelection = (next: Selection) => {
		selection = next;
		announce(selectionText(next), 'polite');
	};

	const togglePage = () => {
		applySelection(pageSelected ? deselectPage(selection, keys) : selectPage(selection, keys));
	};

	const clearAll = () => {
		applySelection(clearSelection(selection));
	};

	const selectMatching = () => {
		applySelection(selectAllMatching(selection));
	};

	const revealActiveRow = () => {
		if (null === frameEl) {
			return;
		}
		const top = cursor.active * rowHeight;
		const bottom = top + rowHeight;
		if (top < frameEl.scrollTop) {
			frameEl.scrollTop = top;
		} else if (bottom > frameEl.scrollTop + frameEl.clientHeight) {
			frameEl.scrollTop = bottom - frameEl.clientHeight;
		}
	};

	const focusActiveRow = () => {
		requestAnimationFrame(() => {
			tableEl?.querySelector<HTMLTableRowElement>(`tr[data-row-index="${cursor.active}"]`)?.focus();
		});
	};

	const moveCursor = (destination: number, extend: boolean) => {
		if (extend && selectable) {
			const anchor = null === cursor.anchor ? cursor.active : cursor.anchor;
			applySelection(selectRange(selection, keys, anchor, destination));
		} else {
			cursor.anchor = destination;
		}
		cursor.active = destination;
		revealActiveRow();
		focusActiveRow();
	};

	const openRowMenu = (index: number) => {
		const triggers = tableEl?.querySelectorAll<HTMLButtonElement>(
			`tr[data-row-index="${index}"] .actions-cell button`
		);
		const trigger = triggers?.[triggers.length - 1];
		trigger?.dispatchEvent(new KeyboardEvent('keydown', { key: 'F10', shiftKey: true, bubbles: true, cancelable: true }));
	};

	const handleRowKeydown = (event: KeyboardEvent) => {
		if (!(event.target instanceof HTMLTableRowElement)) {
			return;
		}
		const index = Number(event.target.dataset.rowIndex);
		const row = rows[index];
		const key = keys[index];
		if ((event.metaKey || event.ctrlKey) && 'a' === event.key.toLowerCase()) {
			event.preventDefault();
			if (selectable) {
				applySelection(selectPage(selection, keys));
			}
			return;
		}
		if ('Escape' === event.key) {
			if (selectable) {
				applySelection(clearSelection(selection));
			}
			return;
		}
		if (('F10' === event.key && event.shiftKey) || 'ContextMenu' === event.key) {
			event.preventDefault();
			openRowMenu(index);
			return;
		}
		if ('Enter' === event.key || 'o' === event.key) {
			if (undefined !== row) {
				event.preventDefault();
				onopen?.(row);
			}
			return;
		}
		if ((' ' === event.key || 'x' === event.key) && selectable && undefined !== key) {
			event.preventDefault();
			applySelection(toggleRow(selection, key));
			return;
		}
		const destination = destinationFor(event.key, index, rows.length);
		if (null !== destination) {
			event.preventDefault();
			moveCursor(destination, event.shiftKey && ('ArrowDown' === event.key || 'ArrowUp' === event.key));
		}
	};

	const handleRowClick = (event: MouseEvent, index: number) => {
		if (event.target instanceof Element && null !== event.target.closest('button, input, a, label')) {
			return;
		}
		if (event.shiftKey && selectable) {
			const anchor = null === cursor.anchor ? cursor.active : cursor.anchor;
			applySelection(selectRange(selection, keys, anchor, index));
		} else {
			cursor.anchor = index;
		}
		cursor.active = index;
	};
</script>

{#snippet dataRow(row: T, index: number, key: string)}
	{@const isSelected = selection.ids.has(key)}
	<tr
		class="row"
		data-row-index={index}
		tabindex={index === activeRow ? 0 : -1}
		aria-selected={selectable ? isSelected : undefined}
		data-state={isSelected ? 'selected' : undefined}
		onclick={(event) => handleRowClick(event, index)}
		onfocus={() => {
			cursor.active = index;
		}}
	>
		{#if selectable}
			<td class="select-cell" class:frozen={freezeFirst}>
				<CheckboxCell
					checked={isSelected}
					label={t('datatable.selectRow', { name: rowLabel?.(row) ?? key })}
					onchange={() => applySelection(toggleRow(selection, key))}
				/>
			</td>
		{/if}
		{#each visibleColumns as column, columnIndex (column.id)}
			<td data-align={column.align ?? 'start'} class:frozen={freezeFirst && !selectable && 0 === columnIndex}>
				{@render column.cell(row)}
			</td>
		{/each}
		{#if hasRowActions}
			<td class="actions-wrap">
				<ActionsCell inline={rowInline?.(row) ?? []} menu={rowMenu?.(row) ?? []} label={t('datatable.actions')} />
			</td>
		{/if}
	</tr>
{/snippet}

{#snippet virtualRow(index: number)}
	{@const row = rows[index]}
	{#if undefined !== row}
		{@render dataRow(row, index, rowKey(row))}
	{/if}
{/snippet}

<div class={['data-table', className]} data-density={density} data-state={status} {...rest}>
	{#if (controls && !narrow) || undefined !== onexport || undefined !== toolbar || undefined !== search || undefined !== filters}
		<TableToolbar
			columns={columns as ColumnDef<unknown>[]}
			bind:visible
			bind:order
			bind:density
			{onexport}
			{search}
			{filters}
			extra={toolbar}
			controls={controls && !narrow}
		/>
	{/if}

	{#if selectable && (0 < selection.ids.size || selection.allMatching)}
		<div class="selection-bar">
			<span class="selection-text">{selectionText(selection)}</span>
			{#if selectAllOffer}
				<Button variant="ghost" size="sm" onclick={selectMatching}>
					{t('datatable.selectAllMatching', { count: formatNumber(total) })}
				</Button>
			{/if}
			<Button variant="ghost" size="sm" onclick={clearAll}>{t('datatable.clearSelection')}</Button>
		</div>
	{/if}

	{#if false === narrow && 'error' === status && 0 < rows.length}
		<InlineAlert tone="danger" title={errorView.title} body={errorView.body} />
	{/if}
	{#if false === narrow && 'partial' === status}
		<InlineAlert tone="warn" title={t('datatable.partial')} body={t('datatable.partialBody')} />
	{/if}

	{#if narrow}
		<TableCardView
			{rows}
			columns={visibleColumns}
			{rowKey}
			{label}
			{status}
			error={errorView}
			bind:selection
			{rowMenu}
			{onopen}
			{empty}
		/>
	{:else}
		<div class="frame" bind:this={frameEl}>
			<table
				class="table"
				class:sticky-header={stickyHeader}
				aria-label={label}
				aria-busy={'loading' === status}
				bind:this={tableEl}
				onkeydown={handleRowKeydown}
			>
				{#if caption}
					<caption><VisuallyHidden>{caption}</VisuallyHidden></caption>
				{/if}
				<colgroup>
					{#if selectable}<col class="select-col" />{/if}
					{#each visibleColumns as column (column.id)}
						<col style:width={`${widthOf(column)}px`} />
					{/each}
					{#if hasRowActions}<col class="actions-col" />{/if}
				</colgroup>
				<thead>
					<tr>
						{#if selectable}
							<th scope="col" class="select-cell" class:frozen={freezeFirst}>
								<CheckboxCell checked={pageSelected} label={t('datatable.selectPage')} onchange={togglePage} />
							</th>
						{/if}
						{#each visibleColumns as column, columnIndex (column.id)}
							<th
								scope="col"
								data-align={column.align ?? 'start'}
								aria-sort={ariaSort(column)}
								class:frozen={freezeFirst && !selectable && 0 === columnIndex}
							>
								{#if undefined !== column.sortKey}
									<SortHeader
										as="span"
										label={column.header}
										sortKey={column.sortKey}
										sort={sort}
										onsort={changeSort}
										help={column.headerHelp}
									/>
								{:else}
									<span class="header-text">{column.header}</span>
								{/if}
								{#if resizable}
									<ResizeHandle
										orientation="vertical"
										bind:value={() => widthOf(column), (next: number) => setWidth(column.id, next)}
										min={column.minWidth ?? MIN_COLUMN_WIDTH_PX}
										max={MAX_COLUMN_WIDTH_PX}
										label={t('datatable.resizeColumn', { column: column.header })}
									/>
								{/if}
							</th>
						{/each}
						{#if hasRowActions}
							<th scope="col" class="actions-head"><VisuallyHidden>{t('datatable.actions')}</VisuallyHidden></th>
						{/if}
					</tr>
				</thead>
				<tbody>
					{#if showSkeleton}
						{#each SKELETON_ROWS as skeletonRow (skeletonRow)}
							<tr class="skeleton-row" aria-hidden="true">
								<td colspan={columnCount}><Skeleton shape="text" /></td>
							</tr>
						{/each}
					{:else if showEmpty}
						<tr>
							<td colspan={columnCount} class="state-cell">{@render empty()}</td>
						</tr>
					{:else if showError}
						<tr>
							<td colspan={columnCount} class="state-cell">
								<ErrorState error={errorView} onretry={onretry} variant="region" />
							</td>
						</tr>
					{:else if useVirtual}
						<VirtualRows count={rows.length} {rowHeight} scrollEl={frameEl} render={virtualRow} />
					{:else}
						{#each rows as row, index (rowKey(row))}
							{@render dataRow(row, index, rowKey(row))}
						{/each}
					{/if}
				</tbody>
			</table>
		</div>
	{/if}

	<div class="footer">
		{#if 0 < rows.length}<span>{footerText}</span>{/if}
		{#if hasMore}
			<Button variant="secondary" size="sm" onclick={() => onloadmore?.()}>{t('datatable.loadMore')}</Button>
		{/if}
		{#if footer}{@render footer()}{/if}
	</div>
</div>

<style>
	.data-table {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		min-inline-size: 0;
		max-inline-size: 100%;
		color: var(--text);
	}

	.selection-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
		padding-inline: var(--table-edge-px);
		font-size: var(--text-sm-size);
	}

	.frame {
		contain: inline-size;
		overflow: auto;
		max-block-size: var(--table-max-h, calc(var(--viewport-h) * 0.7));
		border: var(--hairline) solid var(--table-border, var(--border-subtle));
		border-radius: var(--table-radius, var(--radius-lg));
		background: var(--table-bg, var(--surface));
	}

	.table {
		inline-size: max-content;
		min-inline-size: 100%;
		border-collapse: separate;
		border-spacing: 0;
		font-size: var(--table-text);
	}

	.table th,
	.table td {
		padding-inline: var(--cell-px);
		vertical-align: middle;
		text-align: start;
	}

	.table th:first-child,
	.table td:first-child {
		padding-inline-start: var(--table-edge-px);
	}

	.table th:last-child,
	.table td:last-child {
		padding-inline-end: var(--table-edge-px);
	}

	.table th {
		position: relative;
		block-size: var(--control-h);
		padding-block: var(--space-2);
		border-block-end: var(--hairline) solid var(--border-subtle);
		background: var(--table-header-bg, var(--surface-raised));
		color: var(--text-muted);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		font-weight: var(--text-control-weight);
		white-space: nowrap;
	}

	.table th :global(.resize-handle) {
		--resize-handle-color: transparent;
		position: absolute;
		inset-block: 0;
		inset-inline-end: calc(var(--target-min) / -2);
		z-index: var(--z-raised);
		block-size: auto;
	}

	@media (hover: hover) {
		.table th:hover :global(.resize-handle) {
			--resize-handle-color: var(--border-strong);
		}
	}

	.table th:last-child :global(.resize-handle) {
		inset-inline-end: 0;
	}

	.table td {
		border-block-end: var(--hairline) solid var(--border-subtle);
	}

	.table tbody tr:last-child td {
		border-block-end: 0;
	}

	.sticky-header thead th {
		position: sticky;
		inset-block-start: 0;
		z-index: var(--z-sticky);
	}

	.table [data-align='end'] {
		text-align: end;
		font-variant-numeric: tabular-nums;
	}

	.header-text {
		display: inline-block;
	}

	.row {
		block-size: var(--table-row-h, var(--row-h));
	}

	.row td {
		background: var(--table-row-bg, var(--surface));
	}

	@media (hover: hover) {
		.row:hover td {
			background: var(--table-row-hover, var(--surface-hover));
		}
	}

	.row[data-state='selected'] td {
		background: var(--table-row-selected, var(--accent-soft));
	}

	.row:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: inset 0 0 0 var(--focus-width) var(--focus-ring-color);
	}

	.frozen {
		position: sticky;
		inset-inline-start: 0;
		z-index: var(--z-raised);
	}

	.skeleton-row td {
		block-size: var(--table-row-h, var(--row-h));
	}

	.state-cell {
		padding-block: var(--space-6);
		text-align: center;
	}

	.footer {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
		padding-inline: var(--table-edge-px);
		color: var(--text-muted);
		font-size: var(--text-sm-size);
	}
</style>
