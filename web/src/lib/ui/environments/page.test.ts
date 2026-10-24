import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, describe, expect, it } from 'vitest';
import PageFrame from './PageFrame.svelte';
import SplitView from './SplitView.svelte';
import OverlayHost from './OverlayHost.svelte';

function text(value: string) {
	return createRawSnippet(() => ({ render: () => `<p>${value}</p>` }));
}

function button(label: string) {
	return createRawSnippet(() => ({ render: () => `<button type="button">${label}</button>` }));
}

function stubMatchMedia(matching: (query: string) => boolean): void {
	window.matchMedia = (query: string) =>
		({
			matches: matching(query),
			media: query,
			onchange: null,
			addEventListener: () => undefined,
			removeEventListener: () => undefined,
			addListener: () => undefined,
			removeListener: () => undefined,
			dispatchEvent: () => false
		}) as unknown as MediaQueryList;
}

afterEach(() => {
	cleanup();
	Reflect.deleteProperty(window, 'matchMedia');
});

describe('PageFrame', () => {
	const baseProps = {
		title: 'Files',
		job: 'Browse and share',
		children: text('Body content')
	};

	it('renders the title as the level-1 heading with the job line', () => {
		render(PageFrame, { props: baseProps });
		expect(screen.getByRole('heading', { level: 1, name: 'Files' })).toBeInTheDocument();
		expect(screen.getByText('Browse and share')).toBeInTheDocument();
		expect(screen.getByText('Body content')).toBeInTheDocument();
	});

	it('sets the document title to the page title with the product suffix', () => {
		render(PageFrame, { props: baseProps });
		expect(document.title).toBe('Files · callfs');
	});

	it('focuses the heading on mount and again when the title changes', async () => {
		const view = render(PageFrame, { props: baseProps });
		await tick();
		expect(screen.getByRole('heading', { level: 1 })).toHaveFocus();
		await view.rerender({ ...baseProps, title: 'Trash' });
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Trash' })).toHaveFocus();
	});

	it('places the primary action after the secondary action', () => {
		render(PageFrame, {
			props: { ...baseProps, secondary: button('Cancel'), primary: button('Save') }
		});
		const secondary = screen.getByRole('button', { name: 'Cancel' });
		const primary = screen.getByRole('button', { name: 'Save' });
		expect(secondary.compareDocumentPosition(primary) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
	});

	it('renders the aside landmark only when an aside is provided', () => {
		const { unmount } = render(PageFrame, { props: baseProps });
		expect(screen.queryByRole('complementary')).toBeNull();
		unmount();
		render(PageFrame, { props: { ...baseProps, aside: text('Inspector content') } });
		expect(screen.getByRole('complementary')).toBeInTheDocument();
		expect(screen.getByText('Inspector content')).toBeInTheDocument();
	});
});

describe('SplitView', () => {
	const splitProps = {
		list: text('List body'),
		detail: text('Detail body'),
		label: 'Files',
		detailWidth: 400
	};

	it('docks a resizable detail with a separator at 1280px and wider', () => {
		stubMatchMedia(() => true);
		const { container } = render(SplitView, { props: { ...splitProps, detailOpen: true } });
		expect(screen.getByRole('separator')).toBeInTheDocument();
		expect(screen.getByText('Detail body')).toBeInTheDocument();
		expect(container.querySelector('[data-mode]')).toHaveAttribute('data-mode', 'docked');
	});

	it('shows the detail as a drawer between 840px and 1279px without a separator', () => {
		stubMatchMedia((query) => query.includes('840px'));
		const { container } = render(SplitView, { props: { ...splitProps, detailOpen: true } });
		expect(screen.queryByRole('separator', { name: 'Files' })).toBeNull();
		expect(screen.getByText('Detail body')).toBeInTheDocument();
		expect(container.querySelector('[data-mode]')).toHaveAttribute('data-mode', 'drawer');
	});

	it('replaces the list with the detail below 840px and keeps the list mounted and inert', () => {
		stubMatchMedia(() => false);
		const { container } = render(SplitView, { props: { ...splitProps, detailOpen: true } });
		const list = container.querySelector('.list');
		expect((list as HTMLElement).inert).toBe(true);
		expect(list).toHaveClass('covered');
		expect(screen.getByText('List body')).toBeInTheDocument();
		expect(container.querySelector('.detail.covering')).not.toBeNull();
	});

	it('closes the covering detail on Escape', async () => {
		stubMatchMedia(() => false);
		const { container } = render(SplitView, { props: { ...splitProps, detailOpen: true } });
		const detail = container.querySelector('.detail.covering');
		expect(detail).not.toBeNull();
		await fireEvent.keyDown(detail as HTMLElement, { key: 'Escape' });
		await tick();
		expect(screen.queryByText('Detail body')).toBeNull();
		expect((container.querySelector('.list') as HTMLElement).inert).toBe(false);
	});

	it('shows only the list when the detail is closed', () => {
		stubMatchMedia(() => false);
		const { container } = render(SplitView, { props: { ...splitProps, detailOpen: false } });
		expect(screen.getByText('List body')).toBeInTheDocument();
		expect(screen.queryByText('Detail body')).toBeNull();
		expect((container.querySelector('.list') as HTMLElement).inert).toBe(false);
	});
});

describe('OverlayHost', () => {
	it('renders one portal target per layer in z-order', () => {
		const { container } = render(OverlayHost);
		const layers = Array.from(container.querySelectorAll('[data-layer]')).map((element) =>
			element.getAttribute('data-layer')
		);
		expect(layers).toEqual(['dropdown', 'scrim', 'drawer', 'dialog', 'palette', 'popover-top', 'toast', 'tooltip']);
		expect(container.querySelector('#overlay-dialog')).not.toBeNull();
	});
});
