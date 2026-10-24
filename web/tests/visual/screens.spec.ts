import { expect, test } from '@playwright/test';
import { signIn } from '../helpers';
import { installMocks } from '../mocks/server';

const SCREENS = [
	{ name: 'overview', path: './' },
	{ name: 'browse', path: 'files' },
	{ name: 'topology', path: 'cluster' },
	{ name: 'dashboards', path: 'insights' },
	{ name: 'settings', path: 'settings/network' }
] as const;

const VIEWPORTS = [
	{ width: 1280, height: 800 },
	{ width: 390, height: 844 }
] as const;

const SCHEMES = ['light', 'dark'] as const;

for (const colorScheme of SCHEMES) {
	for (const viewport of VIEWPORTS) {
		for (const screen of SCREENS) {
			test(`${screen.name} ${colorScheme} at ${viewport.width}px`, async ({ page }) => {
				await page.setViewportSize(viewport);
				await page.emulateMedia({ colorScheme });
				await installMocks(page, 'healthy');
				await signIn(page, 'admin');
				await page.goto(screen.path);
				await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
				await expect(page).toHaveScreenshot(
					`${screen.name}-${colorScheme}-${viewport.width}.png`,
					{ fullPage: true }
				);
			});
		}
	}
}
