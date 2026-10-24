import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import type { MenuItem } from '../types';
import ContextMenu from './ContextMenu.svelte';
import Combobox, { type ComboboxItem } from './Combobox.svelte';
import Menu from './Menu.svelte';
import Popover from './Popover.svelte';

type Props = Record<string, unknown>;

const applyProps = (element: Element, props: Props) => {
	for (const [name, value] of Object.entries(props)) {
		if ('function' === typeof value && name.startsWith('on')) {
			element.addEventListener(name.slice(2), value as EventListener);
		} else if ('string' === typeof value || 'number' === typeof value) {
			element.setAttribute(name, String(value));
		} else if (true === value) {
			element.setAttribute(name, '');
		}
	}
};

const buttonTrigger = (text: string) =>
	createRawSnippet<[Props]>((getProps) => ({
		render: () => `<button type="button">${text}</button>`,
		setup: (element) => {
			applyProps(element, getProps());
		}
	}));

const textSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<span>${text}</span>` }));

const noop = () => {};

afterEach(() => {
	cleanup();
});

describe('Menu', () => {
	const build = () => {
		const onOpen = vi.fn();
		const onDelete = vi.fn();
		const items: MenuItem[] = [
			{ id: 'open', label: 'Open', icon: 'folder-open', shortcut: ['enter'], onselect: onOpen },
			{ id: 'share', label: 'Share', group: 'links', onselect: noop },
			{ id: 'lock', label: 'Lock', disabledReason: 'Owner only', onselect: noop },
			{ id: 'pin', label: 'Pinned', checked: true, onselect: noop },
			{ id: 'delete', label: 'Move to trash', tone: 'danger', onselect: onDelete }
		];
		return { items, onOpen };
	};

	it('exposes the trigger as a menu button with its visible name', () => {
		const { items } = build();
		render(Menu, { props: { trigger: buttonTrigger('Actions'), items, label: 'File actions' } });
		const trigger = screen.getByRole('button', { name: 'Actions' });
		expect(trigger).toHaveAttribute('aria-haspopup', 'menu');
		expect(trigger).toHaveAttribute('aria-expanded', 'false');
	});

	it('opens from the keyboard and names the menu', async () => {
		const { items } = build();
		render(Menu, { props: { trigger: buttonTrigger('Actions'), items, label: 'File actions' } });
		fireEvent.keyDown(screen.getByRole('button', { name: 'Actions' }), { key: 'Enter' });
		expect(await screen.findByRole('menu', { name: 'File actions' })).toBeInTheDocument();
	});

	it('puts danger items last after a separator and exposes groups', async () => {
		const { items } = build();
		render(Menu, { props: { trigger: buttonTrigger('Actions'), items, label: 'File actions' } });
		fireEvent.keyDown(screen.getByRole('button', { name: 'Actions' }), { key: 'Enter' });
		await screen.findByRole('menu', { name: 'File actions' });
		const entries = screen.getAllByRole('menuitem');
		expect(entries.at(-1)).toHaveAttribute('data-tone', 'danger');
		expect(entries.at(-1)).toHaveTextContent('Move to trash');
		expect(screen.getAllByRole('separator')).toHaveLength(2);
	});

	it('runs the item callback on Enter with the item focused', async () => {
		const { items, onOpen } = build();
		render(Menu, { props: { trigger: buttonTrigger('Actions'), items, label: 'File actions' } });
		fireEvent.keyDown(screen.getByRole('button', { name: 'Actions' }), { key: 'Enter' });
		await screen.findByRole('menu', { name: 'File actions' });
		const open = screen.getByRole('menuitem', { name: /Open/ });
		open.focus();
		fireEvent.keyDown(open, { key: 'Enter' });
		await waitFor(() => expect(onOpen).toHaveBeenCalledTimes(1));
	});

	it('marks a disabled item with aria-disabled and shows its reason', async () => {
		const { items } = build();
		render(Menu, { props: { trigger: buttonTrigger('Actions'), items, label: 'File actions' } });
		fireEvent.keyDown(screen.getByRole('button', { name: 'Actions' }), { key: 'Enter' });
		await screen.findByRole('menu', { name: 'File actions' });
		expect(screen.getByRole('menuitem', { name: /Lock/ })).toHaveAttribute('aria-disabled', 'true');
		expect(screen.getByText('Owner only')).toBeInTheDocument();
	});

	it('exposes checked items as checkboxes', async () => {
		const { items } = build();
		render(Menu, { props: { trigger: buttonTrigger('Actions'), items, label: 'File actions' } });
		fireEvent.keyDown(screen.getByRole('button', { name: 'Actions' }), { key: 'Enter' });
		await screen.findByRole('menu', { name: 'File actions' });
		expect(screen.getByRole('menuitemcheckbox', { name: 'Pinned' })).toHaveAttribute('aria-checked', 'true');
	});
});

describe('ContextMenu', () => {
	const rowItems: MenuItem[] = [
		{ id: 'open', label: 'Open', onselect: noop },
		{ id: 'delete', label: 'Move to trash', tone: 'danger', onselect: noop }
	];

	it('opens on right click over the trigger', async () => {
		render(ContextMenu, { props: { items: rowItems, children: textSnippet('Row one') } });
		fireEvent.contextMenu(screen.getByText('Row one'));
		expect(await screen.findByRole('menu')).toBeInTheDocument();
		expect(screen.getAllByRole('menuitem')).toHaveLength(2);
	});

	it('opens on Shift+F10 over the trigger', async () => {
		render(ContextMenu, { props: { items: rowItems, children: textSnippet('Row one') } });
		fireEvent.keyDown(screen.getByText('Row one'), { key: 'F10', shiftKey: true });
		expect(await screen.findByRole('menu')).toBeInTheDocument();
	});

	it('ignores other keys on the trigger', async () => {
		render(ContextMenu, { props: { items: rowItems, children: textSnippet('Row one') } });
		fireEvent.keyDown(screen.getByText('Row one'), { key: 'F10' });
		expect(screen.queryByRole('menu')).toBeNull();
	});
});

describe('Popover', () => {
	it('opens a named dialog from its trigger and closes on Escape', async () => {
		render(Popover, {
			props: {
				trigger: buttonTrigger('Sort'),
				content: textSnippet('Sort by name'),
				label: 'Sort options'
			}
		});
		fireEvent.click(screen.getByRole('button', { name: 'Sort' }));
		const dialog = await screen.findByRole('dialog', { name: 'Sort options' });
		expect(dialog).toHaveTextContent('Sort by name');
		fireEvent.keyDown(dialog, { key: 'Escape' });
		await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
	});

	it('removes the width cap only when wide', async () => {
		render(Popover, {
			props: {
				trigger: buttonTrigger('Columns'),
				content: textSnippet('Columns list'),
				label: 'Columns',
				wide: true
			}
		});
		fireEvent.click(screen.getByRole('button', { name: 'Columns' }));
		expect(await screen.findByRole('dialog', { name: 'Columns' })).toHaveAttribute('data-wide', 'true');
	});

	it('leaves the width cap in place by default', async () => {
		render(Popover, {
			props: {
				trigger: buttonTrigger('Columns'),
				content: textSnippet('Columns list'),
				label: 'Columns'
			}
		});
		fireEvent.click(screen.getByRole('button', { name: 'Columns' }));
		expect(await screen.findByRole('dialog', { name: 'Columns' })).not.toHaveAttribute('data-wide');
	});
});

describe('Combobox', () => {
	const items: ComboboxItem[] = [
		{ value: 'a', label: 'Alpha' },
		{ value: 'b', label: 'Beta' },
		{ value: 'c', label: 'Gamma' }
	];

	it('labels the input with its visible label', () => {
		render(Combobox, { props: { label: 'Owner', items, value: [] } });
		expect(screen.getByRole('combobox', { name: 'Owner' })).toBeInTheDocument();
	});

	it('filters options by typed text', async () => {
		render(Combobox, { props: { label: 'Owner', items, value: [] } });
		fireEvent.input(screen.getByRole('combobox', { name: 'Owner' }), { target: { value: 'al' } });
		expect(await screen.findByRole('option', { name: 'Alpha' })).toBeInTheDocument();
		expect(screen.queryByRole('option', { name: 'Beta' })).toBeNull();
	});

	it('shows the chosen label in single mode', async () => {
		render(Combobox, { props: { label: 'Owner', items, value: [] } });
		const input = screen.getByRole('combobox', { name: 'Owner' });
		fireEvent.input(input, { target: { value: 'be' } });
		await screen.findByRole('option', { name: 'Beta' });
		fireEvent.keyDown(input, { key: 'Enter' });
		await waitFor(() => expect(input).toHaveValue('Beta'));
	});

	it('shows the first two chips and a count for the rest in multiple mode', () => {
		render(Combobox, { props: { label: 'Owners', items, multiple: true, value: ['a', 'b', 'c'] } });
		expect(screen.getByRole('button', { name: 'Alpha' })).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Beta' })).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: 'Gamma' })).toBeNull();
		expect(screen.getByText('+1')).toBeInTheDocument();
	});

	it('removes the last chip on Backspace in an empty input', () => {
		render(Combobox, { props: { label: 'Owners', items, multiple: true, value: ['a', 'b'] } });
		fireEvent.keyDown(screen.getByRole('combobox', { name: 'Owners' }), { key: 'Backspace' });
		expect(screen.queryByRole('button', { name: 'Beta' })).toBeNull();
		expect(screen.getByRole('button', { name: 'Alpha' })).toBeInTheDocument();
	});

	it('offers to create a typed value when creatable', async () => {
		render(Combobox, { props: { label: 'Tags', items: [], multiple: true, creatable: true, value: [] } });
		const input = screen.getByRole('combobox', { name: 'Tags' });
		fireEvent.input(input, { target: { value: 'Delta' } });
		await screen.findByRole('option', { name: 'Delta' });
		fireEvent.keyDown(input, { key: 'Enter' });
		expect(await screen.findByRole('button', { name: 'Delta' })).toBeInTheDocument();
	});

	it('shows the error state when search fails', async () => {
		const search = vi.fn(() => Promise.reject(new Error('offline')));
		render(Combobox, { props: { label: 'Owner', search, value: [] } });
		fireEvent.input(screen.getByRole('combobox', { name: 'Owner' }), { target: { value: 'x' } });
		expect(await screen.findByRole('alert')).toHaveTextContent(t('errors.network.title'));
	});

	it('shows loading and then the search results', async () => {
		const search = vi.fn(async () => [{ value: 'z', label: 'Zeta' }]);
		render(Combobox, { props: { label: 'Owner', search, value: [] } });
		fireEvent.input(screen.getByRole('combobox', { name: 'Owner' }), { target: { value: 'z' } });
		expect(await screen.findByRole('option', { name: 'Zeta' })).toBeInTheDocument();
		expect(search).toHaveBeenCalledWith('z');
	});
});
