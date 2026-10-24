import { expect, test } from '@playwright/test';
import { signIn } from '../helpers';
import { installMocks } from '../mocks/server';

const NAV_COMMANDS: [string, RegExp][] = [
	['Overview', /\/ui\/$/],
	['Files', /\/ui\/files\/?$/],
	['Sharing', /\/ui\/sharing$/],
	['Cluster', /\/ui\/cluster$/],
	['Insights', /\/ui\/insights$/],
	['Activity', /\/ui\/activity$/],
	['Settings', /\/ui\/settings\/network$/]
];

const SETTING_COMMANDS: [string, RegExp][] = [
	['Listen address', /\/ui\/settings\/network$/],
	['Store new files on', /\/ui\/settings\/storage$/],
	['Detail level', /\/ui\/settings\/logging$/],
	['Shared server secret', /\/ui\/settings\/security$/]
];

test('palette reaches every nav command and one setting per settings group', async ({ page }) => {
	await installMocks(page, 'healthy');
	await signIn(page, 'admin');
	await page.goto('');

	for (const [title, url] of NAV_COMMANDS) {
		await page.keyboard.press('ControlOrMeta+k');
		const palette = page.getByRole('dialog');
		await expect(palette).toBeVisible();
		await palette.getByRole('combobox').fill(title);
		await page.keyboard.press('Enter');
		await expect(palette).toBeHidden();
		await expect(page).toHaveURL(url);
	}

	for (const [title, url] of SETTING_COMMANDS) {
		await page.keyboard.press('ControlOrMeta+k');
		const palette = page.getByRole('dialog');
		await palette.getByRole('combobox').fill(title);
		await page.keyboard.press('Enter');
		await expect(page).toHaveURL(url);
	}
});

test('palette closes with Escape and returns focus to the page', async ({ page }) => {
	await installMocks(page, 'healthy');
	await signIn(page, 'admin');
	await page.goto('');
	await page.keyboard.press('ControlOrMeta+k');
	await expect(page.getByRole('dialog')).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.getByRole('dialog')).toBeHidden();
});
