import { cleanup, fireEvent, render, waitFor } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FileEntry, Result } from '$lib/api/types';
import { t } from '$lib/copy/t';
import type { Selection, UiError } from '../types';
import FileGrid from './FileGrid.svelte';
import FileTable from './FileTable.svelte';
import FileTree from './FileTree.svelte';

afterEach(() => {
	cleanup();
});

const fileEntry = (overrides: Partial<FileEntry> & Pick<FileEntry, 'name' | 'path'>): FileEntry => ({
	type: 'file',
	size: 2048,
	mode: '0644',
	owner: 'alice',
	mtime: '2026-10-01T10:00:00Z',
	etag: '"9f2c4a7b1e0d"',
	storage: 'localfs',
	instance_id: 'node-a',
	erasure_coded: false,
	locked: false,
	...overrides
});

const folderEntry = (name: string, path: string): FileEntry => fileEntry({ name, path, type: 'directory', size: 0 });

const selectionOf = (ids: string[], mode: Selection['mode'] = 'multi'): Selection => ({
	mode,
	ids: new Set(ids),
	allMatching: false
});

const uiError: UiError = { title: 'Failed', body: 'Failed', retryable: true };

const ok = (value: FileEntry[]): Result<FileEntry[], UiError> => ({ ok: true, value });

const failed = (): Result<FileEntry[], UiError> => ({ ok: false, error: uiError });

const tableProps = (entries: FileEntry[], overrides: Record<string, unknown> = {}) => ({
	entries,
	status: 'success' as const,
	selection: selectionOf([]),
	hiddenVisible: false,
	...overrides
});

describe('FileTable', () => {
	it('renders a named table with folders listed before files', async () => {
		const { getByRole } = render(
			FileTable,
			{
				props: tableProps([
					fileEntry({ name: 'report.pdf', path: '/team/report.pdf' }),
					folderEntry('Archive', '/team/Archive')
				])
			}
		);
		const table = getByRole('table', { name: t('files.table.label') });
		const rows = table.querySelectorAll('tbody tr');
		expect(rows[0]?.textContent).toContain('Archive');
		expect(rows[1]?.textContent).toContain('report.pdf');
	});

	it('hides dot entries until hidden entries are shown', () => {
		const entries = [fileEntry({ name: '.env', path: '/team/.env' })];
		const hidden = render(FileTable, { props: tableProps(entries) });
		expect(hidden.queryByLabelText('.env')).toBeNull();
		hidden.unmount();
		const shown = render(FileTable, { props: tableProps(entries, { hiddenVisible: true }) });
		expect(shown.getByLabelText('.env')).toBeTruthy();
	});

	it('marks only folder name cells as drop targets', () => {
		const { container } = render(FileTable, {
			props: tableProps([
				fileEntry({ name: 'report.pdf', path: '/team/report.pdf' }),
				folderEntry('Archive', '/team/Archive')
			])
		});
		expect(container.querySelectorAll('[data-drop-target]')).toHaveLength(1);
	});

	it('shows an inline rename input for the path being renamed', () => {
		const { container } = render(FileTable, {
			props: tableProps([fileEntry({ name: 'report.pdf', path: '/team/report.pdf' })], {
				renaming: '/team/report.pdf'
			})
		});
		expect(container.querySelector('input.rename')).not.toBeNull();
	});

	it('checks the select box of every selected row', () => {
		const { getByRole } = render(FileTable, {
			props: tableProps(
				[
					fileEntry({ name: 'report.pdf', path: '/team/report.pdf' }),
					fileEntry({ name: 'notes.txt', path: '/team/notes.txt' })
				],
				{ selection: selectionOf(['/team/report.pdf']) }
			)
		});
		const selected = getByRole('checkbox', { name: t('files.table.select_row', { name: 'report.pdf' }) });
		const unselected = getByRole('checkbox', { name: t('files.table.select_row', { name: 'notes.txt' }) });
		expect(selected.getAttribute('aria-checked')).toBe('true');
		expect(unselected.getAttribute('aria-checked')).toBe('false');
	});

	it('shows the folder empty copy when there are no entries', () => {
		const { getByText } = render(FileTable, { props: tableProps([]) });
		expect(getByText(t('files.table.empty_title'))).toBeTruthy();
	});
});

