import AxeBuilder from '@axe-core/playwright';
import { expect, type Page } from '@playwright/test';
import { cookieValueFor, MOCK_CSRF, SESSION_COOKIE, type Role } from './mocks/server';

const WCAG_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'];

type AxeResults = Awaited<ReturnType<AxeBuilder['analyze']>>;
export type AxeViolations = AxeResults['violations'];

export async function signIn(page: Page, role: Role): Promise<void> {
	await page.context().addCookies([
		{
			name: SESSION_COOKIE,
			value: cookieValueFor(role),
			domain: '127.0.0.1',
			path: '/',
			httpOnly: true,
			sameSite: 'Strict'
		}
	]);
	await page.addInitScript(`sessionStorage.setItem('callfs.csrf', '${MOCK_CSRF}');`);
}

export async function expectLandmarks(page: Page): Promise<void> {
	await expect(page.getByRole('banner')).toHaveCount(1);
	await expect(page.getByRole('main')).toHaveCount(1);
	await expect(page.getByRole('navigation', { name: 'Main' })).toHaveCount(1);
	await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
}

export async function axe(page: Page): Promise<AxeViolations> {
	const results = await new AxeBuilder({ page }).withTags(WCAG_TAGS).analyze();
	return results.violations.filter(
		(violation) => 'serious' === violation.impact || 'critical' === violation.impact
	);
}
