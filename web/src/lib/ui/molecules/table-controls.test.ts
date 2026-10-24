import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { ColumnDef } from '../types';
import ColumnChooser from './ColumnChooser.svelte';
import CursorPager from './CursorPager.svelte';
import DensityToggle from './DensityToggle.svelte';
import Pagination from './Pagination.svelte';
import SortHeader from './SortHeader.svelte';

afterEach(() => {
	cleanup();
});

const column = (id: string, header: string, hidden = false): ColumnDef<unknown> => ({
	id,
	header,
	hidden,
	cell: createRawSnippet<[unknown]>(() => ({ render: () => '<span></span>' }))
});

const columns: ColumnDef<unknown>[] = [column('name', 'Name'), column('size', 'Size'), column('modified', 'Modified', true)];

describe('Pagination', () => {
	it('names the navigation and shows the visible range in whole numbers', () => {
		render(Pagination, { props: { page: 1, pageSize: 50, total: 1234 } });
		expect(screen.queryByRole('navigation', { name: t('tablecontrols.pagination.label') })).not.toBeNull();
		expect(
			screen.queryByText(t('tablecontrols.pagination.range', { from: '1', to: '50', total: '1,234' }))
		).not.toBeNull();
	});

	it('advances one page with Next', async () => {
		render(Pagination, { props: { page: 1, pageSize: 50, total: 1234 } });
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.pagination.next') }));
		expect(
			screen.queryByText(t('tablecontrols.pagination.range', { from: '51', to: '100', total: '1,234' }))
		).not.toBeNull();
	});

	it('disables First and Previous on the first page', () => {
		render(Pagination, { props: { page: 1, pageSize: 50, total: 1234 } });
		expect(screen.getByRole('button', { name: t('tablecontrols.pagination.first') }).hasAttribute('disabled')).toBe(true);
		expect(screen.getByRole('button', { name: t('tablecontrols.pagination.previous') }).hasAttribute('disabled')).toBe(true);
	});

	it('jumps to the last page with Last', async () => {
		render(Pagination, { props: { page: 1, pageSize: 50, total: 1234 } });
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.pagination.last') }));
		expect(
			screen.queryByText(t('tablecontrols.pagination.range', { from: '1,201', to: '1,234', total: '1,234' }))
		).not.toBeNull();
	});

	it('returns to page one when the page size changes', async () => {
		render(Pagination, { props: { page: 1, pageSize: 50, total: 1234 } });
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.pagination.next') }));
		await fireEvent.change(screen.getByRole('combobox', { name: t('tablecontrols.pagination.size_label') }), {
			target: { value: '100' }
		});
		expect(
			screen.queryByText(t('tablecontrols.pagination.range', { from: '1', to: '100', total: '1,234' }))
		).not.toBeNull();
	});

	it('shows the empty copy when there are no items', () => {
		render(Pagination, { props: { page: 1, pageSize: 25, total: 0 } });
		expect(screen.queryByText(t('tablecontrols.pagination.empty'))).not.toBeNull();
	});
});

describe('CursorPager', () => {
	it('shows the count label and names the navigation', () => {
		render(CursorPager, {
			props: { hasPrev: false, hasNext: true, onprev: vi.fn(), onnext: vi.fn(), countLabel: 'Showing 1,000 · more available' }
		});
		expect(screen.queryByRole('navigation', { name: t('tablecontrols.pagination.label') })).not.toBeNull();
		expect(screen.queryByText('Showing 1,000 · more available')).not.toBeNull();
	});

	it('disables only the direction without a page', () => {
		render(CursorPager, {
			props: { hasPrev: false, hasNext: true, onprev: vi.fn(), onnext: vi.fn(), countLabel: '1,000' }
		});
		expect(screen.getByRole('button', { name: t('tablecontrols.pagination.previous') }).hasAttribute('disabled')).toBe(true);
		expect(screen.getByRole('button', { name: t('tablecontrols.pagination.next') }).hasAttribute('disabled')).toBe(false);
	});

	it('calls onnext and onprev when the matching button is pressed', async () => {
		const onprev = vi.fn();
		const onnext = vi.fn();
		render(CursorPager, { props: { hasPrev: true, hasNext: true, onprev, onnext, countLabel: '1,000' } });
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.pagination.previous') }));
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.pagination.next') }));
		expect(onprev).toHaveBeenCalledTimes(1);
		expect(onnext).toHaveBeenCalledTimes(1);
	});
});

