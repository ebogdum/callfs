import { cleanup, fireEvent, render } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Breadcrumbs from './Breadcrumbs.svelte';
import SegmentedControl from './SegmentedControl.svelte';
import Tabs from './Tabs.svelte';

afterEach(() => {
	cleanup();
});

const layoutOptions = [
	{ value: 'list', label: 'List', icon: 'list' as const },
	{ value: 'grid', label: 'Grid', icon: 'grid' as const }
];

describe('SegmentedControl', () => {
	it('names the group and shows each option with its visible label', () => {
		const { getByRole } = render(SegmentedControl, { props: { value: 'list', options: layoutOptions, label: 'Layout' } });
		expect(getByRole('group', { name: 'Layout' })).toBeTruthy();
		expect(getByRole('radio', { name: 'List' })).toBeTruthy();
		expect(getByRole('radio', { name: 'Grid' })).toBeTruthy();
	});

	it('marks only the selected option as pressed', () => {
		const { getByRole } = render(SegmentedControl, { props: { value: 'list', options: layoutOptions, label: 'Layout' } });
		expect(getByRole('radio', { name: 'List' }).getAttribute('aria-checked')).toBe('true');
		expect(getByRole('radio', { name: 'Grid' }).getAttribute('aria-checked')).toBe('false');
	});

	it('moves the selection when another option is clicked', async () => {
		const { getByRole } = render(SegmentedControl, { props: { value: 'list', options: layoutOptions, label: 'Layout' } });
		await fireEvent.click(getByRole('radio', { name: 'Grid' }));
		expect(getByRole('radio', { name: 'Grid' }).getAttribute('aria-checked')).toBe('true');
		expect(getByRole('radio', { name: 'List' }).getAttribute('aria-checked')).toBe('false');
	});

	it('keeps the selection when the selected option is clicked again', async () => {
		const { getByRole } = render(SegmentedControl, { props: { value: 'list', options: layoutOptions, label: 'Layout' } });
		await fireEvent.click(getByRole('radio', { name: 'List' }));
		expect(getByRole('radio', { name: 'List' }).getAttribute('aria-checked')).toBe('true');
	});

	it('moves focus to the next option with the arrow key', async () => {
		const { getByRole } = render(SegmentedControl, { props: { value: 'list', options: layoutOptions, label: 'Layout' } });
		const list = getByRole('radio', { name: 'List' });
		list.focus();
		await fireEvent.keyDown(list, { key: 'ArrowRight' });
		expect(document.activeElement).toBe(getByRole('radio', { name: 'Grid' }));
	});
});

const linkOptions = [
	{ id: 'active', label: 'Active', count: 1204 },
	{ id: 'done', label: 'Done', href: '/sharing/done' }
];

describe('Tabs', () => {
	it('names the tab list and marks the selected tab', () => {
		const { getByRole } = render(Tabs, { props: { value: 'active', items: linkOptions, label: 'Links' } });
		expect(getByRole('tablist', { name: 'Links' })).toBeTruthy();
		expect(getByRole('tab', { name: /Active/ }).getAttribute('aria-selected')).toBe('true');
		expect(getByRole('tab', { name: /Done/ }).getAttribute('aria-selected')).toBe('false');
	});

	it('shows the count as a whole number', () => {
		const { getByRole } = render(Tabs, { props: { value: 'active', items: linkOptions, label: 'Links' } });
		expect(getByRole('tab', { name: /Active/ }).textContent).toContain('1,204');
	});

	it('renders a tab with href as a link to that href', () => {
		const { getByRole } = render(Tabs, { props: { value: 'active', items: linkOptions, label: 'Links' } });
		const done = getByRole('tab', { name: /Done/ });
		expect(done.tagName).toBe('A');
		expect(done.getAttribute('href')).toBe('/sharing/done');
	});

	it('activates the next tab with the arrow key', async () => {
		const { getByRole } = render(Tabs, { props: { value: 'active', items: linkOptions, label: 'Links' } });
		const active = getByRole('tab', { name: /Active/ });
		active.focus();
		await fireEvent.keyDown(active, { key: 'ArrowRight' });
		expect(getByRole('tab', { name: /Done/ }).getAttribute('aria-selected')).toBe('true');
	});
});

const trail = [
	{ label: 'Files', href: '/files' },
	{ label: 'team', href: '/files/team' },
	{ label: 'reports', href: '/files/team/reports' }
];

