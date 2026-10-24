import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import type { Command, SearchHit } from '$lib/commands/registry.svelte';
import CommandPalette from './CommandPalette.svelte';
import ShortcutSheet, { type KeymapSection } from './ShortcutSheet.svelte';

afterEach(cleanup);

const noop = (): void => undefined;

const navFiles: Command = {
	id: 'nav.files',
	title: 'Files',
	group: 'Navigate',
	keywords: ['browse'],
	run: noop
};

const uploadFiles: Command = {
	id: 'files.upload',
	title: 'Upload files',
	group: 'Files',
	keywords: [],
	shortcut: ['u'],
	run: noop,
	secondary: { title: 'Upload folder', run: noop }
};

const addMember: Command = {
	id: 'cluster.add',
	title: 'Add member',
	group: 'Cluster',
	keywords: [],
	disabledReason: () => 'Needs admin',
	run: noop
};

function hit(command: Command, indices: number[] = [], disabledReason: string | null = null): SearchHit {
	return { command, disabledReason, score: 0, indices };
}

const allCommands = [navFiles, uploadFiles, addMember];

interface PaletteOverrides {
	recent?: string[];
	search?: (raw: string) => SearchHit[];
	completePath?: (partial: string) => string | null;
}

function renderPalette(overrides: PaletteOverrides = {}) {
	const search = vi.fn(
		overrides.search ??
			((_raw: string): SearchHit[] => [hit(navFiles), hit(uploadFiles), hit(addMember, [], 'Needs admin')])
	);
	const onrun = vi.fn();
	const completePath = vi.fn(overrides.completePath ?? ((_partial: string): string | null => null));
	const result = render(CommandPalette, {
		props: {
			open: true,
			commands: allCommands,
			recent: overrides.recent ?? [],
			search,
			completePath,
			onrun
		}
	});
	const input = screen.getByRole('combobox', { name: 'Search or run a command' }) as HTMLInputElement;
	return { ...result, input, search, onrun, completePath };
}

function optionsOf(): HTMLElement[] {
	return screen.getAllByRole('option');
}

describe('CommandPalette', () => {
	it('names the dialog and the combobox and lists every option', () => {
		renderPalette();

		expect(screen.getByRole('dialog', { name: 'Command palette' })).toBeTruthy();
		expect(screen.getByRole('listbox')).toBeTruthy();
		expect(optionsOf()).toHaveLength(3);
	});

	it('shows recent commands first and lists each command once', () => {
		renderPalette({ recent: ['files.upload'] });

		expect(screen.getByRole('group', { name: 'Recent' })).toBeTruthy();
		const options = optionsOf();
		expect(options).toHaveLength(3);
		expect(options[0].textContent).toContain('Upload files');
	});

	it('passes the raw input, including its prefix, to search', async () => {
		const { input, search } = renderPalette();

		await fireEvent.input(input, { target: { value: '>up' } });

		expect(search).toHaveBeenLastCalledWith('>up');
	});

	it('moves the active option with the arrow keys', async () => {
		const { input } = renderPalette();

		await fireEvent.keyDown(input, { key: 'ArrowDown' });

		const options = optionsOf();
		expect(options[1].getAttribute('aria-selected')).toBe('true');
		expect(input.getAttribute('aria-activedescendant')).toBe(options[1].id);
	});

	it('runs the active command as primary on Enter', async () => {
		const { input, onrun } = renderPalette();

		await fireEvent.keyDown(input, { key: 'Enter' });

		expect(onrun).toHaveBeenCalledWith(navFiles, 'primary');
	});

	it('runs the secondary action on mod+Enter when the command has one', async () => {
		const { input, onrun } = renderPalette();

		await fireEvent.keyDown(input, { key: 'ArrowDown' });
		await fireEvent.keyDown(input, { key: 'Enter', metaKey: true });

		expect(onrun).toHaveBeenCalledWith(uploadFiles, 'secondary');
	});

	it('does nothing on mod+Enter when the command has no secondary action', async () => {
		const { input, onrun } = renderPalette();

		await fireEvent.keyDown(input, { key: 'Enter', metaKey: true });

		expect(onrun).not.toHaveBeenCalled();
	});

	it('marks disabled commands and refuses to run them', async () => {
		const { input, onrun } = renderPalette();

		await fireEvent.keyDown(input, { key: 'ArrowDown' });
		await fireEvent.keyDown(input, { key: 'ArrowDown' });

		const disabled = optionsOf()[2];
		expect(disabled.getAttribute('aria-disabled')).toBe('true');
		expect(disabled.textContent).toContain('Needs admin');

		await fireEvent.keyDown(input, { key: 'Enter' });
		expect(onrun).not.toHaveBeenCalled();
	});

	it('completes a path on Tab and keeps focus in the palette', async () => {
		const completePath = vi.fn((_partial: string): string | null => '/team/');
		const { input } = renderPalette({ completePath });

		await fireEvent.input(input, { target: { value: '/te' } });
		const notPrevented = await fireEvent.keyDown(input, { key: 'Tab' });

		expect(completePath).toHaveBeenCalledWith('/te');
		expect(notPrevented).toBe(false);
		expect(input.value).toBe('/team/');
	});

	it('leaves Tab alone when there is no path completion', async () => {
		const { input } = renderPalette();

		await fireEvent.input(input, { target: { value: '/zz' } });

		expect(await fireEvent.keyDown(input, { key: 'Tab' })).toBe(true);
		expect(input.value).toBe('/zz');
	});

	it('shows an empty message that repeats the query', async () => {
		const { input } = renderPalette({ search: () => [] });

		await fireEvent.input(input, { target: { value: 'zzq' } });

		expect(screen.queryByRole('listbox')).toBeNull();
		expect(screen.getByRole('status').textContent).toContain('zzq');
	});

	it('marks the matched characters of a title', () => {
		renderPalette({ search: () => [hit(navFiles, [0, 1])] });

		expect(document.querySelector('mark')?.textContent).toBe('Fi');
	});
});

