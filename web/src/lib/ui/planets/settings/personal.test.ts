import '@testing-library/jest-dom/vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { preferences } from '$lib/stores/preferences.svelte';
import { toasts } from '$lib/stores/toasts.svelte';
import AppearancePlanet from './AppearancePlanet.svelte';
import ShortcutsPlanet from './ShortcutsPlanet.svelte';

beforeEach(() => {
	preferences.reset();
	preferences.status = 'ready';
	preferences.error = null;
	toasts.items = [];
	toasts.queue = [];
});

afterEach(() => {
	cleanup();
});

describe('AppearancePlanet', () => {
	it('renders the heading and the theme editor with the current mode', () => {
		render(AppearancePlanet);
		expect(screen.getByRole('heading', { level: 1, name: 'Appearance' })).toBeInTheDocument();
		expect(screen.getByText('Make the console look right for you.')).toBeInTheDocument();
		expect(screen.getByRole('radio', { name: 'Dark' })).toBeInTheDocument();
	});

	it('shows no editor while preferences load', () => {
		preferences.status = 'loading';
		render(AppearancePlanet);
		expect(screen.getByRole('heading', { level: 1, name: 'Appearance' })).toBeInTheDocument();
		expect(screen.queryByRole('radio', { name: 'Dark' })).toBeNull();
	});

	it('shows the error copy when preferences fail to load', () => {
		preferences.status = 'error';
		preferences.error = {
			title: 'We cannot reach callfs.',
			body: 'Check your connection. We will try again.',
			retryable: true
		};
		render(AppearancePlanet);
		expect(screen.getByText('We cannot reach callfs.')).toBeInTheDocument();
		expect(screen.queryByRole('radio', { name: 'Dark' })).toBeNull();
	});

	it('saves a theme change at once and undoes it from the toast', async () => {
		render(AppearancePlanet);
		await fireEvent.click(screen.getByRole('radio', { name: 'Dark' }));
		await tick();
		expect(preferences.value.theme).toBe('dark');
		expect(toasts.items).toHaveLength(1);
		const action = toasts.items[0]?.action;
		expect(action?.label).toBe('Undo');
		action?.run();
		await tick();
		expect(preferences.value.theme).toBe('system');
	});
});

describe('ShortcutsPlanet', () => {
	it('renders the commands table and the single-key switch on by default', () => {
		render(ShortcutsPlanet);
		expect(screen.getByRole('heading', { level: 1, name: 'Shortcuts' })).toBeInTheDocument();
		expect(screen.getByRole('table', { name: 'Commands' })).toBeInTheDocument();
		expect(screen.getByRole('switch', { name: 'Use keys without a modifier' })).toBeChecked();
	});

	it('turns single-key shortcuts off from the switch', async () => {
		render(ShortcutsPlanet);
		await fireEvent.click(screen.getByRole('switch', { name: 'Use keys without a modifier' }));
		await tick();
		expect(preferences.value.singleKeyShortcuts).toBe(false);
	});

	it('records a new key sequence for a command and saves it', async () => {
		render(ShortcutsPlanet);
		await fireEvent.click(screen.getByRole('button', { name: 'Change Go to Files' }));
		expect(screen.getByText('Press the new keys. Esc cancels.')).toBeInTheDocument();
		await fireEvent.keyDown(window, { key: 'y' });
		await tick();
		expect(preferences.value.keymapOverrides['go.files']).toEqual(['y']);
		expect(screen.queryByText('Press the new keys. Esc cancels.')).toBeNull();
	});

	it('names the conflicting command and keeps recording when keys are taken', async () => {
		render(ShortcutsPlanet);
		await fireEvent.click(screen.getByRole('button', { name: 'Change Go to Files' }));
		await fireEvent.keyDown(window, { key: 'k', metaKey: true });
		await tick();
		expect(
			screen.getByText('Already used by Open command palette. Press other keys, or Esc to cancel.')
		).toBeInTheDocument();
		expect(preferences.value.keymapOverrides['go.files']).toBeUndefined();
		expect(screen.getByText('Press the new keys. Esc cancels.')).toBeInTheDocument();
	});

	it('cancels recording with Escape without saving', async () => {
		render(ShortcutsPlanet);
		await fireEvent.click(screen.getByRole('button', { name: 'Change Go to Files' }));
		await fireEvent.keyDown(window, { key: 'Escape' });
		await tick();
		expect(screen.queryByText('Press the new keys. Esc cancels.')).toBeNull();
		expect(preferences.value.keymapOverrides['go.files']).toBeUndefined();
	});

	it('clears every recorded override with Reset all', async () => {
		preferences.update({ keymapOverrides: { 'go.files': ['y'] } });
		render(ShortcutsPlanet);
		await fireEvent.click(screen.getByRole('button', { name: 'Reset all' }));
		await tick();
		expect(preferences.value.keymapOverrides).toEqual({});
		expect(toasts.items).toHaveLength(1);
	});
});
