import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import { formatDuration } from '$lib/format/duration';
import DurationInput from './DurationInput.svelte';
import PathInput from './PathInput.svelte';
import SearchInput from './SearchInput.svelte';
import SizeInput from './SizeInput.svelte';

afterEach(() => {
	cleanup();
});

const valueOf = (element: HTMLElement): string => (element as HTMLInputElement).value;

describe('SearchInput', () => {
	it('names the searchbox by its visible label', () => {
		render(SearchInput, { props: { label: 'Filter this folder', value: '' } });
		expect(screen.getByRole('searchbox', { name: 'Filter this folder' })).toBeTruthy();
	});

	it('calls onsubmit with the typed value on Enter', async () => {
		const onsubmit = vi.fn();
		render(SearchInput, { props: { label: 'Filter', value: 'report', onsubmit } });
		await fireEvent.keyDown(screen.getByRole('searchbox', { name: 'Filter' }), { key: 'Enter' });
		expect(onsubmit).toHaveBeenCalledWith('report');
	});

	it('clears on the first Escape and blurs on the second', async () => {
		render(SearchInput, { props: { label: 'Filter', value: 'report' } });
		const field = screen.getByRole('searchbox', { name: 'Filter' });
		field.focus();
		await fireEvent.keyDown(field, { key: 'Escape' });
		expect(valueOf(field)).toBe('');
		await fireEvent.keyDown(field, { key: 'Escape' });
		expect(document.activeElement).not.toBe(field);
	});

	it('clears with the clear button named by its copy key', async () => {
		render(SearchInput, { props: { label: 'Filter', value: 'report' } });
		const field = screen.getByRole('searchbox', { name: 'Filter' });
		await fireEvent.click(screen.getByRole('button', { name: t('common.clear') }));
		expect(valueOf(field)).toBe('');
	});

	it('announces the result count politely', () => {
		render(SearchInput, { props: { label: 'Filter', value: 'abc', resultCount: 3 } });
		expect(screen.getByRole('status').textContent?.trim()).toBe(t('inputs.search.results', { count: 3 }));
	});

	it('shows the shortcut only while empty and not loading', () => {
		const { unmount } = render(SearchInput, { props: { label: 'Filter', value: '', loading: false } });
		expect(screen.getByText('/')).toBeTruthy();
		unmount();
		render(SearchInput, { props: { label: 'Filter', value: '', loading: true } });
		expect(screen.queryByText('/')).toBeNull();
	});
});

describe('PathInput', () => {
	it('names the textbox by its visible label without suggestions', () => {
		render(PathInput, { props: { label: 'Path', value: '' } });
		expect(screen.getByRole('textbox', { name: 'Path' })).toBeTruthy();
	});

	it('accepts a pasted path without a leading slash', async () => {
		const onsubmit = vi.fn();
		render(PathInput, { props: { label: 'Path', value: '', onsubmit } });
		const field = screen.getByRole('textbox', { name: 'Path' });
		await fireEvent.input(field, { target: { value: 'team/docs' } });
		await fireEvent.keyDown(field, { key: 'Enter' });
		expect(onsubmit).toHaveBeenCalledWith('/team/docs');
	});

	it('marks a path with a backslash invalid and does not submit it', async () => {
		const onsubmit = vi.fn();
		render(PathInput, { props: { label: 'Path', value: '', onsubmit } });
		const field = screen.getByRole('textbox', { name: 'Path' });
		await fireEvent.input(field, { target: { value: 'a\\b' } });
		await fireEvent.keyDown(field, { key: 'Enter' });
		expect(field.getAttribute('aria-invalid')).toBe('true');
		expect(screen.getByText(t('path.error.backslash'))).toBeTruthy();
		expect(onsubmit).not.toHaveBeenCalled();
	});

	it('opens a listbox of suggested folders with the combobox pattern', async () => {
		const suggest = vi.fn(async () => ['/team/', '/team/archive/']);
		render(PathInput, { props: { label: 'Path', value: '', suggest } });
		const field = screen.getByRole('combobox', { name: 'Path' });
		await fireEvent.focus(field);
		const options = await screen.findAllByRole('option');
		expect(options.map((option) => option.textContent)).toEqual(['/team/', '/team/archive/']);
		expect(suggest).toHaveBeenCalledWith('/');
	});

	it('chooses the active suggestion with ArrowDown and Enter', async () => {
		const suggest = vi.fn(async () => ['/team/', '/team/archive/']);
		const onsubmit = vi.fn();
		render(PathInput, { props: { label: 'Path', value: '', suggest, onsubmit } });
		const field = screen.getByRole('combobox', { name: 'Path' });
		await fireEvent.focus(field);
		await screen.findAllByRole('option');
		await fireEvent.keyDown(field, { key: 'ArrowDown' });
		await fireEvent.keyDown(field, { key: 'Enter' });
		expect(valueOf(field)).toBe('/team/');
		expect(onsubmit).not.toHaveBeenCalled();
	});

	it('closes the listbox on Escape', async () => {
		const suggest = vi.fn(async () => ['/team/']);
		render(PathInput, { props: { label: 'Path', value: '', suggest } });
		const field = screen.getByRole('combobox', { name: 'Path' });
		await fireEvent.focus(field);
		await screen.findAllByRole('option');
		await fireEvent.keyDown(field, { key: 'Escape' });
		expect(screen.queryByRole('listbox')).toBeNull();
	});

	it('shows the no-match copy when no folder starts with the text', async () => {
		const suggest = vi.fn(async () => []);
		render(PathInput, { props: { label: 'Path', value: '/zzz', suggest } });
		await fireEvent.focus(screen.getByRole('combobox', { name: 'Path' }));
		expect(await screen.findByText(t('inputs.path.no_match'))).toBeTruthy();
	});

	it('shows the suggestion error copy when suggestions fail', async () => {
		const suggest = vi.fn(() => Promise.reject(new Error('unavailable')));
		render(PathInput, { props: { label: 'Path', value: '', suggest } });
		await fireEvent.focus(screen.getByRole('combobox', { name: 'Path' }));
		expect(await screen.findByText(t('inputs.path.suggest_error'))).toBeTruthy();
	});
});