describe('SortHeader', () => {
	const sortButton = (): HTMLElement => screen.getByRole('button', { name: 'Size' });

	it('exposes aria-sort none when unsorted and cycles to ascending on press', async () => {
		const onsort = vi.fn();
		render(SortHeader, { props: { label: 'Size', sortKey: 'size', sort: null, onsort } });
		expect(sortButton().closest('th')?.getAttribute('aria-sort')).toBe('none');
		await fireEvent.click(sortButton());
		expect(onsort).toHaveBeenCalledWith({ key: 'size', dir: 'asc' });
	});

	it('cycles ascending to descending', async () => {
		const onsort = vi.fn();
		render(SortHeader, { props: { label: 'Size', sortKey: 'size', sort: { key: 'size', dir: 'asc' }, onsort } });
		expect(sortButton().closest('th')?.getAttribute('aria-sort')).toBe('ascending');
		await fireEvent.click(sortButton());
		expect(onsort).toHaveBeenCalledWith({ key: 'size', dir: 'desc' });
	});

	it('cycles descending back to no sort', async () => {
		const onsort = vi.fn();
		render(SortHeader, { props: { label: 'Size', sortKey: 'size', sort: { key: 'size', dir: 'desc' }, onsort } });
		expect(sortButton().closest('th')?.getAttribute('aria-sort')).toBe('descending');
		await fireEvent.click(sortButton());
		expect(onsort).toHaveBeenCalledWith(null);
	});

	it('ignores a sort on another column', () => {
		render(SortHeader, { props: { label: 'Size', sortKey: 'size', sort: { key: 'name', dir: 'asc' }, onsort: vi.fn() } });
		expect(sortButton().closest('th')?.getAttribute('aria-sort')).toBe('none');
	});
});

describe('ColumnChooser', () => {
	const openChooser = async (): Promise<void> => {
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.columns.trigger') }));
	};

	it('lists each column with its visibility state', async () => {
		render(ColumnChooser, { props: { columns, visible: ['name', 'size'], order: ['name', 'size', 'modified'] } });
		await openChooser();
		expect(screen.getByRole('checkbox', { name: 'Size' }).getAttribute('aria-checked')).toBe('true');
		expect(screen.getByRole('checkbox', { name: 'Modified' }).getAttribute('aria-checked')).toBe('false');
	});

	it('shows a hidden column when its checkbox is checked', async () => {
		render(ColumnChooser, { props: { columns, visible: ['name', 'size'], order: ['name', 'size', 'modified'] } });
		await openChooser();
		await fireEvent.click(screen.getByRole('checkbox', { name: 'Modified' }));
		expect(screen.getByRole('checkbox', { name: 'Modified' }).getAttribute('aria-checked')).toBe('true');
	});

	it('moves a column up and disables Up on the first row', async () => {
		render(ColumnChooser, { props: { columns, visible: ['name', 'size'], order: ['name', 'size', 'modified'] } });
		await openChooser();
		expect(screen.getByRole('button', { name: 'Move Name up' }).hasAttribute('disabled')).toBe(true);
		await fireEvent.click(screen.getByRole('button', { name: 'Move Size up' }));
		const size = screen.getByRole('checkbox', { name: 'Size' });
		const name = screen.getByRole('checkbox', { name: 'Name' });
		expect(size.compareDocumentPosition(name) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
	});

	it('restores the default order and visibility on Reset columns', async () => {
		render(ColumnChooser, { props: { columns, visible: ['size'], order: ['size', 'name', 'modified'] } });
		await openChooser();
		await fireEvent.click(screen.getByRole('button', { name: t('tablecontrols.columns.reset') }));
		const name = screen.getByRole('checkbox', { name: 'Name' });
		const size = screen.getByRole('checkbox', { name: 'Size' });
		expect(name.compareDocumentPosition(size) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
		expect(name.getAttribute('aria-checked')).toBe('true');
	});
});

describe('DensityToggle', () => {
	const option = (label: string): HTMLElement => screen.getByText(label).closest('button') as HTMLElement;

	it('labels the three densities and marks the current one', () => {
		render(DensityToggle, { props: { value: 'comfortable' } });
		expect(option(t('tablecontrols.density.compact')).getAttribute('data-state')).toBe('off');
		expect(option(t('tablecontrols.density.comfortable')).getAttribute('data-state')).toBe('on');
	});

	it('selects the pressed density', async () => {
		render(DensityToggle, { props: { value: 'default' } });
		await fireEvent.click(option(t('tablecontrols.density.compact')));
		expect(option(t('tablecontrols.density.compact')).getAttribute('data-state')).toBe('on');
		expect(option(t('tablecontrols.density.default')).getAttribute('data-state')).toBe('off');
	});
});