describe('FileGrid', () => {
	const gridEntries = (): FileEntry[] => [
		fileEntry({ name: 'report.pdf', path: '/team/report.pdf' }),
		fileEntry({ name: 'notes.txt', path: '/team/notes.txt' })
	];

	it('renders one named option per entry', () => {
		const { getAllByRole, getByRole } = render(FileGrid, {
			props: { entries: gridEntries(), selection: selectionOf([]), thumbnails: () => null }
		});
		expect(getByRole('listbox', { name: t('files.grid.label') })).toBeTruthy();
		const options = getAllByRole('option');
		expect(options.map((option) => option.textContent?.trim())).toEqual(['report.pdf', 'notes.txt']);
	});

	it('marks selected entries with aria-selected', () => {
		const { getAllByRole } = render(FileGrid, {
			props: {
				entries: gridEntries(),
				selection: selectionOf(['/team/notes.txt']),
				thumbnails: () => null
			}
		});
		const [first, second] = getAllByRole('option');
		expect(first?.getAttribute('aria-selected')).toBe('false');
		expect(second?.getAttribute('aria-selected')).toBe('true');
	});

	it('keeps exactly one option in the tab order', () => {
		const { getAllByRole } = render(FileGrid, {
			props: { entries: gridEntries(), selection: selectionOf([]), thumbnails: () => null }
		});
		const stops = getAllByRole('option').filter((option) => '0' === option.getAttribute('tabindex'));
		expect(stops).toHaveLength(1);
	});

	it('moves focus to the next card on ArrowRight', async () => {
		const { getAllByRole } = render(FileGrid, {
			props: { entries: gridEntries(), selection: selectionOf([]), thumbnails: () => null }
		});
		const [first, second] = getAllByRole('option') as [HTMLElement, HTMLElement];
		first.focus();
		await fireEvent.keyDown(first, { key: 'ArrowRight' });
		await waitFor(() => {
			expect(document.activeElement).toBe(second);
		});
		expect(second.getAttribute('tabindex')).toBe('0');
	});

	it('opens the focused entry on Enter', async () => {
		const onopen = vi.fn();
		const { getAllByRole } = render(FileGrid, {
			props: { entries: gridEntries(), selection: selectionOf([]), thumbnails: () => null, onopen }
		});
		const [first] = getAllByRole('option') as [HTMLElement];
		await fireEvent.keyDown(first, { key: 'Enter' });
		expect(onopen).toHaveBeenCalledWith(gridEntries()[0]);
	});

	it('opens an entry on double click', async () => {
		const onopen = vi.fn();
		const { getAllByRole } = render(FileGrid, {
			props: { entries: gridEntries(), selection: selectionOf([]), thumbnails: () => null, onopen }
		});
		const [, second] = getAllByRole('option') as [HTMLElement, HTMLElement];
		await fireEvent.dblClick(second);
		expect(onopen).toHaveBeenCalledWith(gridEntries()[1]);
	});

	it('shows the folder empty copy when there are no entries', () => {
		const { getByText } = render(FileGrid, {
			props: { entries: [], selection: selectionOf([]), thumbnails: () => null }
		});
		expect(getByText(t('files.table.empty_title'))).toBeTruthy();
	});
});

describe('FileTree', () => {
	const tree = (children: Record<string, Result<FileEntry[], UiError>>) =>
		vi.fn(async (path: string): Promise<Result<FileEntry[], UiError>> => {
			const result = children[path];
			return undefined === result ? ok([]) : result;
		});

	it('lists the child folders of the root and hides files', async () => {
		const load = tree({
			'/': ok([folderEntry('Documents', '/Documents'), fileEntry({ name: 'readme.txt', path: '/readme.txt' })])
		});
		const { findByRole, queryByRole } = render(FileTree, {
			props: { root: '/', load, current: '/', onnavigate: vi.fn() }
		});
		expect(await findByRole('treeitem', { name: 'Documents' })).toBeTruthy();
		expect(queryByRole('treeitem', { name: 'readme.txt' })).toBeNull();
	});

	it('expands a collapsed folder with ArrowRight and loads its children', async () => {
		const load = tree({
			'/': ok([folderEntry('Documents', '/Documents')]),
			'/Documents': ok([folderEntry('Notes', '/Documents/Notes')])
		});
		const { findByRole, getByRole } = render(FileTree, {
			props: { root: '/', load, current: '/', onnavigate: vi.fn() }
		});
		const documents = await findByRole('treeitem', { name: 'Documents' });
		documents.focus();
		await fireEvent.keyDown(documents, { key: 'ArrowRight' });
		expect(load).toHaveBeenCalledWith('/Documents');
		expect(await findByRole('treeitem', { name: 'Notes' })).toBeTruthy();
		expect(getByRole('treeitem', { name: t('files.tree.root') }).getAttribute('aria-expanded')).toBe('true');
	});

	it('moves focus with ArrowDown and calls onnavigate on Enter', async () => {
		const onnavigate = vi.fn();
		const load = tree({ '/': ok([folderEntry('Documents', '/Documents')]) });
		const { findByRole, getByRole } = render(FileTree, {
			props: { root: '/', load, current: '/', onnavigate }
		});
		const rootItem = getByRole('treeitem', { name: t('files.tree.root') });
		const documents = await findByRole('treeitem', { name: 'Documents' });
		rootItem.focus();
		await fireEvent.keyDown(rootItem, { key: 'ArrowDown' });
		await waitFor(() => {
			expect(document.activeElement).toBe(documents);
		});
		await fireEvent.keyDown(documents, { key: 'Enter' });
		expect(onnavigate).toHaveBeenCalledWith('/Documents');
	});

	it('marks the current folder as selected', async () => {
		const load = tree({ '/': ok([folderEntry('Documents', '/Documents')]) });
		const { findByRole } = render(FileTree, {
			props: { root: '/', load, current: '/Documents', onnavigate: vi.fn() }
		});
		const documents = await findByRole('treeitem', { name: 'Documents' });
		expect(documents.getAttribute('aria-selected')).toBe('true');
	});

	it('shows the error copy when a folder fails to load', async () => {
		const load = tree({ '/': failed() });
		const { findByText } = render(FileTree, {
			props: { root: '/', load, current: '/', onnavigate: vi.fn() }
		});
		expect(await findByText(t('files.tree.error'))).toBeTruthy();
	});
});
