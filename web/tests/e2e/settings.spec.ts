import { expect, test } from '@playwright/test';
import { signIn } from '../helpers';
import { installMocks } from '../mocks/server';

test('edit a network setting, fix a validation error, save and see the restart banner', async ({ page }) => {
	await installMocks(page, 'healthy');
	await signIn(page, 'admin');
	const invalid = {
		valid: false,
		errors: [{ path: 'server.listen_addr', message: 'Enter an address as host:port.' }],
		warnings: [],
		restart_required: ['server.listen_addr'],
		applies_now: []
	};
	const valid = { ...invalid, valid: true, errors: [] };
	let validations = 0;
	await page.route('**/v1/admin/config/validate', (route) => {
		validations += 1;
		return route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify(1 === validations ? invalid : valid)
		});
	});

	await page.goto('settings/network');
	const listen = page.getByLabel('Listen address');
	await listen.fill('not an address');
	await page.getByRole('button', { name: /Review and save/ }).click();
	await expect(page.getByRole('dialog').getByText('Enter an address as host:port.')).toBeVisible();
	await page.keyboard.press('Escape');

	await listen.fill(':9443');
	await page.getByRole('button', { name: /Review and save/ }).click();
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByText('Changed from :8443 to :9443')).toBeVisible();
	const saved = page.waitForRequest((request) => 'PUT' === request.method() && request.url().includes('/v1/admin/config'));
	await dialog.getByRole('button', { name: /^Save 1 change/ }).click();
	await saved;
	await expect(page.getByText('Saved. Restart to use the new values.')).toBeVisible();
	await expect(page.getByText('Restart callfs to use them.')).toBeVisible();
});
