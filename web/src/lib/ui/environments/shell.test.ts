import { cleanup, fireEvent, render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import AppSidebar, { type SidebarGroup } from './AppSidebar.svelte';
import BottomNav, { type BottomNavGroup } from './BottomNav.svelte';
import Dock from './Dock.svelte';
import TopBar from './TopBar.svelte';

afterEach(() => {
	cleanup();
});

const text = (value: string) => createRawSnippet(() => ({ render: () => `<span>${value}</span>` }));

const sidebarGroups: SidebarGroup[] = [
	{
		id: 'work',
		label: 'Work',
		items: [
			{ id: 'overview', label: 'Overview', href: '/', icon: 'home' },
			{ id: 'files', label: 'Files', href: '/files', icon: 'folder' }
		]
	},
	{
		id: 'configure',
		label: 'Configure',
		items: [{ id: 'settings', label: 'Settings', href: '/settings/network', icon: 'settings' }]
	}
];

describe('AppSidebar', () => {
	it('names the navigation landmark and labels each group', () => {
		const { getByRole } = render(AppSidebar, {
			props: { groups: sidebarGroups, current: 'files', footer: text('Footer') }
		});
		expect(getByRole('navigation', { name: 'Main' })).toBeTruthy();
		expect(getByRole('heading', { name: 'Work' })).toBeTruthy();
		expect(getByRole('heading', { name: 'Configure' })).toBeTruthy();
	});

	it('marks only the current item with aria-current page', () => {
		const { getByRole } = render(AppSidebar, {
			props: { groups: sidebarGroups, current: 'files', footer: text('Footer') }
		});
		expect(getByRole('link', { name: 'Files' }).getAttribute('aria-current')).toBe('page');
		expect(getByRole('link', { name: 'Overview' }).getAttribute('aria-current')).toBe(null);
	});

	it('keeps the item label in the accessible name when collapsed to the rail', () => {
		const { getByRole } = render(AppSidebar, {
			props: { groups: sidebarGroups, current: 'files', collapsed: true, footer: text('Footer') }
		});
		expect(getByRole('link', { name: 'Overview' })).toBeTruthy();
	});

	it('names the collapse button with the expanded state', () => {
		const { getByRole } = render(AppSidebar, {
			props: { groups: sidebarGroups, current: 'files', footer: text('Footer') }
		});
		const toggle = getByRole('button', { name: 'Collapse sidebar' });
		expect(toggle.getAttribute('aria-expanded')).toBe('true');
	});

	it('collapses to the rail when the toggle is pressed', async () => {
		const { getByRole, container } = render(AppSidebar, {
			props: { groups: sidebarGroups, current: 'files', footer: text('Footer') }
		});
		await fireEvent.click(getByRole('button', { name: 'Collapse sidebar' }));
		expect(getByRole('button', { name: 'Expand sidebar' }).getAttribute('aria-expanded')).toBe('false');
		expect(container.querySelector('nav')?.getAttribute('data-state')).toBe('collapsed');
	});
});

const moreGroups: BottomNavGroup[] = [
	{
		id: 'operate',
		label: 'Operate',
		items: [{ id: 'activity', label: 'Activity', href: '/activity', icon: 'activity' }]
	}
];

const tabItems = [
	{ id: 'overview', label: 'Overview', href: '/', icon: 'home' as const },
	{ id: 'files', label: 'Files', href: '/files', icon: 'folder' as const }
];

describe('BottomNav', () => {
	it('names the mobile navigation landmark and links each tab', () => {
		const { getByRole } = render(BottomNav, { props: { items: tabItems, current: 'files', more: [] } });
		expect(getByRole('navigation', { name: 'Main' })).toBeTruthy();
		expect(getByRole('link', { name: 'Overview' })).toBeTruthy();
		expect(getByRole('link', { name: 'Files' }).getAttribute('aria-current')).toBe('page');
	});

	it('shows the More trigger only when there are more pages', () => {
		const withMore = render(BottomNav, { props: { items: tabItems, current: 'files', more: moreGroups } });
		expect(withMore.getByRole('button', { name: 'More' })).toBeTruthy();
		withMore.unmount();
		const withoutMore = render(BottomNav, { props: { items: tabItems, current: 'files', more: [] } });
		expect(withoutMore.queryByRole('button', { name: 'More' })).toBe(null);
	});
});

describe('TopBar', () => {
	it('renders the banner landmark with every slot', () => {
		const { getByRole, getByText } = render(TopBar, {
			props: {
				title: text('Files'),
				search: text('Search files'),
				status: text('Live'),
				actions: text('Jobs'),
				account: text('Account')
			}
		});
		expect(getByRole('banner')).toBeTruthy();
		expect(getByText('Files')).toBeTruthy();
		expect(getByText('Search files')).toBeTruthy();
		expect(getByText('Live')).toBeTruthy();
		expect(getByText('Jobs')).toBeTruthy();
		expect(getByText('Account')).toBeTruthy();
	});
});

describe('Dock', () => {
	it('renders its children in the bottom region', () => {
		const { getByText } = render(Dock, { props: { children: text('2 transfers running') } });
		expect(getByText('2 transfers running')).toBeTruthy();
	});
});