describe('DurationInput', () => {
	it('shows the amount and the largest exact unit', () => {
		render(DurationInput, { props: { label: 'Timeout', value: '1h30m' } });
		expect(valueOf(screen.getByRole('textbox', { name: 'Timeout' }))).toBe('90');
		expect(valueOf(screen.getByRole('combobox', { name: `Timeout, ${t('inputs.unit')}` }))).toBe('m');
	});

	it('shows the long form of the duration', () => {
		render(DurationInput, { props: { label: 'Timeout', value: '30s' } });
		expect(screen.getByText(formatDuration(30000, 'long'))).toBeTruthy();
	});

	it('writes a Go duration when the amount changes', async () => {
		let stored = '30s';
		render(DurationInput, {
			props: {
				label: 'Timeout',
				get value() {
					return stored;
				},
				set value(next: string) {
					stored = next;
				}
			}
		});
		const field = screen.getByRole('textbox', { name: 'Timeout' });
		await fireEvent.input(field, { target: { value: '2' } });
		await fireEvent.blur(field);
		expect(stored).toBe('2s');
	});

	it('writes a Go duration when the unit changes', async () => {
		let stored = '30s';
		render(DurationInput, {
			props: {
				label: 'Timeout',
				get value() {
					return stored;
				},
				set value(next: string) {
					stored = next;
				}
			}
		});
		await fireEvent.change(screen.getByRole('combobox', { name: `Timeout, ${t('inputs.unit')}` }), {
			target: { value: 'm' }
		});
		expect(stored).toBe('30m');
	});

	it('marks an amount below min invalid and names the minimum', () => {
		render(DurationInput, { props: { label: 'Timeout', value: '500ms', min: '1s' } });
		const field = screen.getByRole('textbox', { name: 'Timeout' });
		expect(field.getAttribute('aria-invalid')).toBe('true');
		expect(screen.getByText(t('inputs.duration.below_min', { value: formatDuration(1000, 'long') }))).toBeTruthy();
	});
});

describe('SizeInput', () => {
	it('shows the amount in the largest unit that is at least one', () => {
		render(SizeInput, { props: { label: 'Size', value: 1500 } });
		expect(valueOf(screen.getByRole('textbox', { name: 'Size' }))).toBe('1.5');
		expect(valueOf(screen.getByRole('combobox', { name: `Size, ${t('inputs.unit')}` }))).toBe('kB');
	});

	it('shows zero bytes in bytes', () => {
		render(SizeInput, { props: { label: 'Size', value: 0 } });
		expect(valueOf(screen.getByRole('textbox', { name: 'Size' }))).toBe('0');
		expect(valueOf(screen.getByRole('combobox', { name: `Size, ${t('inputs.unit')}` }))).toBe('B');
	});

	it('writes bytes when the amount changes', async () => {
		let stored = 1500;
		render(SizeInput, {
			props: {
				label: 'Size',
				get value() {
					return stored;
				},
				set value(next: number) {
					stored = next;
				}
			}
		});
		const field = screen.getByRole('textbox', { name: 'Size' });
		await fireEvent.input(field, { target: { value: '2' } });
		await fireEvent.blur(field);
		expect(stored).toBe(2000);
	});

	it('keeps the amount and rescales the bytes when the unit changes', async () => {
		let stored = 1500;
		render(SizeInput, {
			props: {
				label: 'Size',
				get value() {
					return stored;
				},
				set value(next: number) {
					stored = next;
				}
			}
		});
		await fireEvent.change(screen.getByRole('combobox', { name: `Size, ${t('inputs.unit')}` }), {
			target: { value: 'MB' }
		});
		expect(stored).toBe(1500000);
	});
});