const longTrail = [
	{ label: 'a', href: '/files/a' },
	{ label: 'b', href: '/files/a/b' },
	{ label: 'c', href: '/files/a/b/c' },
	{ label: 'd', href: '/files/a/b/c/d' },
	{ label: 'e', href: '/files/a/b/c/d/e' },
	{ label: 'f', href: '/files/a/b/c/d/e/f' }
];

describe('Breadcrumbs', () => {
	it('names the trail and marks the last segment as the current page', () => {
		const { getByRole, getByText } = render(Breadcrumbs, { props: { segments: trail, label: 'Location' } });
		expect(getByRole('navigation', { name: 'Location' })).toBeTruthy();
		expect(getByRole('link', { name: 'Files' }).getAttribute('href')).toBe('/files');
		expect(getByText('reports').getAttribute('aria-current')).toBe('page');
		expect(getByText('reports').tagName).not.toBe('A');
	});

	it('shows the first and last two segments and moves the middle into a menu beyond four', () => {
		const { getByRole, queryByRole } = render(Breadcrumbs, { props: { segments: longTrail, label: 'Location' } });
		expect(getByRole('link', { name: 'a' })).toBeTruthy();
		expect(getByRole('link', { name: 'e' })).toBeTruthy();
		expect(queryByRole('link', { name: 'b' })).toBeNull();
		expect(getByRole('button', { name: /More/ })).toBeTruthy();
	});

	it('shows every segment when there are four or fewer', () => {
		const { queryByRole } = render(Breadcrumbs, { props: { segments: trail, label: 'Location' } });
		expect(queryByRole('button', { name: /More/ })).toBeNull();
	});

	it('opens the path input when the empty trail area is clicked and editing is allowed', async () => {
		const { container } = render(Breadcrumbs, { props: { segments: trail, label: 'Location', editable: true } });
		const trailList = container.querySelector('ol') as HTMLElement;
		await fireEvent.click(trailList);
		expect(container.querySelector('input')).not.toBeNull();
	});

	it('does not open the path input when editing is not allowed', async () => {
		const { container } = render(Breadcrumbs, { props: { segments: trail, label: 'Location' } });
		const trailList = container.querySelector('ol') as HTMLElement;
		await fireEvent.click(trailList);
		expect(container.querySelector('input')).toBeNull();
	});

	it('opens the path input with Ctrl+L', async () => {
		const { container, getByRole } = render(Breadcrumbs, { props: { segments: trail, label: 'Location', editable: true } });
		await fireEvent.keyDown(getByRole('link', { name: 'team' }), { key: 'l', ctrlKey: true });
		expect(container.querySelector('input')).not.toBeNull();
	});

	it('returns to the trail when Escape is pressed in the path input', async () => {
		const { container } = render(Breadcrumbs, { props: { segments: trail, label: 'Location', editable: true } });
		const trailList = container.querySelector('ol') as HTMLElement;
		await fireEvent.click(trailList);
		const input = container.querySelector('input') as HTMLInputElement;
		await fireEvent.keyDown(input, { key: 'Escape' });
		expect(container.querySelector('ol')).not.toBeNull();
	});

	it('reports a valid typed path through onpath', async () => {
		const onpath = vi.fn();
		const { container } = render(Breadcrumbs, { props: { segments: trail, label: 'Location', editable: true, onpath } });
		const trailList = container.querySelector('ol') as HTMLElement;
		await fireEvent.click(trailList);
		const input = container.querySelector('input') as HTMLInputElement;
		await fireEvent.input(input, { target: { value: '/files/other' } });
		await fireEvent.keyDown(input, { key: 'Enter' });
		expect(onpath).toHaveBeenCalledWith('/files/other');
	});

	it('passes the dropped segment path to ondrop when droppable', async () => {
		const ondrop = vi.fn();
		const { getByRole } = render(Breadcrumbs, { props: { segments: trail, label: 'Location', droppable: true, ondrop } });
		await fireEvent.drop(getByRole('link', { name: 'team' }));
		expect(ondrop).toHaveBeenCalledWith('/files/team', expect.anything());
	});

	it('ignores drops when not droppable', async () => {
		const ondrop = vi.fn();
		const { getByRole } = render(Breadcrumbs, { props: { segments: trail, label: 'Location', ondrop } });
		await fireEvent.drop(getByRole('link', { name: 'team' }));
		expect(ondrop).not.toHaveBeenCalled();
	});
});
