import { afterEach, describe, expect, it, vi } from 'vitest';
import { shortcutDefinitions } from '$lib/shortcuts/keymap';
import { ShortcutManager } from '$lib/shortcuts/manager.svelte';
import { visibleNavItems } from '$lib/nav/model';
import { fuzzyMatch } from '../format/fuzzy';
import { CommandRegistry, parseCommandQuery, type Command, type CommandContext } from './registry.svelte';

const ctx: CommandContext = { role: 'admin', route: '/', selectionCount: 0, capabilities: [] };

const command = (overrides: Partial<Command> & Pick<Command, 'id' | 'title' | 'group'>): Command => ({
	keywords: [],
	run: vi.fn(),
	...overrides
});

const makeRegistry = (recent: string[] = [], persistRecent?: (ids: string[]) => void) =>
	new CommandRegistry({ context: () => ctx, recent, persistRecent });

const press = (key: string, init: KeyboardEventInit = {}, target: EventTarget = document.body): void => {
	target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init }));
};

let stopListening: (() => void) | null = null;

afterEach(() => {
	stopListening?.();
	stopListening = null;
	vi.useRealTimers();
});

describe('fuzzyMatch', () => {
	it('matches query characters in order and reports their indices', () => {
		expect(fuzzyMatch('fs', 'Files')?.indices).toEqual([0, 4]);
	});

	it('returns null when a query character is missing', () => {
		expect(fuzzyMatch('xq', 'Files')).toBeNull();
	});

	it('matches case-insensitively', () => {
		expect(fuzzyMatch('FIL', 'files')?.indices).toEqual([0, 1, 2]);
	});

	it('scores contiguous matches above scattered ones', () => {
		expect(fuzzyMatch('fil', 'files')?.score ?? 0).toBeGreaterThan(fuzzyMatch('fls', 'files')?.score ?? 0);
	});

	it('returns an empty match for a blank query', () => {
		expect(fuzzyMatch('  ', 'Files')).toEqual({ score: 0, indices: [] });
	});
});

describe('parseCommandQuery', () => {
	it('maps each prefix to its mode and strips it', () => {
		expect(parseCommandQuery('>open')).toEqual({ mode: 'actions', query: 'open' });
		expect(parseCommandQuery('@log')).toEqual({ mode: 'settings', query: 'log' });
		expect(parseCommandQuery('#node')).toEqual({ mode: 'cluster', query: 'node' });
		expect(parseCommandQuery('?term')).toEqual({ mode: 'help', query: 'term' });
		expect(parseCommandQuery('/tmp')).toEqual({ mode: 'paths', query: 'tmp' });
	});

	it('keeps the whole input in all mode when no prefix is given', () => {
		expect(parseCommandQuery('files')).toEqual({ mode: 'all', query: 'files' });
	});
});

describe('CommandRegistry', () => {
	it('lists visible commands and hides those whose when is false', () => {
		const registry = makeRegistry();
		registry.register([
			command({ id: 'a', title: 'Files', group: 'Navigate' }),
			command({ id: 'b', title: 'Hidden', group: 'Navigate', when: () => false })
		]);
		expect(registry.list(ctx).map((entry) => entry.command.id)).toEqual(['a']);
	});

	it('exposes the disabled reason of a visible command', () => {
		const registry = makeRegistry();
		registry.register([
			command({ id: 'a', title: 'Move', group: 'Files', disabledReason: () => 'Select a file first' })
		]);
		expect(registry.list(ctx)[0]?.disabledReason).toBe('Select a file first');
	});

	it('removes commands when the returned disposer runs', () => {
		const registry = makeRegistry();
		const unregister = registry.register([command({ id: 'a', title: 'Files', group: 'Navigate' })]);
		unregister();
		expect(registry.list(ctx)).toEqual([]);
	});

	it('matches titles with indices and keywords without indices', () => {
		const registry = makeRegistry();
		registry.register([command({ id: 'files', title: 'Files', group: 'Navigate', keywords: ['browse'] })]);
		expect(registry.search('fil', 'all')[0]?.indices).toEqual([0, 1, 2]);
		expect(registry.search('browse', 'all')[0]?.command.id).toBe('files');
		expect(registry.search('browse', 'all')[0]?.indices).toEqual([]);
	});

	it('restricts search results to the mode group', () => {
		const registry = makeRegistry();
		registry.register([
			command({ id: 'setting.logging', title: 'Logging', group: 'Settings' }),
			command({ id: 'nav.logs', title: 'Logs', group: 'Navigate' })
		]);
		expect(registry.search('log', 'settings').map((hit) => hit.command.id)).toEqual(['setting.logging']);
		expect(registry.search('log', 'all')).toHaveLength(2);
	});

	it('persists recent commands and lists them first for an empty query', async () => {
		const persist = vi.fn();
		const registry = makeRegistry([], persist);
		registry.register([
			command({ id: 'a', title: 'Alpha', group: 'Navigate' }),
			command({ id: 'b', title: 'Beta', group: 'Navigate' })
		]);
		expect(await registry.execute('b')).toEqual({ ok: true, value: null });
		expect(persist).toHaveBeenCalledWith(['b']);
		expect(registry.recent).toEqual(['b']);
		expect(registry.search('', 'all').map((hit) => hit.command.id)).toEqual(['b', 'a']);
	});

	it('returns a missing error for unknown ids', async () => {
		expect(await makeRegistry().execute('nope')).toEqual({ ok: false, error: { code: 'missing', reason: null } });
	});

	it('refuses disabled commands without running them', async () => {
		const registry = makeRegistry();
		const run = vi.fn();
		registry.register([
			command({ id: 'a', title: 'Move', group: 'Files', run, disabledReason: () => 'Select a file first' })
		]);
		expect(await registry.execute('a')).toEqual({
			ok: false,
			error: { code: 'disabled', reason: 'Select a file first' }
		});
		expect(run).not.toHaveBeenCalled();
	});

	it('reports a rejected run as a failed result with its message', async () => {
		const registry = makeRegistry();
		registry.register([
			command({ id: 'a', title: 'Boom', group: 'Account', run: () => Promise.reject(new Error('boom')) })
		]);
		expect(await registry.execute('a')).toEqual({ ok: false, error: { code: 'failed', reason: 'boom' } });
		expect(registry.recent).toEqual([]);
	});
});

