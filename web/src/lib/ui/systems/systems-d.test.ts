import { createRawSnippet } from 'svelte';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getConfig } from '$lib/api/admin/config';
import { clearCache } from '$lib/data/cache';
import AuthSystem from './AuthSystem.svelte';
import SettingsSystem from './SettingsSystem.svelte';

const sessionState = vi.hoisted(() => ({ isAdmin: false, me: null as unknown }));
const pageState = vi.hoisted(() => ({ pathname: '/settings/network', search: '' }));
const navigation = vi.hoisted(() => ({ goto: vi.fn() }));

vi.mock('$app/navigation', () => ({ goto: navigation.goto }));

vi.mock('$app/state', () => ({
	page: {
		route: { id: '/(console)/settings/network' },
		get url() {
			return new URL(`http://127.0.0.1:4173${pageState.pathname}${pageState.search}`);
		}
	}
}));

vi.mock('$lib/api/admin/config', () => ({ getConfig: vi.fn() }));

vi.mock('$lib/stores/session.svelte', () => ({
	session: {
		get isAdmin() {
			return sessionState.isAdmin;
		},
		get me() {
			return sessionState.me;
		}
	}
}));

const childSnippet = () => createRawSnippet(() => ({ render: () => '<p>Child content</p>' }));

const configWithPending = (pending: string[]) => ({
	ok: true,
	value: { config_path: '/etc/callfs/config.yaml', restart_required: 0 < pending.length, pending, keys: [] }
});

describe('SettingsSystem', () => {
	beforeEach(() => {
		clearCache();
		sessionState.isAdmin = true;
		pageState.pathname = '/settings/network';
		pageState.search = '';
		vi.mocked(getConfig).mockResolvedValue(configWithPending([]) as never);
	});

	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it('renders the section navigation with the current page marked', () => {
		render(SettingsSystem, { props: { children: childSnippet() } });

		const network = screen.getByRole('link', { name: 'Network' });
		expect(network.getAttribute('href')).toBe('/ui/settings/network');
		expect(network.getAttribute('aria-current')).toBe('page');
		expect(screen.getByText('Child content')).toBeTruthy();
	});

	it('shows every admin section group to admins', () => {
		render(SettingsSystem, { props: { children: childSnippet() } });

		for (const name of ['Network', 'Limits and timeouts', 'Storage', 'File index', 'Cluster and locks', 'Users and keys', 'Logging', 'Config file', 'About', 'Appearance', 'Shortcuts']) {
			expect(screen.getByRole('link', { name })).toBeTruthy();
		}
	});

	it('shows only Personal and About to members', () => {
		sessionState.isAdmin = false;
		pageState.pathname = '/settings/appearance';

		render(SettingsSystem, { props: { children: childSnippet() } });

		expect(screen.getByRole('link', { name: 'About' }).getAttribute('href')).toBe('/ui/settings/about');
		expect(screen.getByRole('link', { name: 'Appearance' })).toBeTruthy();
		expect(screen.getByRole('link', { name: 'Shortcuts' })).toBeTruthy();
		expect(screen.queryByRole('link', { name: 'Network' })).toBeNull();
		expect(screen.queryByRole('link', { name: 'Config file' })).toBeNull();
		expect(screen.queryByRole('link', { name: 'Users and keys' })).toBeNull();
	});

	it('shows the restart banner with the count of pending saved changes', async () => {
		vi.mocked(getConfig).mockResolvedValue(configWithPending(['server.listen_addr', 'log.level']) as never);

		render(SettingsSystem, { props: { children: childSnippet() } });

		expect(await screen.findByText('2 saved changes need a restart. Restart callfs to use them.')).toBeTruthy();
	});

	it('uses singular copy when one change is pending', async () => {
		vi.mocked(getConfig).mockResolvedValue(configWithPending(['server.listen_addr']) as never);

		render(SettingsSystem, { props: { children: childSnippet() } });

		expect(await screen.findByText('1 saved change needs a restart. Restart callfs to use it.')).toBeTruthy();
	});

	it('hides the restart banner when nothing is pending', async () => {
		render(SettingsSystem, { props: { children: childSnippet() } });

		await vi.waitFor(() => expect(getConfig).toHaveBeenCalled());
		expect(screen.queryByText(/need a restart/)).toBeNull();
	});

	it('does not load admin config for members', async () => {
		sessionState.isAdmin = false;

		render(SettingsSystem, { props: { children: childSnippet() } });

		await Promise.resolve();
		expect(getConfig).not.toHaveBeenCalled();
	});

	it('opens the config file page from the restart banner action', async () => {
		vi.mocked(getConfig).mockResolvedValue(configWithPending(['log.level']) as never);

		render(SettingsSystem, { props: { children: childSnippet() } });

		await fireEvent.click(await screen.findByRole('button', { name: 'Review changes' }));

		expect(navigation.goto).toHaveBeenCalledWith('/ui/settings/file');
	});
});

describe('AuthSystem', () => {
	beforeEach(() => {
		sessionState.me = null;
		pageState.search = '';
	});

	afterEach(() => {
		cleanup();
		vi.clearAllMocks();
	});

	it('renders its children', () => {
		render(AuthSystem, { props: { children: childSnippet() } });

		expect(screen.getByText('Child content')).toBeTruthy();
	});

	it('does not navigate before sign-in', () => {
		pageState.search = '?redirect=%2Ffiles%2Fa';

		render(AuthSystem, { props: { children: childSnippet() } });

		expect(navigation.goto).not.toHaveBeenCalled();
	});

	it('navigates to the redirect target after sign-in', () => {
		pageState.search = '?redirect=%2Ffiles%2Fa';
		sessionState.me = { user_id: 'alice', is_admin: true };

		render(AuthSystem, { props: { children: childSnippet() } });

		expect(navigation.goto).toHaveBeenCalledWith('/ui/files/a', { replaceState: true });
	});

	it('navigates to the overview when no redirect is given', () => {
		sessionState.me = { user_id: 'alice', is_admin: false };

		render(AuthSystem, { props: { children: childSnippet() } });

		expect(navigation.goto).toHaveBeenCalledWith('/ui/', { replaceState: true });
	});

	it('ignores redirect targets that leave the console', () => {
		sessionState.me = { user_id: 'alice', is_admin: false };

		for (const target of ['https%3A%2F%2Fevil.test', '%2F%2Fevil.test', '%2F%5Cevil.test']) {
			pageState.search = `?redirect=${target}`;
			render(AuthSystem, { props: { children: childSnippet() } });
		}

		expect(navigation.goto.mock.calls.every(([path]) => '/ui/' === path)).toBe(true);
		expect(navigation.goto).toHaveBeenCalledTimes(3);
	});
});
