import { expect, test, type Page } from '@playwright/test';
import { axe, expectLandmarks, signIn } from '../helpers';
import { installMocks } from '../mocks/server';

type MediaOptions = Parameters<Page['emulateMedia']>[0];

interface ListedTarget {
	list: string;
	prefix: string;
}

interface RouteCase {
	name: string;
	target: string | ListedTarget;
	console: boolean;
}

interface ThemeCase {
	name: string;
	media: MediaOptions;
}

interface ViewportCase {
	name: string;
	width: number;
	height: number;
}

const ROUTES: RouteCase[] = [
	{ name: 'Overview', target: '', console: true },
	{ name: 'Files', target: 'files', console: true },
	{ name: 'Files search', target: 'files/search?q=report', console: true },
	{ name: 'Files flat list', target: 'files/flat', console: true },
	{ name: 'Transfers', target: 'files/transfers', console: true },
	{ name: 'Trash', target: 'files/trash', console: true },
	{ name: 'Sharing', target: 'sharing', console: true },
	{
		name: 'Link detail',
		target: { list: 'sharing', prefix: '/sharing/' },
		console: true
	},
	{ name: 'Topology', target: 'cluster', console: true },
	{ name: 'Members', target: 'cluster/nodes', console: true },
	{
		name: 'Member detail',
		target: { list: 'cluster/nodes', prefix: '/cluster/nodes/' },
		console: true
	},
	{ name: 'Raft', target: 'cluster/raft', console: true },
	{ name: 'Servers', target: 'cluster/servers', console: true },
	{ name: 'Storage', target: 'cluster/storage', console: true },
	{ name: 'Durability', target: 'cluster/durability', console: true },
	{
		name: 'Durability file',
		target: { list: 'cluster/durability', prefix: '/cluster/durability/file/' },
		console: true
	},
	{ name: 'Backup copy', target: 'cluster/backup', console: true },
	{ name: 'Hygiene', target: 'cluster/hygiene', console: true },
	{ name: 'Edit locks', target: 'cluster/locks', console: true },
	{ name: 'Index cache', target: 'cluster/cache', console: true },
	{ name: 'Request limits', target: 'cluster/limits', console: true },
	{ name: 'Dashboards', target: 'insights', console: true },
	{ name: 'Explore', target: 'insights/explore', console: true },
	{ name: 'Storage insights', target: 'insights/storage', console: true },
	{ name: 'Metrics catalog', target: 'insights/metrics', console: true },
	{ name: 'Events', target: 'activity', console: true },
	{ name: 'Audit', target: 'activity/audit', console: true },
	{ name: 'Logs', target: 'activity/logs', console: true },
	{ name: 'Jobs', target: 'activity/jobs', console: true },
	{
		name: 'Job detail',
		target: { list: 'activity/jobs', prefix: '/activity/jobs/' },
		console: true
	},
	{ name: 'Sign-ins', target: 'activity/sign-ins', console: true },
	{ name: 'Settings network', target: 'settings/network', console: true },
	{ name: 'Settings limits', target: 'settings/limits', console: true },
	{ name: 'Settings security', target: 'settings/security', console: true },
	{ name: 'Settings storage', target: 'settings/storage', console: true },
	{ name: 'Settings file index', target: 'settings/index', console: true },
	{ name: 'Settings coordination', target: 'settings/coordination', console: true },
	{ name: 'Settings logging', target: 'settings/logging', console: true },
	{ name: 'Users and keys', target: 'settings/access', console: true },
	{ name: 'Config file', target: 'settings/file', console: true },
	{ name: 'Appearance', target: 'settings/appearance', console: true },
	{ name: 'Shortcuts', target: 'settings/shortcuts', console: true },
	{ name: 'About', target: 'settings/about', console: true },
	{ name: 'Wall', target: 'wall', console: false },
	{ name: 'Report', target: 'report', console: false },
	{ name: 'Login', target: 'login', console: false },
	{ name: 'Not found', target: 'no-such-page', console: false }
];

const THEMES: ThemeCase[] = [
	{ name: 'light', media: { colorScheme: 'light', contrast: 'no-preference' } },
	{ name: 'dark', media: { colorScheme: 'dark', contrast: 'no-preference' } },
	{ name: 'more contrast', media: { colorScheme: 'light', contrast: 'more' } }
];

const VIEWPORTS: ViewportCase[] = [
	{ name: '1280x800', width: 1280, height: 800 },
	{ name: '390x844', width: 390, height: 844 }
];

async function resolveTarget(page: Page, target: RouteCase['target']): Promise<string> {
	if ('string' === typeof target) {
		return target;
	}
	await page.goto(target.list);
	const href = await page
		.locator(`main a[href*="${target.prefix}"]`)
		.first()
		.getAttribute('href');
	expect(href).toEqual(expect.any(String));
	return href ?? '';
}

for (const route of ROUTES) {
	for (const viewport of VIEWPORTS) {
		for (const theme of THEMES) {
			test(`${route.name} has no serious or critical axe violations (${theme.name}, ${viewport.name})`, async ({
				page
			}) => {
				await page.setViewportSize({ width: viewport.width, height: viewport.height });
				await page.emulateMedia(theme.media);
				await installMocks(page, 'healthy');
				if (route.console) {
					await signIn(page, 'admin');
				}
				const path = await resolveTarget(page, route.target);
				await page.goto(path);
				await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
				if (route.console) {
					await expectLandmarks(page);
				}
				const violations = await axe(page);
				expect(violations.map((violation) => violation.id)).toEqual([]);
			});
		}
	}
}
