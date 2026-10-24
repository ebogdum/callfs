import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { ColumnDef, MenuItem, UiError } from '$lib/ui/types';
import BulkBar from '../BulkBar.svelte';
import { download, toCSV, toJSON } from './export';
import TableCardView from './TableCardView.svelte';

interface Row {
	id: string;
	name: string;
	size: number;
	kind: string;
}

const textCell = (pick: (row: Row) => string) =>
	createRawSnippet<[unknown]>((getRow) => ({ render: () => `<span>${pick(getRow() as Row)}</span>` }));

const emptySnippet = createRawSnippet(() => ({ render: () => '<p>Nothing here</p>' }));

const rows: Row[] = [
	{ id: 'a', name: 'report.pdf', size: 9000, kind: 'file' },
	{ id: 'b', name: 'photos', size: 1, kind: 'folder' }
];

const cardColumns: ColumnDef<unknown>[] = [
	{ id: 'name', header: 'Name', cell: textCell((row) => row.name), priority: 1 },
	{ id: 'size', header: 'Size', cell: textCell((row) => String(row.size)), priority: 2 },
	{ id: 'kind', header: 'Kind', cell: textCell((row) => row.kind), priority: 1, hidden: true }
];

const exportColumns: ColumnDef<Row>[] = [
	{ id: 'name', header: 'Name', cell: textCell((row) => row.name), csv: (row) => row.name },
	{ id: 'size', header: 'Size', cell: textCell((row) => String(row.size)), csv: (row) => String(row.size) },
	{ id: 'kind', header: 'Kind', cell: textCell((row) => row.kind) }
];

const cardProps = (overrides: Record<string, unknown> = {}) => ({
	rows,
	columns: cardColumns,
	rowKey: (row: unknown) => (row as Row).id,
	label: 'Files',
	status: 'success' as const,
	empty: emptySnippet,
	...overrides
});

const action = (id: string, label: string, extra: Partial<MenuItem> = {}): MenuItem => ({
	id,
	label,
	onselect: vi.fn(),
	...extra
});

