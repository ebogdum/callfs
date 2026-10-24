import { expect, test } from '@playwright/test';
import { signIn } from '../helpers';
import { fixtures } from '../mocks/fixtures';
import { installMocks } from '../mocks/server';

const HOLD_MS = 4000;

test('loading skeleton shows while the findings request is pending', async ({ page }) => {
	await installMocks(page, 'healthy');
	await page.route('**/v1/admin/findings**', async (route) => {
		await new Promise((resolve) => setTimeout(resolve, HOLD_MS));
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify(fixtures['GET /v1/admin/findings']?.body ?? null)
		});
	});
	await signIn(page, 'admin');
	await page.goto('');
	await expect(page.locator('[data-state="loading"]').first()).toBeVisible();
});

test('empty copy shows on a fresh install with no files', async ({ page }) => {
	await installMocks(page, 'fresh_install');
	await signIn(page, 'admin');
	await page.goto('files');
	await expect(page.getByText('No files yet')).toBeVisible();
});

test('error copy shows when every read fails', async ({ page }) => {
	await installMocks(page, 'errors_everywhere');
	await signIn(page, 'admin');
	await page.goto('');
	await expect(page.getByRole('alert').first()).toBeVisible();
});

test('partial alert shows when a huge folder listing is truncated', async ({ page }) => {
	await installMocks(page, 'huge_folder');
	await signIn(page, 'admin');
	await page.goto('files');
	await expect(page.getByText('Some items could not be listed.')).toBeVisible();
});

test('success content shows the storage totals on a healthy server', async ({ page }) => {
	await installMocks(page, 'healthy');
	await signIn(page, 'admin');
	await page.goto('');
	await expect(page.getByText(/48,210 files/).first()).toBeVisible();
});

test('degraded cluster banner shows when there is no leader', async ({ page }) => {
	await installMocks(page, 'degraded_no_leader');
	await signIn(page, 'admin');
	await page.goto('');
	await expect(page.getByText('callfs cannot accept changes right now.').first()).toBeVisible();
});

test('member sees no cluster navigation', async ({ page }) => {
	await installMocks(page, 'member_user');
	await signIn(page, 'member');
	await page.goto('files');
	await expect(page.getByRole('navigation', { name: 'Main' }).getByText('Cluster', { exact: true })).toHaveCount(0);
});
