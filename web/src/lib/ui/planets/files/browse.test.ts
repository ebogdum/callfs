import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { goto } from '$app/navigation';
import { base } from '$app/paths';
import { audit } from '$lib/api/admin/telemetry';
import { instances } from '$lib/api/admin/topology';
import { locks } from '$lib/api/admin/runtime';
import { getSplitFile, repair, verify } from '$lib/api/erasure';
import { listDir, readRange, stat } from '$lib/api/files';
import { listLinks } from '$lib/api/links';
import { permissionCheck } from '$lib/api/search';
import type { DirPage, FileEntry } from '$lib/api/types';
import { CommandRegistry } from '$lib/commands/registry.svelte';
import { entryFor } from '$lib/data/cache';
import { t } from '$lib/copy/t';
import { session } from '$lib/stores/session.svelte';
import BrowsePlanet from './BrowsePlanet.svelte';
import { BrowseState, hrefFor, pathFromRoute, type BrowseDefaults } from './browse-state.svelte';

const route = vi.hoisted(() => ({
	url: new URL('http://127.0.0.1/ui/files/team'),
	params: { path: 'team' } as Record<string, string>
}));

vi.mock('$app/state', () => ({ page: route }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(() => Promise.resolve()) }));
vi.mock('$lib/api/files', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/files')>()),
	listDir: vi.fn(),
	stat: vi.fn(),
	readRange: vi.fn()
}));
vi.mock('$lib/api/search', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/search')>()),
	permissionCheck: vi.fn()
}));
vi.mock('$lib/api/links', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/links')>()),
	listLinks: vi.fn()
}));
vi.mock('$lib/api/erasure', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/erasure')>()),
	getSplitFile: vi.fn(),
	verify: vi.fn(),
	repair: vi.fn()
}));
vi.mock('$lib/api/admin/telemetry', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/admin/telemetry')>()),
	audit: vi.fn()
}));
vi.mock('$lib/api/admin/topology', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/admin/topology')>()),
	instances: vi.fn()
}));
vi.mock('$lib/api/admin/runtime', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api/admin/runtime')>()),
	locks: vi.fn()
}));

const COMMANDS_CONTEXT = 'callfs.commands';
const DEFAULTS: BrowseDefaults = { view: 'list', hidden: false, sort: { key: 'name', dir: 'asc' } };

const response = <T,>(data: T) => ({ data, status: 200, headers: new Headers(), requestId: null, etag: null });
const success = <T,>(data: T) => ({ ok: true as const, value: response(data) });

const fileEntry = (path: string, overrides: Partial<FileEntry> = {}): FileEntry => ({
	name: path.split('/').pop() ?? '',
	path,
	type: 'file',
	size: 2048,
	mode: '0644',
	owner: 'alice',
	mtime: '2026-10-01T00:00:00Z',
	etag: 'abc123',
	storage: 'localfs',
	instance_id: 'callfs-1',
	erasure_coded: false,
	locked: false,
	...overrides
});

const dirPage = (items: FileEntry[], overrides: Partial<DirPage> = {}): DirPage => ({
	path: '/team',
	type: 'directory',
	count: items.length,
	total: items.length,
	items,
	next_cursor: null,
	partial: false,
	...overrides
});

const memberSession = {
	user_id: 'alice',
	is_admin: false,
	auth_method: 'session' as const,
	session_expires_at: null,
	csrf_token: null,
	permissions_model: 'owner' as const
};

describe('hrefFor and pathFromRoute', () => {
	it('maps the root to /files and encodes each segment of a nested path', () => {
		expect(hrefFor('/')).toBe('/files');
		expect(hrefFor('/team/report 1.pdf')).toBe('/files/team/report%201.pdf');
	});

	it('turns the route parameter into an absolute path with the root for an empty parameter', () => {
		expect(pathFromRoute('team/docs')).toBe('/team/docs');
		expect(pathFromRoute(undefined)).toBe('/');
	});
});

