import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { CommandRegistry } from '$lib/commands/registry.svelte';
import type { PreferencesStore } from '$lib/stores/preferences.svelte';
import FilesSystem from './FilesSystem.svelte';
import { FilesStore, RECENT_PATH_LIMIT } from './files-store.svelte';
import OverviewSystem from './OverviewSystem.svelte';

const COMMANDS_CONTEXT = 'callfs.commands';

const mocks = vi.hoisted(() => ({
	goto: vi.fn(),
	page: {
		route: { id: '/(console)/files/[...path]' as string | null },
		params: {} as Record<string, string | undefined>,
		url: new URL('http://127.0.0.1/ui/files')
	},
	session: { isAdmin: true },
	preferences: {
		value: { files: { view: 'list', hidden: false, pinned: [] as string[], recent: [] as string[] } },
		update: vi.fn()
	}
}));

vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$app/state', () => ({ page: mocks.page }));
vi.mock('$lib/stores/session.svelte', () => ({ session: mocks.session }));
vi.mock('$lib/stores/preferences.svelte', () => ({ preferences: mocks.preferences }));
vi.mock('../planets/overview/OverviewPlanet.svelte', () => ({
	default: (anchor: ChildNode) => {
		const node = document.createElement('p');
		node.textContent = 'Overview planet';
		anchor.before(node);
	}
}));

function fakeRegistry(): { registry: CommandRegistry; register: ReturnType<typeof vi.fn> } {
	const register = vi.fn(() => () => undefined);
	return { registry: { register } as unknown as CommandRegistry, register };
}

interface RegisteredCommand {
	id: string;
	shortcut?: string[];
	run: () => unknown;
}

function commandById(register: ReturnType<typeof vi.fn>, id: string): RegisteredCommand {
	const commands = register.mock.calls.flatMap((call) => call[0] as RegisteredCommand[]);
	const command = commands.find((item) => id === item.id);
	expect(command).toBeDefined();
	return command as RegisteredCommand;
}

function childSnippet(text: string) {
	return createRawSnippet(() => ({ render: () => `<p>${text}</p>` }));
}

function sinkFor(initial: { view?: 'list' | 'grid'; hidden?: boolean; pinned?: string[]; recent?: string[] } = {}) {
	const files = {
		view: initial.view ?? 'list',
		hidden: initial.hidden ?? false,
		pinned: initial.pinned ?? [],
		recent: initial.recent ?? []
	};
	return {
		value: { files },
		update: vi.fn((patch: { files?: Record<string, unknown> }) => {
			Object.assign(files, patch.files);
		})
	} as unknown as Pick<PreferencesStore, 'value' | 'update'> & { update: ReturnType<typeof vi.fn> };
}

beforeEach(() => {
	mocks.goto.mockReset();
	mocks.preferences.update.mockReset();
	mocks.preferences.value.files.recent = [];
	mocks.preferences.value.files.pinned = [];
	mocks.preferences.value.files.view = 'list';
	mocks.preferences.value.files.hidden = false;
	mocks.session.isAdmin = true;
	mocks.page.route.id = '/(console)/files/[...path]';
	mocks.page.params = {};
});

afterEach(() => {
	cleanup();
});

