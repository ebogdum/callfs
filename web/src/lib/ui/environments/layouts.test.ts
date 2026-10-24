import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import DashboardGrid from './DashboardGrid.svelte';
import type { DashboardPanel } from './DashboardGrid.svelte';
import SectionNav from './SectionNav.svelte';
import type { SectionNavGroup } from './SectionNav.svelte';
import SettingsLayout from './SettingsLayout.svelte';

function text(value: string) {
	return createRawSnippet(() => ({ render: () => `<p>${value}</p>` }));
}

function panel(
	id: string,
	title: string,
	span: DashboardPanel['span'] = 6,
	rows: DashboardPanel['rows'] = 1,
	content = text(`${title} body`)
): DashboardPanel {
	return { id, title, span, rows, content };
}

function headingTitles(): string[] {
	return screen.getAllByRole('heading', { level: 2 }).map((heading) => heading.textContent ?? '');
}

const navGroups: SectionNavGroup[] = [
	{
		label: 'Server',
		items: [
			{ id: 'network', label: 'Network', href: '/ui/settings/network', icon: 'network' },
			{ id: 'limits', label: 'Limits and timeouts', href: '/ui/settings/limits', icon: 'limit', badge: '2' }
		]
	}
];

afterEach(() => {
	cleanup();
});

describe('SectionNav', () => {
	it('renders a named navigation landmark with the group caption and links', () => {
		render(SectionNav, { props: { groups: navGroups, current: 'network', label: 'Settings sections' } });
		expect(screen.getByRole('navigation', { name: 'Settings sections' })).toBeInTheDocument();
		expect(screen.getByText('Server')).toBeInTheDocument();
		expect(screen.getByRole('link', { name: 'Network' })).toHaveAttribute('href', '/ui/settings/network');
	});

	it('marks the current item as the page and selected', () => {
		render(SectionNav, { props: { groups: navGroups, current: 'network', label: 'Settings sections' } });
		const current = screen.getByRole('link', { name: 'Network' });
		expect(current).toHaveAttribute('aria-current', 'page');
		expect(current).toHaveAttribute('data-state', 'selected');
		expect(screen.getByRole('link', { name: /Limits and timeouts/ })).not.toHaveAttribute('aria-current');
	});

	it('shows the badge text inside the item name', () => {
		render(SectionNav, { props: { groups: navGroups, current: 'network', label: 'Settings sections' } });
		expect(screen.getByRole('link', { name: /Limits and timeouts/ })).toHaveTextContent('2');
	});
});

