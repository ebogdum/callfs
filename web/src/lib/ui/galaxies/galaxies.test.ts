import type { Command, CommandContext } from '$lib/commands/registry.svelte';
import galaxy from '$lib/copy/en/galaxy';
import { GLOSSARY } from '$lib/copy/glossary';
import { announce } from '$lib/ui/quarks/announce';
import { render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import LiveRegions from './LiveRegions.svelte';
import {
	APPEARANCE_RESET,
	ROUTES,
	buildDiagnostics,
	createGlobalCommands,
	recentRequestIds,
	selectBanner,
	type GlobalCommandDeps
} from './global-commands';

function makeDeps(role: 'admin' | 'member', overrides: Partial<GlobalCommandDeps> = {}): GlobalCommandDeps {
	return {
		role,
		go: vi.fn(),
		currentTheme: () => 'light',
		setPreferences: vi.fn(),
		openShortcuts: vi.fn(),
		openHelp: vi.fn(),
		openNotifications: vi.fn(),
		toggleSidebar: vi.fn(),
		reportProblem: vi.fn(),
		signOut: vi.fn(),
		copyUserId: vi.fn(),
		...overrides
	};
}

const context: CommandContext = { role: 'admin', route: '/', selectionCount: 0, capabilities: [] };

function commandIds(commands: Command[]): string[] {
	return commands.map((command) => command.id);
}

function commandById(commands: Command[], id: string): Command | undefined {
	return commands.find((command) => id === command.id);
}

describe('createGlobalCommands', () => {
	it('registers a navigation command for every static route to admins', () => {
		const ids = commandIds(createGlobalCommands(makeDeps('admin')));
		for (const route of ROUTES) {
			expect(ids).toContain(`nav.${route.slug}`);
		}
	});

	it('hides admin-only routes from members and keeps member routes', () => {
		const ids = commandIds(createGlobalCommands(makeDeps('member')));
		for (const route of ROUTES.filter((entry) => entry.admin)) {
			expect(ids).not.toContain(`nav.${route.slug}`);
		}
		for (const route of ROUTES.filter((entry) => !entry.admin)) {
			expect(ids).toContain(`nav.${route.slug}`);
		}
	});

	it('gives every command a unique id', () => {
		const ids = commandIds(createGlobalCommands(makeDeps('admin')));
		expect(new Set(ids).size).toBe(ids.length);
	});

	it('has a copy key for every route command title', () => {
		for (const route of ROUTES) {
			expect(Object.hasOwn(galaxy, `galaxy.route.${route.slug}`)).toBe(true);
		}
	});

	it('navigates to the route path when a route command runs', async () => {
		const deps = makeDeps('admin');
		await commandById(createGlobalCommands(deps), 'nav.cluster.nodes')?.run(context);
		expect(deps.go).toHaveBeenCalledWith('/cluster/nodes');
	});

	it('applies one preference patch per appearance choice', async () => {
		const deps = makeDeps('member');
		const commands = createGlobalCommands(deps);
		await commandById(commands, 'appearance.theme.dark')?.run(context);
		await commandById(commands, 'appearance.font.xlarge')?.run(context);
		expect(deps.setPreferences).toHaveBeenCalledWith({ theme: 'dark' });
		expect(deps.setPreferences).toHaveBeenCalledWith({ fontScale: 1.25 });
	});

	it('toggles the theme away from the current one', async () => {
		const deps = makeDeps('member', { currentTheme: () => 'dark' });
		await commandById(createGlobalCommands(deps), 'appearance.theme.toggle')?.run(context);
		expect(deps.setPreferences).toHaveBeenCalledWith({ theme: 'light' });
	});

	it('resets appearance to the defaults', async () => {
		const deps = makeDeps('member');
		await commandById(createGlobalCommands(deps), 'appearance.reset')?.run(context);
		expect(deps.setPreferences).toHaveBeenCalledWith(APPEARANCE_RESET);
	});

	it('registers one help command per glossary term that opens that term', async () => {
		const deps = makeDeps('member');
		const commands = createGlobalCommands(deps);
		const glossaryIds = commandIds(commands).filter((id) => id.startsWith('help.glossary.'));
		expect(glossaryIds).toHaveLength(Object.keys(GLOSSARY).length);
		await commandById(commands, 'help.glossary.file')?.run(context);
		expect(deps.openHelp).toHaveBeenCalledWith('file');
	});

	it('wires help, account and global shell commands to their deps', async () => {
		const deps = makeDeps('member');
		const commands = createGlobalCommands(deps);
		await commandById(commands, 'help.shortcuts')?.run(context);
		await commandById(commands, 'help.report')?.run(context);
		await commandById(commands, 'account.sign_out')?.run(context);
		await commandById(commands, 'account.copy_user_id')?.run(context);
		await commandById(commands, 'nav.notifications')?.run(context);
		await commandById(commands, 'nav.sidebar')?.run(context);
		expect(deps.openShortcuts).toHaveBeenCalledTimes(1);
		expect(deps.reportProblem).toHaveBeenCalledTimes(1);
		expect(deps.signOut).toHaveBeenCalledTimes(1);
		expect(deps.copyUserId).toHaveBeenCalledTimes(1);
		expect(deps.openNotifications).toHaveBeenCalledTimes(1);
		expect(deps.toggleSidebar).toHaveBeenCalledTimes(1);
	});
});

describe('selectBanner', () => {
	it('shows a pending restart before every other condition', () => {
		expect(
			selectBanner({ restartRequired: true, noLeader: true, apiReachable: false, criticalFindings: 3 })
		).toBe('restart');
	});

	it('shows no leader before an unreachable API', () => {
		expect(
			selectBanner({ restartRequired: false, noLeader: true, apiReachable: false, criticalFindings: 1 })
		).toBe('no-leader');
	});

	it('shows an unreachable API before critical findings', () => {
		expect(
			selectBanner({ restartRequired: false, noLeader: false, apiReachable: false, criticalFindings: 2 })
		).toBe('unreachable');
	});

	it('shows critical findings when nothing else applies', () => {
		expect(
			selectBanner({ restartRequired: false, noLeader: false, apiReachable: true, criticalFindings: 2 })
		).toBe('findings');
	});

	it('shows no banner when the console is healthy', () => {
		expect(
			selectBanner({ restartRequired: false, noLeader: false, apiReachable: true, criticalFindings: 0 })
		).toBeNull();
	});
});

describe('diagnostics', () => {
	it('keeps the last ten request ids and skips records without one', () => {
		const records = [{ requestId: null }, ...Array.from({ length: 12 }, (_, index) => ({ requestId: `req-${index}` }))];
		const ids = recentRequestIds(records);
		expect(ids).toHaveLength(10);
		expect(ids[0]).toBe('req-2');
		expect(ids[9]).toBe('req-11');
	});

	it('builds a JSON report with version, route, request ids and browser', () => {
		const report = JSON.parse(
			buildDiagnostics({ version: '1.2.3', route: '/files', requestIds: ['abc'], userAgent: 'agent' })
		);
		expect(report).toEqual({ version: '1.2.3', route: '/files', requestIds: ['abc'], browser: 'agent' });
	});
});

describe('LiveRegions', () => {
	it('renders a polite and an assertive region before any update', () => {
		const { container } = render(LiveRegions);
		const polite = container.querySelector('#live-polite');
		const assertive = container.querySelector('#live-assertive');
		expect(polite?.getAttribute('aria-live')).toBe('polite');
		expect(polite?.getAttribute('role')).toBe('status');
		expect(assertive?.getAttribute('aria-live')).toBe('assertive');
		expect(assertive?.getAttribute('role')).toBe('alert');
	});

	it('writes announced text into the matching region', () => {
		const { container } = render(LiveRegions);
		const frame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
			callback(0);
			return 0;
		});
		announce('Saved', 'polite');
		announce('Failed', 'assertive');
		expect(container.querySelector('#live-polite')?.textContent).toBe('Saved');
		expect(container.querySelector('#live-assertive')?.textContent).toBe('Failed');
		frame.mockRestore();
	});
});
