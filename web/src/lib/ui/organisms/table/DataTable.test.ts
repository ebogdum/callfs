import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { ColumnDef, MenuItem, Selection } from '../../types';
import DataTable from './DataTable.svelte';

afterEach(() => {
	cleanup();
});

interface Row {
	id: string;
	name: string;
	size: number;
}

const nameCell = createRawSnippet<[Row]>((row) => ({ render: () => `<span>${row().name}</span>` }));
const sizeCell = createRawSnippet<[Row]>((row) => ({ render: () => `<span>${row().size}</span>` }));
const emptyState = createRawSnippet(() => ({ render: () => '<p>Nothing here</p>' }));

const columns = [
	{ id: 'name', header: 'Name', sortKey: 'name', cell: nameCell },
	{ id: 'size', header: 'Size', sortKey: 'size', align: 'end', cell: sizeCell }
] as ColumnDef<unknown>[];

const makeRows = (count: number): Row[] =>
	Array.from({ length: count }, (_, index) => ({ id: `id-${index}`, name: `file-${index}.txt`, size: index }));

const multiSelection = (): Selection => ({ mode: 'multi', ids: new Set<string>(), allMatching: false });

const bodyRow = (index: number): HTMLTableRowElement =>
	document.querySelector(`tbody tr[data-row-index="${index}"]`) as HTMLTableRowElement;

const renderedRowCount = (): number => document.querySelectorAll('tbody tr[data-row-index]').length;

const isSelected = (index: number): boolean => 'true' === bodyRow(index).getAttribute('aria-selected');

const baseProps = {
	rows: makeRows(10),
	columns,
	rowKey: (row: unknown) => (row as Row).id,
	label: 'Files',
	status: 'success' as const,
	empty: emptyState
};

describe('DataTable structure', () => {
	it('names the table and renders one row per item', () => {
		render(DataTable, { props: baseProps });
		expect(screen.queryByRole('table', { name: 'Files' })).not.toBeNull();
		expect(renderedRowCount()).toBe(10);
	});

	it('keeps exactly one row in the tab order', () => {
		render(DataTable, { props: baseProps });
		expect(bodyRow(0).getAttribute('tabindex')).toBe('0');
		expect(bodyRow(1).getAttribute('tabindex')).toBe('-1');
	});

	it('omits selection attributes when selection mode is none', () => {
		render(DataTable, { props: baseProps });
		expect(bodyRow(0).hasAttribute('aria-selected')).toBe(false);
	});

	it('shows the bounded footer range', () => {
		render(DataTable, { props: { ...baseProps, rows: makeRows(10), total: 1234 } });
		expect(
			screen.queryByText(t('tablecontrols.pagination.range', { from: '1', to: '10', total: '1,234' }))
		).not.toBeNull();
	});

	it('renders the caller empty state inside the table when there are no rows', () => {
		render(DataTable, { props: { ...baseProps, rows: [] } });
		expect(screen.queryByText('Nothing here')).not.toBeNull();
	});

	it('renders skeleton rows while loading with no rows', () => {
		render(DataTable, { props: { ...baseProps, rows: [], status: 'loading' } });
		expect(document.querySelectorAll('tr.skeleton-row')).toHaveLength(6);
		expect(screen.queryByText('Nothing here')).toBeNull();
	});

	it('renders the error state with a retry action when there are no rows', async () => {
		const onretry = vi.fn();
		render(DataTable, {
			props: {
				...baseProps,
				rows: [],
				status: 'error',
				error: { title: 'Could not load files', body: 'Try again soon.', retryable: true },
				onretry
			}
		});
		expect(screen.queryByText('Could not load files')).not.toBeNull();
		await fireEvent.click(screen.getByRole('button', { name: t('common.retry') }));
		expect(onretry).toHaveBeenCalledTimes(1);
	});

	it('keeps the rows and shows an alert when a refresh fails', () => {
		render(DataTable, {
			props: {
				...baseProps,
				status: 'error',
				error: { title: 'Could not refresh files', body: 'Try again soon.', retryable: true }
			}
		});
		expect(renderedRowCount()).toBe(10);
		expect(screen.queryByText('Could not refresh files')).not.toBeNull();
	});

	it('keeps the loaded rows and shows a partial alert', () => {
		render(DataTable, { props: { ...baseProps, status: 'partial' } });
		expect(screen.queryByText(t('datatable.partial'))).not.toBeNull();
		expect(renderedRowCount()).toBe(10);
	});
});

