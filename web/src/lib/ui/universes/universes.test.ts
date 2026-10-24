import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import ConsoleUniverse from './ConsoleUniverse.svelte';
import WallUniverse from './WallUniverse.svelte';
import PrintUniverse from './PrintUniverse.svelte';

const state = vi.hoisted(() => ({
	session: { me: undefined as unknown, restored: true, isAdmin: false, restore: () => Promise.resolve() },
	capabilities: { status: 'success' as string, error: null as unknown, load: () => Promise.resolve() },
	preferences: { load: () => Promise.resolve() }
}));

vi.mock('$lib/stores/session.svelte', () => ({ session: state.session }));
vi.mock('$lib/stores/capabilities.svelte', () => ({ capabilities: state.capabilities }));
vi.mock('$lib/stores/preferences.svelte', () => ({ preferences: state.preferences }));
vi.mock('$lib/theme/controller.svelte', () => ({
	ThemeController: class {
		dispose(): void {}
	}
}));
vi.mock('$lib/ui/galaxies/AuthGalaxy.svelte', () => ({ default: () => undefined }));
vi.mock('$app/navigation', () => ({ goto: () => Promise.resolve() }));
vi.mock('$app/paths', () => ({ base: '' }));
vi.mock('$app/state', () => ({
	page: { url: { pathname: '/files', search: '', searchParams: new URLSearchParams() } }
}));

const childSnippet = (text: string) => createRawSnippet(() => ({ render: () => `<p>${text}</p>` }));

describe('ConsoleUniverse', () => {
	beforeEach(() => {
		state.session.me = undefined;
		state.session.restored = true;
		state.session.isAdmin = false;
		state.capabilities.status = 'success';
		state.capabilities.error = null;
	});

	afterEach(() => {
		cleanup();
	});

	it('shows the shell skeleton while the session is loading', () => {
		state.session.restored = false;
		render(ConsoleUniverse, { props: { children: childSnippet('console-child') } });
		expect(null === document.querySelector('[aria-busy="true"]')).toBe(false);
		expect(null === document.querySelector('p')).toBe(true);
	});

	it('renders no page content when the visitor is signed out', () => {
		state.session.me = null;
		render(ConsoleUniverse, { props: { children: childSnippet('console-child') } });
		expect(null === document.querySelector('p')).toBe(true);
	});

	it('renders children once the session is loaded and capabilities are ready', () => {
		state.session.me = {};
		render(ConsoleUniverse, { props: { children: childSnippet('console-child') } });
		expect(document.querySelector('p')?.textContent).toBe('console-child');
	});

	it('keeps the skeleton while capabilities are still loading', () => {
		state.session.me = {};
		state.capabilities.status = 'loading';
		render(ConsoleUniverse, { props: { children: childSnippet('console-child') } });
		expect(null === document.querySelector('p')).toBe(true);
		expect(null === document.querySelector('[aria-busy="true"]')).toBe(false);
	});
});

describe('WallUniverse', () => {
	afterEach(() => {
		cleanup();
	});

	it('renders the wall board inside the wall layout', () => {
		const { getByText } = render(WallUniverse, { props: { children: childSnippet('wall-board') } });
		expect(getByText('wall-board').tagName).toBe('P');
	});
});

describe('PrintUniverse', () => {
	afterEach(() => {
		cleanup();
	});

	it('renders the report inside the print layout', () => {
		const { getByText } = render(PrintUniverse, { props: { children: childSnippet('print-report') } });
		expect(getByText('print-report').tagName).toBe('P');
	});
});