afterEach(() => {
	cleanup();
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe('TableCardView', () => {
	it('shows priority one columns as label and value pairs and hides the rest', () => {
		render(TableCardView, { props: cardProps() });
		expect(screen.getAllByText('Name')).toHaveLength(rows.length);
		expect(screen.getByText('report.pdf')).toBeTruthy();
		expect(screen.queryByText('Size')).toBeNull();
		expect(screen.queryByText('Kind')).toBeNull();
	});

	it('renders the list with its accessible name', () => {
		render(TableCardView, { props: cardProps() });
		expect(screen.getByRole('list', { name: 'Files' })).toBeTruthy();
		expect(screen.getAllByRole('listitem')).toHaveLength(2);
	});

	it('shows a busy skeleton with a plain loading label when there are no rows yet', () => {
		const { container } = render(TableCardView, { props: cardProps({ rows: [], status: 'loading' }) });
		expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
		expect(screen.getByText(t('u091datatable.loading'))).toBeTruthy();
		expect(screen.queryByText('Nothing here')).toBeNull();
	});

	it('keeps stale rows on screen while loading', () => {
		const { container } = render(TableCardView, { props: cardProps({ status: 'loading' }) });
		expect(container.querySelector('[aria-busy="true"]')).toBeNull();
		expect(screen.getAllByRole('listitem')).toHaveLength(2);
	});

	it('renders the empty snippet when there are no rows', () => {
		render(TableCardView, { props: cardProps({ rows: [], status: 'empty' }) });
		expect(screen.getByText('Nothing here')).toBeTruthy();
	});

	it('shows the error state from the error object and hides the empty snippet', () => {
		const error: UiError = {
			title: 'We cannot reach callfs.',
			body: 'Check your connection.',
			retryable: true
		};
		render(TableCardView, { props: cardProps({ rows: [], status: 'error', error }) });
		expect(screen.getByText('We cannot reach callfs.')).toBeTruthy();
		expect(screen.queryByText('Nothing here')).toBeNull();
	});

	it('shows the plain fallback alert when the error status has no error object', () => {
		render(TableCardView, { props: cardProps({ status: 'error' }) });
		expect(screen.getByText(t('u091datatable.error_title'))).toBeTruthy();
		expect(screen.getAllByRole('listitem')).toHaveLength(2);
	});

	it('shows the partial alert above the rows', () => {
		render(TableCardView, { props: cardProps({ status: 'partial' }) });
		expect(screen.getByText(t('u091datatable.partial_title'))).toBeTruthy();
		expect(screen.getByText(t('u091datatable.partial_body'))).toBeTruthy();
	});

	it('renders no checkboxes when selection mode is none', () => {
		render(TableCardView, {
			props: cardProps({ selection: { mode: 'none', ids: new Set<string>(), allMatching: false } })
		});
		expect(screen.queryByRole('checkbox')).toBeNull();
	});

	it('selects a card through its named checkbox in multi mode', async () => {
		render(TableCardView, {
			props: cardProps({ selection: { mode: 'multi', ids: new Set<string>(), allMatching: false } })
		});
		const first = screen.getByRole('checkbox', { name: t('u091datatable.row_select', { position: 1 }) });
		await fireEvent.click(first);
		expect(first.getAttribute('aria-checked')).toBe('true');
		const second = screen.getByRole('checkbox', { name: t('u091datatable.row_select', { position: 2 }) });
		await fireEvent.click(second);
		expect(first.getAttribute('aria-checked')).toBe('true');
		expect(second.getAttribute('aria-checked')).toBe('true');
	});

	it('keeps only the latest card selected in single mode', async () => {
		render(TableCardView, {
			props: cardProps({ selection: { mode: 'single', ids: new Set<string>(), allMatching: false } })
		});
		const first = screen.getByRole('checkbox', { name: t('u091datatable.row_select', { position: 1 }) });
		const second = screen.getByRole('checkbox', { name: t('u091datatable.row_select', { position: 2 }) });
		await fireEvent.click(first);
		await fireEvent.click(second);
		expect(first.getAttribute('aria-checked')).toBe('false');
		expect(second.getAttribute('aria-checked')).toBe('true');
	});

	it('calls onopen with the row whose Open button is pressed', async () => {
		const onopen = vi.fn();
		render(TableCardView, { props: cardProps({ onopen }) });
		await fireEvent.click(screen.getAllByRole('button', { name: t('u091datatable.open') })[1]);
		expect(onopen).toHaveBeenCalledTimes(1);
		expect(onopen).toHaveBeenCalledWith(rows[1]);
	});

	it('opens the row menu from its named kebab and runs the chosen item', async () => {
		const rename = action('rename', 'Rename');
		render(TableCardView, { props: cardProps({ rowMenu: () => [rename] }) });
		await fireEvent.keyDown(screen.getByRole('button', { name: /row 1/ }), { key: 'Enter' });
		await fireEvent.keyDown(screen.getByRole('menuitem', { name: /Rename/ }), { key: 'Enter' });
		expect(rename.onselect).toHaveBeenCalledTimes(1);
	});

	it('renders inline row actions as labelled buttons', async () => {
		const share = action('share', 'Share', { icon: 'share' });
		render(TableCardView, { props: cardProps({ rowInline: () => [share] }) });
		await fireEvent.click(screen.getAllByRole('button', { name: /Share/ })[0]);
		expect(share.onselect).toHaveBeenCalledTimes(1);
	});
});

describe('toCSV', () => {
	it('writes a header and one CRLF-separated record per row, using only columns with csv', () => {
		expect(toCSV(rows, exportColumns)).toBe('Name,Size\r\nreport.pdf,9000\r\nphotos,1');
	});

	it('quotes fields with commas, quotes or line breaks and doubles inner quotes', () => {
		const tricky: Row[] = [{ id: 'x', name: 'a,"b"\nc', size: 2, kind: 'file' }];
		expect(toCSV(tricky, exportColumns)).toBe('Name,Size\r\n"a,""b""\nc",2');
	});

	it('writes only the header when there are no rows', () => {
		expect(toCSV([], exportColumns)).toBe('Name,Size');
	});
});

describe('toJSON', () => {
	it('writes an array of records keyed by column id using csv values', () => {
		expect(JSON.parse(toJSON(rows, exportColumns))).toEqual([
			{ name: 'report.pdf', size: '9000' },
			{ name: 'photos', size: '1' }
		]);
	});
});

describe('download', () => {
	const clicked: string[] = [];

	beforeEach(() => {
		clicked.length = 0;
		Object.defineProperty(URL, 'createObjectURL', {
			configurable: true,
			writable: true,
			value: vi.fn(() => 'blob:callfs')
		});
		Object.defineProperty(URL, 'revokeObjectURL', {
			configurable: true,
			writable: true,
			value: vi.fn()
		});
		vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
			clicked.push(this.download);
		});
	});

	it('clicks a link named for the file and revokes the blob URL after the download starts', () => {
		vi.useFakeTimers();
		const result = download('rows.csv', new Blob(['Name']));
		expect(result).toEqual({ ok: true, value: undefined });
		expect(clicked).toEqual(['rows.csv']);
		vi.runAllTimers();
		expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:callfs');
	});

	it('returns a plain error value when the blob URL cannot be created', () => {
		Object.defineProperty(URL, 'createObjectURL', { configurable: true, writable: true, value: undefined });
		const result = download('rows.csv', new Blob(['Name']));
		expect(result.ok).toBe(false);
		if (false === result.ok) {
			expect(result.error.title).toBe(t('u091datatable.download_failed_title'));
			expect(result.error.body).toBe(t('u091datatable.download_failed_body'));
			expect(result.error.retryable).toBe(true);
		}
		expect(clicked).toEqual([]);
	});
});

