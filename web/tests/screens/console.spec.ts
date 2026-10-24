import { expect, test } from '@playwright/test';
import { expectLandmarks, signIn } from '../helpers';
import { installMocks } from '../mocks/server';

type Shell = 'console' | 'bare';

interface ScreenRoute {
	path: string;
	shell: Shell;
	member: boolean;
}

const ADMIN_SCENARIOS = ['healthy', 'fresh_install', 'degraded_no_leader', 'errors_everywhere', 'huge_folder'] as const;

const ROUTES: readonly ScreenRoute[] = [
	{ path: '', shell: 'console', member: true },
	{ path: 'files', shell: 'console', member: true },
	{ path: 'files/team', shell: 'console', member: true },
	{ path: 'files/search?q=report', shell: 'console', member: true },
	{ path: 'files/flat?prefix=/', shell: 'console', member: false },
	{ path: 'files/transfers', shell: 'console', member: true },
	{ path: 'files/trash', shell: 'console', member: true },
	{ path: 'sharing', shell: 'console', member: true },
	{ path: 'sharing/1', shell: 'console', member: true },
	{ path: 'cluster', shell: 'console', member: false },
	{ path: 'cluster/nodes', shell: 'console', member: false },
	{ path: 'cluster/nodes/callfs-node-2', shell: 'console', member: false },
	{ path: 'cluster/raft', shell: 'console', member: false },
	{ path: 'cluster/servers', shell: 'console', member: false },
	{ path: 'cluster/storage', shell: 'console', member: false },
	{ path: 'cluster/durability', shell: 'console', member: false },
	{ path: 'cluster/durability/file/archive/split-01.bin', shell: 'console', member: false },
	{ path: 'cluster/backup', shell: 'console', member: false },
	{ path: 'cluster/hygiene', shell: 'console', member: false },
	{ path: 'cluster/locks', shell: 'console', member: false },
	{ path: 'cluster/cache', shell: 'console', member: false },
	{ path: 'cluster/limits', shell: 'console', member: false },
	{ path: 'insights', shell: 'console', member: false },
	{ path: 'insights/explore', shell: 'console', member: false },
	{ path: 'insights/storage', shell: 'console', member: false },
	{ path: 'insights/metrics', shell: 'console', member: false },
	{ path: 'activity', shell: 'console', member: true },
	{ path: 'activity/audit', shell: 'console', member: false },
	{ path: 'activity/logs', shell: 'console', member: false },
	{ path: 'activity/jobs', shell: 'console', member: true },
	{ path: 'activity/jobs/job-2', shell: 'console', member: true },
	{ path: 'activity/sign-ins', shell: 'console', member: false },
	{ path: 'settings/network', shell: 'console', member: false },
	{ path: 'settings/limits', shell: 'console', member: false },
	{ path: 'settings/security', shell: 'console', member: false },
	{ path: 'settings/storage', shell: 'console', member: false },
	{ path: 'settings/index', shell: 'console', member: false },
	{ path: 'settings/coordination', shell: 'console', member: false },
	{ path: 'settings/logging', shell: 'console', member: false },
	{ path: 'settings/access', shell: 'console', member: false },
	{ path: 'settings/file', shell: 'console', member: false },
	{ path: 'settings/appearance', shell: 'console', member: true },
	{ path: 'settings/shortcuts', shell: 'console', member: true },
	{ path: 'settings/about', shell: 'console', member: true },
	{ path: 'wall', shell: 'bare', member: true },
	{ path: 'report', shell: 'bare', member: true },
	{ path: 'login', shell: 'bare', member: true },
	{ path: 'no-such-page', shell: 'bare', member: true }
];

interface ScreenCase {
	scenario: string;
	role: 'admin' | 'member';
	route: ScreenRoute;
}

const CASES: readonly ScreenCase[] = [
	...ADMIN_SCENARIOS.flatMap((scenario) => ROUTES.map((route) => ({ scenario, role: 'admin' as const, route }))),
	...ROUTES.filter((route) => route.member).map((route) => ({ scenario: 'member_user', role: 'member' as const, route }))
];

test.describe('console screens render landmarks and a heading in every scenario', () => {
	for (const { scenario, role, route } of CASES) {
		test(`/${route.path} renders under ${scenario}`, async ({ page }) => {
			await installMocks(page, scenario);
			await signIn(page, role);
			await page.goto(route.path);
			if ('console' === route.shell) {
				await expectLandmarks(page);
			} else {
				await expect(page.getByRole('main')).toHaveCount(1);
				await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
			}
		});
	}
});
