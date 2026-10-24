import type { Page, Route } from '@playwright/test';

export type Role = 'admin' | 'member';

export interface MockResponse {
	status: number;
	body: unknown;
}

export type MockTable = Record<string, MockResponse>;

interface MockModules {
	fixtures: MockTable;
	scenarios: Record<string, MockTable>;
}

export const SESSION_COOKIE = 'callfs_session';
export const MOCK_CSRF = 'mock-csrf';

const ROLES: readonly Role[] = ['admin', 'member'];
const ROLE_COOKIE_VALUE: Record<Role, string> = { admin: 'mock-admin', member: 'mock-member' };

const EMPTY_MODULES: MockModules = { fixtures: {}, scenarios: {} };

async function loadModules(): Promise<MockModules> {
	const [fixtures, scenarios] = await Promise.all([
		import('./fixtures').then(
			(loaded) => loaded.fixtures,
			(): MockTable => EMPTY_MODULES.fixtures
		),
		import('./scenarios').then(
			(loaded) => loaded.scenarios,
			(): Record<string, MockTable> => EMPTY_MODULES.scenarios
		)
	]);
	return { fixtures, scenarios };
}

export function cookieValueFor(role: Role): string {
	return ROLE_COOKIE_VALUE[role];
}

function patternToRegExp(pattern: string): RegExp {
	const source = pattern
		.split(/(\*|\{[^}]+\})/)
		.map((part) => {
			if ('*' === part) {
				return '.*';
			}
			if (part.startsWith('{') && part.endsWith('}')) {
				return '[^/]+';
			}
			return part.replace(/[.+?^${}()|[\]\\]/g, '\\$&');
		})
		.join('');
	return new RegExp(`^${source}$`);
}

function matchesKey(key: string, method: string, pathname: string): boolean {
	const [keyMethod = '', pattern = ''] = key.split(' ');
	return method === keyMethod && patternToRegExp(pattern).test(pathname);
}

function findEntry(table: MockTable, method: string, pathname: string): [string, MockResponse] | undefined {
	const matches = Object.entries(table)
		.filter(([key]) => matchesKey(key, method, pathname))
		.sort(([left], [right]) => right.length - left.length);
	return matches[0];
}

async function currentRole(page: Page): Promise<Role | null> {
	const cookies = await page.context().cookies();
	const session = cookies.find((cookie) => SESSION_COOKIE === cookie.name);
	if (undefined === session) {
		return null;
	}
	return ROLES.find((role) => ROLE_COOKIE_VALUE[role] === session.value) ?? null;
}

function asRecord(value: unknown): Record<string, unknown> {
	if ('object' === typeof value && null !== value) {
		return value as Record<string, unknown>;
	}
	return {};
}

async function resolveMe(page: Page, fallback: MockResponse): Promise<MockResponse> {
	const role = await currentRole(page);
	if (null === role) {
		return { status: 401, body: { code: 'UNAUTHORIZED', message: 'Sign in to continue.' } };
	}
	return {
		status: fallback.status,
		body: { ...asRecord(fallback.body), is_admin: 'admin' === role, csrf_token: MOCK_CSRF }
	};
}

async function answer(page: Page, route: Route, table: MockTable): Promise<void> {
	const request = route.request();
	const pathname = new URL(request.url()).pathname;
	const match = findEntry(table, request.method(), pathname);
	if (undefined === match) {
		await route.fulfill({
			status: 404,
			contentType: 'application/json',
			body: JSON.stringify({ code: 'NOT_FOUND', message: `No mock for ${request.method()} ${pathname}` })
		});
		return;
	}
	const [key, response] = match;
	const resolved = 'GET /v1/me' === key ? await resolveMe(page, response) : response;
	await route.fulfill({
		status: resolved.status,
		contentType: 'application/json',
		body: JSON.stringify(resolved.body ?? null)
	});
}

export async function installMocks(page: Page, scenario: string, overrides?: MockTable): Promise<void> {
	const modules = await loadModules();
	const table: MockTable = {
		...modules.fixtures,
		...(modules.scenarios[scenario] ?? {}),
		...(overrides ?? {})
	};
	const handler = (route: Route): Promise<void> => answer(page, route, table);
	await page.route('**/v1/**', handler);
	await page.route('**/health**', handler);
}