describe('BrowseState', () => {
	it('hydrates every URL parameter and writes the same parameters back', () => {
		const state = new BrowseState(vi.fn());
		state.hydrate(
			'/team',
			new URLSearchParams(
				'q=rep&view=grid&sort=size&order=desc&hidden=true&inspect=/team/a.pdf&tab=links&preview=/team/a.pdf&sel=/team/a.pdf&sel=/team/b.pdf'
			),
			DEFAULTS
		);
		expect(state.path).toBe('/team');
		expect(state.q).toBe('rep');
		expect(state.view).toBe('grid');
		expect(state.sort).toEqual({ key: 'size', dir: 'desc' });
		expect(state.showHidden).toBe(true);
		expect(state.inspect).toBe('/team/a.pdf');
		expect(state.tab).toBe('links');
		expect(state.preview).toBe('/team/a.pdf');
		expect([...state.selection.ids]).toEqual(['/team/a.pdf', '/team/b.pdf']);
		const written = new URLSearchParams(state.search().slice(1));
		expect(written.get('tab')).toBe('links');
		expect(written.getAll('sel')).toEqual(['/team/a.pdf', '/team/b.pdf']);
	});

	it('falls back to defaults for unknown values and absent hidden flag', () => {
		const state = new BrowseState(vi.fn());
		state.hydrate('/', new URLSearchParams('view=table&tab=bogus&sort=owner'), { ...DEFAULTS, hidden: true });
		expect(state.view).toBe('list');
		expect(state.tab).toBe('details');
		expect(state.sort).toEqual({ key: 'name', dir: 'asc' });
		expect(state.showHidden).toBe(true);
		expect(state.selection).toEqual({ mode: 'multi', ids: new Set(), allMatching: false });
	});

	it('navigates with view and sort but drops the filter, selection and inspector', () => {
		const navigateTo = vi.fn();
		const state = new BrowseState(navigateTo);
		state.view = 'grid';
		state.q = 'report';
		state.inspect = '/team/a.pdf';
		state.selection = { mode: 'multi', ids: new Set(['/team/a.pdf']), allMatching: false };
		state.navigate('/team/docs');
		const href = navigateTo.mock.calls[0]?.[0] as string;
		expect(href.startsWith('/files/team/docs?')).toBe(true);
		const params = new URLSearchParams(href.split('?')[1]);
		expect(params.get('view')).toBe('grid');
		expect(params.has('q')).toBe(false);
		expect(params.has('inspect')).toBe(false);
		expect(params.has('sel')).toBe(false);
	});
});

describe('BrowsePlanet', () => {
	beforeEach(() => {
		vi.mocked(goto).mockClear();
		vi.mocked(listDir).mockReset();
		vi.mocked(stat).mockReset();
		vi.mocked(readRange).mockReset();
		vi.mocked(listLinks).mockReset();
		vi.mocked(permissionCheck).mockReset();
		vi.mocked(getSplitFile).mockReset();
		vi.mocked(verify).mockReset();
		vi.mocked(repair).mockReset();
		vi.mocked(audit).mockReset();
		vi.mocked(instances).mockReset();
		vi.mocked(locks).mockReset();
		const cached = entryFor(['dir', '/team', 'name', 'asc', false]);
		cached.data = undefined;
		cached.error = null;
		cached.updatedAt = null;
		cached.inFlight = null;
		route.url = new URL('http://127.0.0.1/ui/files/team');
		route.params = { path: 'team' };
		session.me = memberSession;
	});

	it('renders the folder name as the only level one heading and lists the folder', async () => {
		vi.mocked(listDir).mockResolvedValue(
			success(dirPage([fileEntry('/team/docs', { name: 'docs', type: 'directory', size: 0 }), fileEntry('/team/report.pdf')])) as never
		);
		render(BrowsePlanet);
		expect((await screen.findAllByText('report.pdf')).length).toBeGreaterThan(0);
		expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
		expect(screen.getByRole('heading', { level: 1, name: 'team' })).toBeTruthy();
		expect(listDir).toHaveBeenCalledWith('/team', expect.objectContaining({ limit: 200, includeHidden: false }));
	});

	it('shows the empty folder copy when the listing has no items', async () => {
		vi.mocked(listDir).mockResolvedValue(success(dirPage([])) as never);
		render(BrowsePlanet);
		expect(await screen.findByText(t('browse.empty.folder.title'))).toBeTruthy();
		expect(screen.getByText(t('browse.empty.folder.body'))).toBeTruthy();
	});

	it('shows the error state when the listing fails and no rows are loaded', async () => {
		vi.mocked(listDir).mockResolvedValue({ ok: false, error: { kind: 'network' } } as never);
		render(BrowsePlanet);
		expect(await screen.findByText(/cannot reach callfs/i)).toBeTruthy();
	});

	it('moves to the parent folder when Backspace is pressed outside a field', async () => {
		vi.mocked(listDir).mockResolvedValue(success(dirPage([fileEntry('/team/report.pdf')])) as never);
		render(BrowsePlanet);
		await screen.findAllByText('report.pdf');
		await fireEvent.keyDown(screen.getByRole('heading', { level: 1 }), { key: 'Backspace' });
		const hrefs = vi.mocked(goto).mock.calls.map(([href]) => String(href));
		expect(hrefs.some((href) => href.startsWith(`${base}/files?view=list`))).toBe(true);
	});

	it('registers the Files commands with the command registry', async () => {
		vi.mocked(listDir).mockResolvedValue(success(dirPage([])) as never);
		const registry = new CommandRegistry({ context: () => ({ role: 'member', route: '/files', selectionCount: 0, capabilities: [] }) });
		render(BrowsePlanet, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		const ids = registry
			.list({ role: 'member', route: '/files', selectionCount: 0, capabilities: [] })
			.map((entry) => entry.command.id);
		expect(ids).toEqual(
			expect.arrayContaining([
				'files.browse.view',
				'files.browse.hidden',
				'files.browse.inspector',
				'files.browse.selectAll',
				'files.browse.refresh'
			])
		);
	});
});