describe('BulkBar', () => {
	const bulkActions = (): MenuItem[] => [
		action('move', 'Move 2 to trash', { icon: 'trash' }),
		action('restore', 'Restore 2', { icon: 'restore' }),
		action('copy', 'Copy 2 paths', { icon: 'copy' }),
		action('share', 'Share 2 links', { icon: 'share' })
	];

	const barProps = (overrides: Record<string, unknown> = {}) => ({
		count: 2,
		allMatching: false,
		actions: bulkActions(),
		onselectall: vi.fn(),
		onclear: vi.fn(),
		...overrides
	});

	it('renders nothing when no row is selected', () => {
		render(BulkBar, { props: barProps({ count: 0 }) });
		expect(screen.queryByRole('region', { name: t('u091datatable.bulk_label') })).toBeNull();
	});

	it('shows the selected count and the total size of the selection', () => {
		render(BulkBar, { props: barProps({ bytes: 38200000000 }) });
		const bar = screen.getByRole('region', { name: t('u091datatable.bulk_label') });
		expect(bar).toHaveTextContent('2 selected');
		expect(bar).toHaveTextContent('38.2 GB');
	});

	it('omits the size when it is unknown', () => {
		render(BulkBar, { props: barProps({ bytes: null }) });
		expect(screen.getByRole('region', { name: t('u091datatable.bulk_label') })).not.toHaveTextContent('GB');
	});

	it('shows the matching total when every matching row is selected', () => {
		render(BulkBar, { props: barProps({ count: 50, allMatching: true, total: 1234 }) });
		expect(screen.getByRole('region', { name: t('u091datatable.bulk_label') })).toHaveTextContent(
			'1,234 matching selected'
		);
	});

	it('shows the first three actions as buttons with the first one primary', () => {
		render(BulkBar, { props: barProps() });
		const first = screen.getByRole('button', { name: 'Move 2 to trash' });
		expect(first).toHaveAttribute('data-variant', 'primary');
		expect(screen.getByRole('button', { name: 'Restore 2' })).toHaveAttribute('data-variant', 'secondary');
		expect(screen.getByRole('button', { name: 'Copy 2 paths' })).toBeTruthy();
		expect(screen.queryByRole('button', { name: 'Share 2 links' })).toBeNull();
	});

	it('runs an overflow action from the More actions menu', async () => {
		const share = bulkActions()[3];
		render(BulkBar, { props: barProps({ actions: [...bulkActions().slice(0, 3), share] }) });
		await fireEvent.keyDown(screen.getByRole('button', { name: t('u091datatable.more_actions') }), { key: 'Enter' });
		await fireEvent.keyDown(screen.getByRole('menuitem', { name: /Share 2 links/ }), { key: 'Enter' });
		expect(share.onselect).toHaveBeenCalledTimes(1);
	});

	it('exposes a disabled action through aria-disabled with its reason', () => {
		const actions = [action('move', 'Move 2 to trash', { disabledReason: 'Only the owner can move these.' })];
		render(BulkBar, { props: barProps({ actions }) });
		expect(screen.getByRole('button', { name: 'Move 2 to trash' })).toHaveAttribute('aria-disabled', 'true');
	});

	it('selects all matching rows with the formatted total', async () => {
		const onselectall = vi.fn();
		render(BulkBar, { props: barProps({ total: 1234, onselectall }) });
		await fireEvent.click(screen.getByRole('button', { name: 'Select all 1,234 matching' }));
		expect(onselectall).toHaveBeenCalledTimes(1);
	});

	it('hides select all when the total is not larger than the selection', () => {
		render(BulkBar, { props: barProps({ total: 2 }) });
		expect(screen.queryByRole('button', { name: /Select all/ })).toBeNull();
	});

	it('clears the selection from the Clear selection button', async () => {
		const onclear = vi.fn();
		render(BulkBar, { props: barProps({ onclear }) });
		await fireEvent.click(screen.getByRole('button', { name: t('u091datatable.clear') }));
		expect(onclear).toHaveBeenCalledTimes(1);
	});
});
