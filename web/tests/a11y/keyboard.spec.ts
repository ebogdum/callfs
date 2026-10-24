import { expect, test, type Locator, type Page } from '@playwright/test';
import { signIn } from '../helpers';
import { installMocks } from '../mocks/server';

async function openConsole(page: Page): Promise<void> {
	await installMocks(page, 'healthy');
	await signIn(page, 'admin');
	await page.goto('');
	await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
}

function mainNavLink(page: Page, name: string): Locator {
	return page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name, exact: true });
}

async function focusInside(container: Locator): Promise<boolean> {
	return container.evaluate((element) => element.contains(document.activeElement));
}

async function expectFocusTrapped(page: Page, container: Locator): Promise<void> {
	for (let press = 0; 16 > press; press += 1) {
		await page.keyboard.press('Tab');
		expect(await focusInside(container)).toBe(true);
	}
	for (let press = 0; 16 > press; press += 1) {
		await page.keyboard.press('Shift+Tab');
		expect(await focusInside(container)).toBe(true);
	}
}

test('skip link is the first focusable element and moves focus into main', async ({ page }) => {
	await openConsole(page);
	await page.keyboard.press('Tab');
	const skipLink = page.getByRole('link', { name: 'Skip to content' });
	await expect(skipLink).toBeFocused();
	await expect(skipLink).toBeVisible();
	await page.keyboard.press('Enter');
	await expect
		.poll(() => page.evaluate(() => Boolean(document.activeElement?.closest('main'))))
		.toBe(true);
});

test('route change moves focus to the page h1 and sets a unique title', async ({ page }) => {
	await openConsole(page);
	await mainNavLink(page, 'Files').click();
	await expect(page.getByRole('heading', { level: 1 })).toBeFocused();
	await expect(page).toHaveTitle(/^.+ · callfs$/);
});

test('command palette traps focus and Escape returns focus to the trigger', async ({ page }) => {
	await openConsole(page);
	const trigger = mainNavLink(page, 'Overview');
	await trigger.focus();
	await page.keyboard.press('ControlOrMeta+k');
	const palette = page.getByRole('dialog');
	await expect(palette).toBeVisible();
	await expect(palette.locator('input').first()).toBeFocused();
	await expectFocusTrapped(page, palette);
	await page.keyboard.press('Escape');
	await expect(palette).toBeHidden();
	await expect(trigger).toBeFocused();
});

test('shortcut sheet dialog traps focus and Escape returns focus to the trigger', async ({
	page
}) => {
	await openConsole(page);
	const trigger = mainNavLink(page, 'Overview');
	await trigger.focus();
	await page.keyboard.press('?');
	const sheet = page.getByRole('dialog');
	await expect(sheet).toBeVisible();
	await expectFocusTrapped(page, sheet);
	await page.keyboard.press('Escape');
	await expect(sheet).toBeHidden();
	await expect(trigger).toBeFocused();
});