describe('DataTable keyboard', () => {
	it('moves the active row with j and k and focuses it', async () => {
		render(DataTable, { props: baseProps });
		await fireEvent.keyDown(bodyRow(0), { key: 'j' });
		await waitFor(() => expect(document.activeElement).toBe(bodyRow(1)));
		await fireEvent.keyDown(bodyRow(1), { key: 'k' });
		await waitFor(() => expect(document.activeElement).toBe(bodyRow(0)));
	});

	it('moves to the first and last row with End and Home', async () => {
		render(DataTable, { props: baseProps });
		await fireEvent.keyDown(bodyRow(0), { key: 'End' });
		await waitFor(() => expect(document.activeElement).toBe(bodyRow(9)));
		await fireEvent.keyDown(bodyRow(9), { key: 'Home' });
		await waitFor(() => expect(document.activeElement).toBe(bodyRow(0)));
	});

	it('toggles the active row with x and Space', async () => {
		render(DataTable, { props: { ...baseProps, selection: multiSelection() } });
		await fireEvent.keyDown(bodyRow(0), { key: 'x' });
		expect(isSelected(0)).toBe(true);
		await fireEvent.keyDown(bodyRow(0), { key: ' ' });
		expect(isSelected(0)).toBe(false);
	});

	it('extends the selection with Shift+ArrowDown', async () => {
		render(DataTable, { props: { ...baseProps, selection: multiSelection() } });
		await fireEvent.keyDown(bodyRow(0), { key: 'ArrowDown', shiftKey: true });
		await waitFor(() => expect(document.activeElement).toBe(bodyRow(1)));
		expect(isSelected(0) && isSelected(1)).toBe(true);
		expect(isSelected(2)).toBe(false);
		await fireEvent.keyDown(bodyRow(1), { key: 'ArrowDown', shiftKey: true });
		expect(isSelected(0) && isSelected(1) && isSelected(2)).toBe(true);
		expect(isSelected(3)).toBe(false);
	});

	it('selects a range with Shift+click from the last clicked row', async () => {
		render(DataTable, { props: { ...baseProps, selection: multiSelection() } });
		await fireEvent.click(bodyRow(1));
		await fireEvent.click(bodyRow(3), { shiftKey: true });
		expect(isSelected(0)).toBe(false);
		expect(isSelected(1) && isSelected(2) && isSelected(3)).toBe(true);
		expect(isSelected(4)).toBe(false);
	});

	it('selects the page with Ctrl+A and offers select all matching when the total is larger', async () => {
		render(DataTable, { props: { ...baseProps, selection: multiSelection(), total: 120 } });
		await fireEvent.keyDown(bodyRow(0), { key: 'a', ctrlKey: true });
		expect(renderedRowCount()).toBe(10);
		expect(isSelected(0) && isSelected(9)).toBe(true);
		const offer = screen.getByRole('button', { name: t('datatable.selectAllMatching', { count: '120' }) });
		await fireEvent.click(offer);
		expect(screen.queryByText(t('datatable.allMatching', { count: '120' }))).not.toBeNull();
		expect(screen.queryByRole('button', { name: t('datatable.selectAllMatching', { count: '120' }) })).toBeNull();
	});

	it('does not offer select all matching when the total equals the page', async () => {
		render(DataTable, { props: { ...baseProps, selection: multiSelection(), total: 10 } });
		await fireEvent.keyDown(bodyRow(0), { key: 'a', metaKey: true });
		expect(isSelected(9)).toBe(true);
		expect(screen.queryByRole('button', { name: /matching/ })).toBeNull();
	});

	it('clears the selection, including all matching, with Escape', async () => {
		render(DataTable, { props: { ...baseProps, selection: multiSelection(), total: 120 } });
		await fireEvent.keyDown(bodyRow(0), { key: 'a', ctrlKey: true });
		await fireEvent.click(screen.getByRole('button', { name: t('datatable.selectAllMatching', { count: '120' }) }));
		await fireEvent.keyDown(bodyRow(0), { key: 'Escape' });
		expect(isSelected(0)).toBe(false);
		expect(isSelected(9)).toBe(false);
		expect(screen.queryByRole('button', { name: t('datatable.clearSelection') })).toBeNull();
	});

	it('opens the active row with Enter and o', async () => {
		const onopen = vi.fn();
		render(DataTable, { props: { ...baseProps, onopen } });
		await fireEvent.keyDown(bodyRow(0), { key: 'Enter' });
		expect(onopen).toHaveBeenLastCalledWith(makeRows(10)[0]);
		await fireEvent.keyDown(bodyRow(0), { key: 'o' });
		expect(onopen).toHaveBeenCalledTimes(2);
	});

	it('opens the row menu with Shift+F10', async () => {
		const rename = vi.fn();
		const rowMenu = (): MenuItem[] => [{ id: 'rename', label: 'Rename', onselect: rename }];
		render(DataTable, { props: { ...baseProps, rowMenu } });
		await fireEvent.keyDown(bodyRow(0), { key: 'F10', shiftKey: true });
		await waitFor(() => expect(screen.queryByRole('menu')).not.toBeNull());
	});
});

describe('DataTable virtualization', () => {
	it('renders at most 60 rows for 10,000 items', () => {
		render(DataTable, { props: { ...baseProps, rows: makeRows(10000) } });
		expect(renderedRowCount()).toBeGreaterThan(0);
		expect(renderedRowCount()).toBeLessThanOrEqual(60);
	});

	it('renders every row when virtualization is turned off', () => {
		render(DataTable, { props: { ...baseProps, rows: makeRows(300), virtual: false } });
		expect(renderedRowCount()).toBe(300);
	});
});

describe('DataTable sort', () => {
	it('cycles the sort from ascending to descending to none', async () => {
		render(DataTable, { props: baseProps });
		const nameHeader = document.querySelectorAll('thead th')[0] as HTMLTableCellElement;
		const sortButton = nameHeader.querySelector('button') as HTMLButtonElement;
		expect(nameHeader.getAttribute('aria-sort')).toBe('none');
		await fireEvent.click(sortButton);
		expect(nameHeader.getAttribute('aria-sort')).toBe('ascending');
		await fireEvent.click(sortButton);
		expect(nameHeader.getAttribute('aria-sort')).toBe('descending');
		await fireEvent.click(sortButton);
		expect(nameHeader.getAttribute('aria-sort')).toBe('none');
	});
});
