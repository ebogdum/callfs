import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { MenuItem } from '../types';
import ActionsCell from './ActionsCell.svelte';
import BooleanCell from './BooleanCell.svelte';
import CheckboxCell from './CheckboxCell.svelte';
import TagsCell from './TagsCell.svelte';

afterEach(() => {
	cleanup();
});

const menuItem = (id: string, label: string, onselect: () => void, extra: Partial<MenuItem> = {}): MenuItem => ({
	id,
	label,
	onselect,
	...extra
});

describe('CheckboxCell', () => {
	it('names the checkbox after its row label', () => {
		render(CheckboxCell, { props: { checked: false, label: 'Select report.pdf', onchange: vi.fn() } });
		expect(screen.getByRole('checkbox', { name: 'Select report.pdf' })).toBeTruthy();
	});

	it('reflects the checked value in aria-checked', () => {
		render(CheckboxCell, { props: { checked: true, label: 'Select report.pdf', onchange: vi.fn() } });
		expect(screen.getByRole('checkbox', { name: 'Select report.pdf' }).getAttribute('aria-checked')).toBe('true');
	});

	it('reports the next checked value on click', async () => {
		const onchange = vi.fn();
		render(CheckboxCell, { props: { checked: false, label: 'Select report.pdf', onchange } });
		await fireEvent.click(screen.getByRole('checkbox', { name: 'Select report.pdf' }));
		expect(onchange).toHaveBeenCalledTimes(1);
		expect(onchange).toHaveBeenCalledWith(true);
	});
});

describe('ActionsCell', () => {
	it('renders inline actions by label and a kebab named for the row', () => {
		const onselect = vi.fn();
		render(ActionsCell, {
			props: {
				inline: [menuItem('share', 'Share', onselect, { icon: 'share' })],
				menu: [menuItem('rename', 'Rename', onselect)],
				label: 'report.pdf'
			}
		});
		expect(screen.getByRole('button', { name: 'Share' })).toBeTruthy();
		expect(screen.getByRole('button', { name: `${t('common.more')} report.pdf` })).toBeTruthy();
	});

	it('runs the inline action on click', async () => {
		const onselect = vi.fn();
		render(ActionsCell, {
			props: {
				inline: [menuItem('share', 'Share', onselect, { icon: 'share' })],
				menu: [menuItem('rename', 'Rename', onselect)],
				label: 'report.pdf'
			}
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Share' }));
		expect(onselect).toHaveBeenCalledTimes(1);
	});

	it('opens the full menu with Shift+F10 on the kebab', async () => {
		const onselect = vi.fn();
		render(ActionsCell, {
			props: {
				menu: [menuItem('rename', 'Rename', onselect), menuItem('move', 'Move', onselect)],
				label: 'report.pdf'
			}
		});
		const kebab = screen.getByRole('button', { name: `${t('common.more')} report.pdf` });
		await fireEvent.keyDown(kebab, { key: 'F10', shiftKey: true });
		expect(await screen.findByRole('menu')).toBeTruthy();
		expect(screen.getByRole('menuitem', { name: 'Rename' })).toBeTruthy();
		expect(screen.getByRole('menuitem', { name: 'Move' })).toBeTruthy();
	});

	it('lists danger items last in the menu', async () => {
		const onselect = vi.fn();
		render(ActionsCell, {
			props: {
				menu: [menuItem('delete', 'Delete', onselect, { tone: 'danger' }), menuItem('rename', 'Rename', onselect)],
				label: 'report.pdf'
			}
		});
		const kebab = screen.getByRole('button', { name: `${t('common.more')} report.pdf` });
		await fireEvent.keyDown(kebab, { key: 'F10', shiftKey: true });
		await screen.findByRole('menu');
		const names = screen.getAllByRole('menuitem').map((element) => element.textContent?.trim());
		expect(names).toEqual(['Rename', 'Delete']);
	});

	it('marks a menu item with a disabled reason as disabled', async () => {
		const onselect = vi.fn();
		render(ActionsCell, {
			props: {
				menu: [menuItem('restore', 'Restore', onselect, { disabledReason: 'Trash is empty' })],
				label: 'report.pdf'
			}
		});
		const kebab = screen.getByRole('button', { name: `${t('common.more')} report.pdf` });
		await fireEvent.keyDown(kebab, { key: 'F10', shiftKey: true });
		await screen.findByRole('menu');
		expect(screen.getByRole('menuitem', { name: /Restore/ }).hasAttribute('data-disabled')).toBe(true);
	});
});

describe('BooleanCell', () => {
	it('shows the true word and check glyph for true', () => {
		const { container } = render(BooleanCell, { props: { value: true, trueLabel: 'Yes', falseLabel: 'No' } });
		expect(screen.getByText('Yes')).toBeTruthy();
		expect(container.querySelector('[data-value]')?.getAttribute('data-value')).toBe('true');
	});

	it('shows the false word for false', () => {
		const { container } = render(BooleanCell, { props: { value: false, trueLabel: 'Yes', falseLabel: 'No' } });
		expect(screen.getByText('No')).toBeTruthy();
		expect(container.querySelector('[data-value]')?.getAttribute('data-value')).toBe('false');
	});

	it('shows the unknown word for null', () => {
		const { container } = render(BooleanCell, { props: { value: null, trueLabel: 'Yes', falseLabel: 'No' } });
		expect(screen.getByText(t('common.unknown'))).toBeTruthy();
		expect(container.querySelector('[data-value]')?.getAttribute('data-value')).toBe('unknown');
	});
});

describe('TagsCell', () => {
	it('shows the first two values as tags', () => {
		render(TagsCell, { props: { values: ['alpha', 'beta', 'gamma'] } });
		expect(screen.getByText('alpha')).toBeTruthy();
		expect(screen.getByText('beta')).toBeTruthy();
		expect(screen.queryByText('gamma')).toBeNull();
	});

	it('shows the hidden count as +N and keeps the hidden values in the tooltip list', () => {
		const { container } = render(TagsCell, { props: { values: ['alpha', 'beta', 'gamma', 'delta'] } });
		expect(container.querySelector('.overflow')?.textContent).toBe('+2');
	});

	it('renders no overflow marker when every value fits', () => {
		const { container } = render(TagsCell, { props: { values: ['alpha', 'beta'] } });
		expect(container.querySelector('.overflow')).toBeNull();
	});

	it('honours a custom max', () => {
		const { container } = render(TagsCell, { props: { values: ['alpha', 'beta', 'gamma'], max: 1 } });
		expect(screen.getByText('alpha')).toBeTruthy();
		expect(screen.queryByText('beta')).toBeNull();
		expect(container.querySelector('.overflow')?.textContent).toBe('+2');
	});
});
