import { expect, test } from '@playwright/test';
import { signIn } from '../helpers';
import { installMocks } from '../mocks/server';

test('remove the leader blocked by quorum risk, then transfer leadership first', async ({ page }) => {
	await installMocks(page, 'healthy', {
		'DELETE /v1/admin/cluster/nodes/callfs-node-1': {
			status: 409,
			body: { code: 'QUORUM_RISK', message: 'Removing this member would lose quorum.' }
		},
		'POST /v1/admin/cluster/leadership-transfer': { status: 200, body: { status: 'transferred' } }
	});
	await signIn(page, 'admin');
	await page.goto('cluster/nodes');
	await page.getByRole('link', { name: 'callfs-node-1' }).first().click();
	await page.getByRole('button', { name: 'Remove' }).click();
	const dialog = page.getByRole('alertdialog');
	await expect(dialog.getByText('Members go from 3 to 2.')).toBeVisible();
	const transferred = page.waitForRequest(
		(request) => 'POST' === request.method() && request.url().includes('/v1/admin/cluster/leadership-transfer')
	);
	await dialog.getByRole('button', { name: 'Move the lead first' }).click();
	await transferred;
});