const keymap: KeymapSection[] = [
	{
		id: 'global',
		title: 'Global',
		entries: [
			{ id: 'palette.open', label: 'Open command palette', keys: [['mod', 'k']] },
			{ id: 'help.sheet', label: 'Show shortcuts', keys: [['?']] }
		]
	},
	{
		id: 'files',
		title: 'Files',
		entries: [{ id: 'files.up', label: 'Up one folder', keys: [['backspace'], ['alt', 'ArrowUp']] }]
	}
];

describe('ShortcutSheet', () => {
	it('lists every section with its shortcuts', () => {
		render(ShortcutSheet, { props: { open: true, keymap } });

		const globalRegion = screen.getByRole('region', { name: 'Global' });
		expect(within(globalRegion).getByText('Open command palette')).toBeTruthy();
		expect(within(globalRegion).getByText('Show shortcuts')).toBeTruthy();
		const filesRegion = screen.getByRole('region', { name: 'Files' });
		expect(within(filesRegion).getByText('Up one folder')).toBeTruthy();
	});

	it('names the dialog and says the single-key state is on by default', () => {
		render(ShortcutSheet, { props: { open: true, keymap } });

		expect(screen.getByRole('dialog', { name: 'Keyboard shortcuts' })).toBeTruthy();
		expect(screen.getByText('Single-key shortcuts are on (turn off in Settings)')).toBeTruthy();
	});

	it('says the single-key state is off when turned off', () => {
		render(ShortcutSheet, { props: { open: true, keymap, singleKeyShortcuts: false } });

		expect(screen.getByText('Single-key shortcuts are off (turn on in Settings)')).toBeTruthy();
	});

	it('filters shortcuts and hides sections without a match', async () => {
		render(ShortcutSheet, { props: { open: true, keymap } });

		await fireEvent.input(screen.getByRole('searchbox', { name: 'Find a shortcut' }), {
			target: { value: 'folder' }
		});

		expect(screen.queryByText('Show shortcuts')).toBeNull();
		expect(screen.queryByRole('region', { name: 'Global' })).toBeNull();
		expect(screen.getByText('Up one folder')).toBeTruthy();
	});

	it('shows an empty message that repeats the query', async () => {
		render(ShortcutSheet, { props: { open: true, keymap } });

		await fireEvent.input(screen.getByRole('searchbox', { name: 'Find a shortcut' }), {
			target: { value: 'zzq' }
		});

		expect(screen.getByRole('status').textContent).toContain('zzq');
	});
});
