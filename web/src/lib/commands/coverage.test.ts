import { describe, expect, it } from 'vitest';
import { navItems } from '$lib/nav/model';
import { createGlobalCommands, ROUTES } from '$lib/ui/galaxies/global-commands';
import { SETTINGS_MAP } from '$lib/ui/planets/settings/settings-map';

const commandIds = new Set(
	createGlobalCommands({
		role: 'admin',
		go: () => undefined,
		currentTheme: () => 'light',
		setPreferences: () => undefined,
		openShortcuts: () => undefined,
		openHelp: () => undefined,
		openNotifications: () => undefined,
		toggleSidebar: () => undefined,
		reportProblem: () => undefined,
		signOut: () => undefined,
		copyUserId: () => undefined
	}).map((command) => command.id)
);

const slugByPath = new Map(ROUTES.map((route) => [route.path, route.slug]));

describe('navigation command coverage', () => {
	it('registers a navigation command for every nav item and child', () => {
		const hrefs = navItems.flatMap((item) => [
			item.href,
			...(null === item.memberHref ? [] : [item.memberHref]),
			...item.children.map((child) => child.href)
		]);
		const missing = hrefs.filter((href) => {
			const slug = slugByPath.get(href);
			return undefined === slug || !commandIds.has(`nav.${slug}`);
		});
		expect(missing).toEqual([]);
	});
});

describe('settings command coverage', () => {
	it('gives every settings key a unique id', () => {
		const keys = SETTINGS_MAP.map((entry) => entry.key);
		expect(new Set(keys).size).toBe(keys.length);
	});

	it('routes every settings key to a settings page with a navigation command', () => {
		const missing = SETTINGS_MAP.filter(
			(entry) => !commandIds.has(`nav.settings.${entry.page}`)
		).map((entry) => entry.key);
		expect(missing).toEqual([]);
	});
});
