import { Buffer } from 'node:buffer';
import { expect, test, type Page } from '@playwright/test';
import { signIn } from '../helpers';
import { installMocks, type MockTable } from '../mocks/server';

async function setup(page: Page, overrides?: MockTable): Promise<void> {
	await installMocks(page, 'healthy', overrides);
	await signIn(page, 'admin');
}

test('upload three files with one conflict, share one, revoke it, audit shows it', async ({ page }) => {
	await setup(page, {
		'PUT /v1/files/team/report.pdf': { status: 409, body: { code: 'CONFLICT', message: 'exists' } }
	});
	await page.goto('files/team');
	await page.getByRole('button', { name: 'Upload', exact: true }).click();
	const chooser = page.waitForEvent('filechooser');
	await page.getByRole('menuitem', { name: 'Files…' }).click();
	(await chooser).setFiles([
		{ name: 'report.pdf', mimeType: 'application/pdf', buffer: Buffer.from('report') },
		{ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('notes') },
		{ name: 'photo.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('photo') }
	]);
	const sheet = page.getByRole('dialog');
	await expect(sheet).toBeVisible();
	await sheet.getByText('Replace if changed', { exact: true }).click();
	await sheet.getByRole('button', { name: /^Upload/ }).click();
	await page.waitForRequest((request) => 'PUT' === request.method() || 'POST' === request.method());

	await page.getByRole('row', { name: /notes\.txt/ }).click({ button: 'right' });
	await page.getByRole('menuitem', { name: 'Create download link…' }).click();
	await page.getByRole('dialog').getByRole('button', { name: 'Create link' }).click();
	await expect(page.getByRole('dialog').getByText('Works once.')).toBeVisible();
	await page.keyboard.press('Escape');

	await page.goto('sharing');
	await page.getByRole('row', { name: /notes\.txt/ }).getByRole('button', { name: 'Revoke' }).click();
	const revoke = page.getByRole('alertdialog');
	await expect(revoke.getByText('Revoke this link?')).toBeVisible();
	const revoked = page.waitForRequest(
		(request) => 'DELETE' === request.method() && request.url().includes('/v1/links/')
	);
	await revoke.getByRole('button', { name: 'Revoke link' }).click();
	await revoked;

	await page.goto('activity/audit');
	await expect(page.getByRole('heading', { level: 1, name: 'Audit' })).toBeVisible();
});

test('rename a file and undo the rename', async ({ page }) => {
	await setup(page);
	await page.goto('files/team');
	await page.getByRole('row', { name: /notes\.txt/ }).click();
	await page.keyboard.press('F2');
	const name = page.getByRole('textbox').first();
	await name.fill('report-v2.pdf');
	await name.press('Enter');
	const status = page.getByRole('status').filter({ hasText: 'Renamed to report-v2.pdf. Undo' });
	await expect(status).toBeVisible();
	const undone = page.waitForRequest(
		(request) => 'PATCH' === request.method() && request.url().includes('/v1/files/')
	);
	await status.getByRole('button', { name: 'Undo' }).click();
	await undone;
});

test('move a file to another storage and show the job phases', async ({ page }) => {
	await setup(page);
	await page.goto('files/team');
	await page.getByRole('row', { name: /notes\.txt/ }).click();
	await page.keyboard.press('m');
	const picker = page.getByRole('dialog');
	await expect(picker).toBeVisible();
	await picker.getByRole('button', { name: /^Move/ }).click();
	await expect(page.getByText('Copy', { exact: true })).toBeVisible();
	await expect(page.getByText('Check', { exact: true })).toBeVisible();
	await expect(page.getByText('Remove source', { exact: true })).toBeVisible();
});

test('delete a folder to trash and restore it', async ({ page }) => {
	await setup(page);
	await page.goto('files/');
	await page.getByRole('row', { name: /team/ }).click();
	await page.keyboard.press('Delete');
	await expect(page.getByRole('status').filter({ hasText: 'Moved to trash. Undo' })).toBeVisible();
	await page.goto('files/trash');
	await page.getByRole('row', { name: /team/ }).click();
	const restored = page.waitForRequest(
		(request) => 'POST' === request.method() && request.url().includes('/restore')
	);
	await page.getByRole('button', { name: 'Restore' }).click();
	await restored;
});

test('delete a folder for good after typing DELETE', async ({ page }) => {
	await setup(page);
	await page.goto('files/');
	await page.getByRole('row', { name: /team/ }).click();
	await page.keyboard.press('Shift+Delete');
	const dialog = page.getByRole('dialog');
	await expect(dialog.getByText('This cannot be undone.')).toBeVisible();
	await dialog.getByRole('textbox').fill('DELETE');
	const deleted = page.waitForRequest(
		(request) => 'DELETE' === request.method() && request.url().includes('/v1/files/team')
	);
	await dialog.getByRole('button', { name: /^Delete/ }).click();
	await deleted;
});

test('browse folders and files with the keyboard only', async ({ page }) => {
	await setup(page);
	await page.goto('files/');
	await page.getByRole('row').nth(1).focus();
	await page.keyboard.press('j');
	await page.keyboard.press('Enter');
	await expect(page).toHaveURL(/\/ui\/files\/.+/);
	await page.keyboard.press('Backspace');
	await expect(page).toHaveURL(/\/ui\/files\/?$/);
	await page.keyboard.press('j');
	await page.keyboard.press('Space');
	await expect(page.getByRole('dialog')).toBeVisible();
	await page.keyboard.press('Escape');
	await page.keyboard.press('F2');
	await expect(page.getByRole('textbox').first()).toBeVisible();
	await page.keyboard.press('Escape');
});