describe('DashboardGrid', () => {
	const threePanels = [
		panel('traffic', 'Traffic', 8, 2),
		panel('errors', 'Errors', 4),
		panel('locks', 'Locks', 12)
	];

	it('renders panels in the order list and skips unknown ids', () => {
		render(DashboardGrid, {
			props: { panels: threePanels, order: ['locks', 'missing', 'traffic'], hidden: [] }
		});
		expect(headingTitles()).toEqual(['Locks', 'Traffic', 'Errors']);
	});

	it('applies the span and row classes of each panel', () => {
		render(DashboardGrid, { props: { panels: threePanels, order: [], hidden: [] } });
		const traffic = screen.getByRole('region', { name: 'Traffic' });
		expect(traffic).toHaveClass('span-8', 'rows-2');
		expect(screen.getByRole('region', { name: 'Errors' })).toHaveClass('span-4', 'rows-1');
	});

	it('renders panel content without edit controls when not editable', () => {
		render(DashboardGrid, { props: { panels: threePanels, order: [], hidden: [] } });
		expect(screen.getByText('Traffic body')).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: /^Move / })).not.toBeInTheDocument();
		expect(screen.queryByRole('button', { name: /^Hide:/ })).not.toBeInTheDocument();
	});

	it('shows Inspect data only when an inspect handler is given and calls it with the panel id', async () => {
		const oninspect = vi.fn();
		render(DashboardGrid, {
			props: { panels: threePanels, order: [], hidden: [], oninspect }
		});
		await fireEvent.click(within(screen.getByRole('region', { name: 'Errors' })).getByRole('button', { name: 'Inspect data' }));
		expect(oninspect).toHaveBeenCalledWith('errors');
	});

	it('moves a panel later and earlier and disables the edge buttons', async () => {
		render(DashboardGrid, {
			props: { panels: threePanels, order: [], hidden: [], editable: true }
		});
		expect(screen.getByRole('button', { name: 'Move earlier: Traffic' })).toBeDisabled();
		expect(screen.getByRole('button', { name: 'Move later: Locks' })).toBeDisabled();
		await fireEvent.click(screen.getByRole('button', { name: 'Move later: Traffic' }));
		expect(headingTitles()).toEqual(['Errors', 'Traffic', 'Locks']);
		await fireEvent.click(screen.getByRole('button', { name: 'Move earlier: Traffic' }));
		expect(headingTitles()).toEqual(['Traffic', 'Errors', 'Locks']);
	});

	it('hides a panel into the hidden list and restores it with Show', async () => {
		render(DashboardGrid, {
			props: { panels: threePanels, order: [], hidden: [], editable: true }
		});
		await fireEvent.click(screen.getByRole('button', { name: 'Hide: Errors' }));
		expect(headingTitles()).toEqual(['Traffic', 'Locks', 'Hidden panels']);
		expect(screen.getByText('Hidden panels')).toBeInTheDocument();
		await fireEvent.click(screen.getByRole('button', { name: 'Show Errors' }));
		expect(headingTitles()).toEqual(['Traffic', 'Errors', 'Locks']);
	});

	it('keeps hidden panels out of the grid and shows no hidden list when not editable', () => {
		render(DashboardGrid, { props: { panels: threePanels, order: [], hidden: ['errors'] } });
		expect(screen.queryByRole('region', { name: 'Errors' })).not.toBeInTheDocument();
		expect(screen.queryByText('Hidden panels')).not.toBeInTheDocument();
	});

	it('expands a panel with its Full screen button and collapses it again', async () => {
		render(DashboardGrid, { props: { panels: threePanels, order: [], hidden: [] } });
		const traffic = screen.getByRole('region', { name: 'Traffic' });
		await fireEvent.click(within(traffic).getByRole('button', { name: 'Full screen' }));
		expect(traffic).toHaveAttribute('data-state', 'expanded');
		expect(within(traffic).getByRole('button', { name: 'Exit full screen' })).toBeInTheDocument();
		await fireEvent.click(within(traffic).getByRole('button', { name: 'Exit full screen' }));
		expect(traffic).not.toHaveAttribute('data-state', 'expanded');
	});

	it('toggles full screen with the v key while a panel has focus', async () => {
		render(DashboardGrid, { props: { panels: threePanels, order: [], hidden: [] } });
		const traffic = screen.getByRole('region', { name: 'Traffic' });
		await fireEvent.keyDown(traffic, { key: 'v' });
		expect(traffic).toHaveAttribute('data-state', 'expanded');
		await fireEvent.keyDown(traffic, { key: 'v' });
		expect(traffic).not.toHaveAttribute('data-state', 'expanded');
	});

	it('does not toggle full screen while typing in a field inside the panel', async () => {
		const withInput = [panel('traffic', 'Traffic', 6, 1, createRawSnippet(() => ({ render: () => '<input type="text" aria-label="Filter" />' })))];
		render(DashboardGrid, { props: { panels: withInput, order: [], hidden: [] } });
		const traffic = screen.getByRole('region', { name: 'Traffic' });
		await fireEvent.keyDown(screen.getByRole('textbox', { name: 'Filter' }), { key: 'v' });
		expect(traffic).not.toHaveAttribute('data-state', 'expanded');
	});
});

describe('SettingsLayout', () => {
	const baseProps = {
		nav: navGroups,
		current: 'network',
		search: '',
		children: text('Network settings body')
	};

	it('renders the section nav, the labelled search field and the content', () => {
		render(SettingsLayout, { props: baseProps });
		expect(screen.getByRole('navigation', { name: 'Settings sections' })).toBeInTheDocument();
		expect(screen.getByRole('searchbox', { name: 'Search settings' })).toBeInTheDocument();
		expect(screen.getByText('Network settings body')).toBeInTheDocument();
	});

	it('shows the typed search text in the field', async () => {
		render(SettingsLayout, { props: baseProps });
		const search = screen.getByRole('searchbox', { name: 'Search settings' });
		await fireEvent.input(search, { target: { value: 'log' } });
		expect(search).toHaveValue('log');
	});

	it('renders the save bar only when a save bar is given', () => {
		const { unmount } = render(SettingsLayout, { props: baseProps });
		expect(screen.queryByText('3 changes')).not.toBeInTheDocument();
		unmount();
		render(SettingsLayout, {
			props: { ...baseProps, saveBar: text('3 changes') }
		});
		expect(screen.getByText('3 changes')).toBeInTheDocument();
	});
});