describe('ShortcutManager', () => {
	it('runs a bound chord', () => {
		const manager = new ShortcutManager();
		const open = vi.fn();
		manager.bind('palette.open', open);
		stopListening = manager.start(window);
		press('k', { metaKey: true });
		expect(open).toHaveBeenCalledTimes(1);
	});

	it('runs a g sequence typed within one second and drops it after', () => {
		vi.useFakeTimers();
		const manager = new ShortcutManager();
		const files = vi.fn();
		manager.bind('go.files', files);
		stopListening = manager.start(window);
		press('g');
		press('f');
		expect(files).toHaveBeenCalledTimes(1);
		press('g');
		vi.advanceTimersByTime(1000);
		press('f');
		expect(files).toHaveBeenCalledTimes(1);
	});

	it('ignores single keys typed inside text fields unless a modifier is held', () => {
		const manager = new ShortcutManager();
		const next = vi.fn();
		const open = vi.fn();
		manager.bind('list.next', next);
		manager.bind('palette.open', open);
		const input = document.createElement('input');
		document.body.append(input);
		stopListening = manager.start(window);
		press('j', {}, input);
		expect(next).not.toHaveBeenCalled();
		press('k', { metaKey: true }, input);
		expect(open).toHaveBeenCalledTimes(1);
		input.remove();
	});

	it('turns single-key shortcuts off while keeping arrow keys', () => {
		const manager = new ShortcutManager();
		const next = vi.fn();
		manager.bind('list.next', next);
		manager.setSingleKeyEnabled(false);
		stopListening = manager.start(window);
		press('j');
		expect(next).not.toHaveBeenCalled();
		press('ArrowDown');
		expect(next).toHaveBeenCalledTimes(1);
	});

	it('uses remapped keys instead of the defaults', () => {
		const manager = new ShortcutManager();
		const open = vi.fn();
		manager.bind('palette.open', open);
		manager.setOverride('palette.open', [['mod', 'p']]);
		stopListening = manager.start(window);
		press('k', { metaKey: true });
		expect(open).not.toHaveBeenCalled();
		press('p', { metaKey: true });
		expect(open).toHaveBeenCalledTimes(1);
	});

	it('names the bindings that share keys in an overlapping scope', () => {
		const manager = new ShortcutManager();
		expect(manager.conflictsFor('files.move', [['d']])).toEqual(['files.download']);
		expect(manager.conflictsFor('sheet.open', [['/']])).toEqual(['search.focus']);
		expect(manager.conflictsFor('files.move', [['mod', 'k']])).toEqual(['palette.open']);
		expect(manager.conflictsFor('files.move', [['m']])).toEqual([]);
	});

	it('stops listening when the disposer runs', () => {
		const manager = new ShortcutManager();
		const open = vi.fn();
		manager.bind('palette.open', open);
		const stop = manager.start(window);
		stop();
		press('k', { metaKey: true });
		expect(open).not.toHaveBeenCalled();
	});

	it('gives every shortcut definition a unique id', () => {
		const ids = shortcutDefinitions.map((definition) => definition.id);
		expect(new Set(ids).size).toBe(ids.length);
	});
});

describe('visibleNavItems', () => {
	it('hides admin areas from members and points settings at appearance', () => {
		const items = visibleNavItems('member', []);
		expect(items.map((item) => item.id)).toEqual(['overview', 'files', 'sharing', 'activity', 'settings']);
		expect(items.find((item) => 'settings' === item.id)?.href).toBe('/settings/appearance');
		expect(items.find((item) => 'activity' === item.id)?.children.map((entry) => entry.id)).toEqual([
			'activity.jobs'
		]);
	});

	it('shows capability-gated cluster children only when the capability is granted', () => {
		const childIds = (capabilities: string[]) =>
			visibleNavItems('admin', capabilities)
				.find((item) => 'cluster' === item.id)
				?.children.map((entry) => entry.id) ?? [];
		expect(childIds([])).not.toContain('cluster.raft');
		expect(childIds(['raft'])).toContain('cluster.raft');
	});
});