describe('OverviewSystem', () => {
	it('renders the overview planet inside its wrapper', () => {
		const { registry } = fakeRegistry();
		render(OverviewSystem, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		expect(screen.getByText('Overview planet')).toBeInTheDocument();
	});

	it('registers Go to Overview on g o and navigates to the root', async () => {
		const { registry, register } = fakeRegistry();
		render(OverviewSystem, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		const command = commandById(register, 'overview.go');
		expect(command.shortcut).toEqual(['g', 'o']);
		expect(command.run()).toBeUndefined();
		expect(mocks.goto).toHaveBeenCalledWith('/ui/');
	});

	it('renders without a command registry', () => {
		render(OverviewSystem);
		expect(screen.getByText('Overview planet')).toBeInTheDocument();
	});
});

describe('FilesSystem', () => {
	it('shows every sub-nav tab to an admin', () => {
		const { registry } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Body') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		const tabs = screen.getAllByRole('tab');
		expect(tabs.map((tab) => tab.textContent?.trim())).toEqual(['Browse', 'Search', 'Flat list', 'Transfers', 'Trash']);
	});

	it('hides the admin-only Flat list tab from members', () => {
		mocks.session.isAdmin = false;
		const { registry } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Body') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		expect(screen.queryByRole('tab', { name: 'Flat list' })).toBeNull();
		expect(screen.getByRole('tab', { name: 'Trash' })).toBeInTheDocument();
	});

	it('marks the tab of the current route as selected', () => {
		mocks.page.route.id = '/(console)/files/search';
		const { registry } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Body') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		expect(screen.getByRole('tab', { name: 'Search' }).getAttribute('aria-selected')).toBe('true');
		expect(screen.getByRole('tab', { name: 'Browse' }).getAttribute('aria-selected')).toBe('false');
	});

	it('renders the page content snippet', () => {
		const { registry } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Page content') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		expect(screen.getByText('Page content')).toBeInTheDocument();
	});

	it('remembers the browsed folder in recent paths', async () => {
		mocks.page.params = { path: 'team/reports' };
		const { registry } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Body') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		expect(mocks.preferences.update).toHaveBeenCalledWith({ files: { recent: ['/team/reports'] } });
	});

	it('does not remember the root folder', async () => {
		mocks.page.params = { path: '' };
		const { registry } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Body') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		expect(mocks.preferences.update).not.toHaveBeenCalled();
	});

	it('registers Go to Files and Go to Transfers and navigates to them', async () => {
		const { registry, register } = fakeRegistry();
		render(FilesSystem, { props: { children: childSnippet('Body') }, context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		const files = commandById(register, 'files.go');
		expect(files.shortcut).toEqual(['g', 'f']);
		files.run();
		expect(mocks.goto).toHaveBeenLastCalledWith('/ui/files');
		const transfers = commandById(register, 'files.go.transfers');
		expect(transfers.shortcut).toEqual(['g', 't']);
		transfers.run();
		expect(mocks.goto).toHaveBeenLastCalledWith('/ui/files/transfers');
	});
});

describe('FilesStore', () => {
	it('moves a remembered path to the front without duplicates', () => {
		const sink = sinkFor({ recent: ['/a', '/b'] });
		const store = new FilesStore(sink);
		store.remember('/b');
		expect(store.recent).toEqual(['/b', '/a']);
	});

	it('keeps at most RECENT_PATH_LIMIT recent paths', () => {
		const sink = sinkFor({ recent: Array.from({ length: RECENT_PATH_LIMIT }, (_, index) => `/old-${index}`) });
		const store = new FilesStore(sink);
		store.remember('/new');
		expect(store.recent.length).toBe(RECENT_PATH_LIMIT);
		expect(store.recent[0]).toBe('/new');
		expect(store.recent).not.toContain(`/old-${RECENT_PATH_LIMIT - 1}`);
	});

	it('does not write when the path is already the most recent', () => {
		const sink = sinkFor({ recent: ['/a'] });
		const store = new FilesStore(sink);
		store.remember('/a');
		expect(sink.update).not.toHaveBeenCalled();
	});

	it('pins a path once and unpins it', () => {
		const sink = sinkFor({ pinned: ['/a'] });
		const store = new FilesStore(sink);
		store.pin('/a');
		expect(sink.update).not.toHaveBeenCalled();
		store.pin('/b');
		expect(store.pinned).toEqual(['/a', '/b']);
		store.unpin('/a');
		expect(store.pinned).toEqual(['/b']);
	});

	it('stores the view and toggles hidden files through preferences', () => {
		const sink = sinkFor();
		const store = new FilesStore(sink);
		store.setView('grid');
		expect(store.view).toBe('grid');
		expect(store.hidden).toBe(false);
		store.toggleHidden();
		expect(store.hidden).toBe(true);
	});

});
