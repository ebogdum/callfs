import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	createUser,
	deleteUser,
	owners,
	reassignOwner,
	rotateKey,
	updateUser,
	users,
	type PositionalMapping
} from '$lib/api/admin/users';
import type { ApiFailure } from '$lib/api/errors';
import type { UserInfo } from '$lib/api/types';
import { t } from '$lib/copy/t';
import { createMutation } from '$lib/data/mutation.svelte';
import { createQuery } from '$lib/data/query.svelte';
import type { UiError } from '$lib/ui/types';
import AccessPlanet from './AccessPlanet.svelte';
import PositionalMigration from './PositionalMigration.svelte';

vi.mock('$lib/api/admin/users', () => ({
	users: vi.fn(),
	createUser: vi.fn(),
	rotateKey: vi.fn(),
	updateUser: vi.fn(),
	deleteUser: vi.fn(),
	migratePositional: vi.fn(),
	owners: vi.fn(),
	reassignOwner: vi.fn()
}));
vi.mock('$lib/data/query.svelte', () => ({ createQuery: vi.fn() }));
vi.mock('$lib/data/mutation.svelte', () => ({
	createMutation: vi.fn(() => ({ pending: false, error: null, run: vi.fn(() => Promise.resolve(null)) }))
}));

interface QueryFixture {
	data?: unknown;
	status: string;
	error?: UiError | null;
	updatedAt?: number | null;
}

type QueryOptionsShape = { key: readonly unknown[]; fetch: (signal: AbortSignal) => Promise<unknown> };
type RunShape = (vars: unknown) => Promise<unknown>;

const fakeQuery = (fixture: QueryFixture) => ({
	data: fixture.data,
	status: fixture.status,
	error: fixture.error ?? null,
	updatedAt: fixture.updatedAt ?? null,
	refetch: vi.fn(() => Promise.resolve())
});

const useQueries = (usersFixture: QueryFixture, ownersFixture: QueryFixture = { data: [], status: 'success' }) => {
	vi.mocked(createQuery).mockImplementation(((options: { key: readonly unknown[] }) =>
		fakeQuery('users' === options.key[0] ? usersFixture : ownersFixture)) as unknown as typeof createQuery);
};

const queryOptionsFor = (key: string): QueryOptionsShape | undefined => {
	const call = vi
		.mocked(createQuery)
		.mock.calls.find(([candidate]) => 'object' === typeof candidate && key === (candidate as QueryOptionsShape).key[0]);
	return call?.[0] as unknown as QueryOptionsShape | undefined;
};

const runnerFor = (action: string): RunShape | undefined => {
	const call = vi.mocked(createMutation).mock.calls.find(([options]) => action === options.context.action);
	return call?.[0].run as unknown as RunShape | undefined;
};

const failure: ApiFailure = { kind: 'network' };

const loadError: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true
};

const userRecord = (overrides: Partial<UserInfo>): UserInfo => ({
	user_id: 'alice',
	source: 'api_key_users',
	index: null,
	key_hint: '…a1b2',
	key_fingerprint: 'sha256:0123456789abcdef',
	is_admin: false,
	owned_files: 12,
	owned_bytes: 4096,
	last_auth_at: '2026-10-23T10:00:00Z',
	requests_24h: 340,
	auth_failures_24h: 2,
	...overrides
});

const adminUser = userRecord({ user_id: 'alice', is_admin: true });
const positionalUser = userRecord({ user_id: 'api-user-0', source: 'api_keys', index: 0, key_hint: '…c3d4' });
const rootUser = userRecord({ user_id: 'root', source: 'reserved', is_admin: true, last_auth_at: null });
const proxyUser = userRecord({ user_id: 'internal-proxy', source: 'reserved', last_auth_at: null });

const usersPage = {
	items: [adminUser, positionalUser, rootUser, proxyUser],
	positional_count: 1
};

describe('AccessPlanet', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	afterEach(() => {
		cleanup();
	});

	it('renders the page heading and the add user action', () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		expect(screen.getByRole('heading', { level: 1, name: t('access.title') })).toBeTruthy();
		expect(screen.getByRole('button', { name: t('access.add.open') })).toBeTruthy();
	});

	it('lists managed users in the table and reserved identities apart', () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		const table = screen.getByRole('table', { name: t('access.table.label') });
		expect(within(table).getByLabelText('alice')).toBeTruthy();
		expect(within(table).queryByText('root')).toBeNull();
		expect(screen.getByRole('heading', { level: 2, name: t('access.system.title') })).toBeTruthy();
		expect(screen.getByText('root')).toBeTruthy();
		expect(screen.getByText('internal-proxy')).toBeTruthy();
	});

	it('warns about positional keys and offers the migration', () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		expect(screen.getByText(t('access.positional.title'))).toBeTruthy();
		expect(screen.getByRole('button', { name: t('access.positional.fix') })).toBeTruthy();
	});

	it('warns about files that have no owner', () => {
		useQueries(
			{ data: usersPage, status: 'success', updatedAt: Date.now() },
			{ data: [{ owner: '', files: 4, bytes: 10 }], status: 'success' }
		);
		render(AccessPlanet);
		expect(screen.getByText(t('access.unowned.title'))).toBeTruthy();
	});

	it('shows the empty state when no managed user exists', () => {
		useQueries({ data: { items: [], positional_count: 0 }, status: 'empty', updatedAt: Date.now() });
		render(AccessPlanet);
		expect(screen.getByText(t('access.empty.title'))).toBeTruthy();
	});

	it('does not show the empty state while the list loads', () => {
		useQueries({ status: 'loading' });
		render(AccessPlanet);
		expect(screen.queryByText(t('access.empty.title'))).toBeNull();
	});

	it('shows the error title when the user list fails', () => {
		useQueries({ status: 'error', error: loadError, updatedAt: null });
		render(AccessPlanet);
		expect(screen.getByText(loadError.title)).toBeTruthy();
	});

	it('blocks reserved user IDs in the add dialog', async () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		await fireEvent.click(screen.getByRole('button', { name: t('access.add.open') }));
		await tick();
		const dialog = screen.getByRole('dialog');
		await fireEvent.input(within(dialog).getByLabelText(t('access.add.user_id.label')), {
			target: { value: 'root' }
		});
		await tick();
		expect(within(dialog).getAllByText(t('access.add.user_id.reserved')).length).toBeGreaterThan(0);
		const submit = within(dialog).getByRole('button', { name: t('access.add.submit') });
		expect(submit.getAttribute('aria-disabled')).toBe('true');
	});

	it('creates a user through the API and returns the new key', async () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		const created = { user_id: 'bob', api_key: 'key-one' };
		vi.mocked(createUser).mockResolvedValueOnce({ ok: true, value: created });
		const run = runnerFor('access.action.add');
		await expect(run?.({ id: 'bob', isAdmin: false })).resolves.toEqual(created);
		expect(createUser).toHaveBeenCalledWith('bob', false);
	});

	it('rejects the mutation with the API failure instead of a success value', async () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		vi.mocked(rotateKey).mockResolvedValueOnce({ ok: false, error: failure });
		const run = runnerFor('access.action.rotate');
		await expect(run?.('alice')).rejects.toEqual(failure);
	});

	it('loads every owners page and passes the cursor forward', async () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		vi.mocked(owners)
			.mockResolvedValueOnce({ ok: true, value: { items: [{ owner: '', files: 4, bytes: 10 }], next_cursor: 'page-2' } })
			.mockResolvedValueOnce({ ok: true, value: { items: [{ owner: 'bob', files: 1, bytes: 2 }], next_cursor: null } });
		const fetchOwners = queryOptionsFor('owners')?.fetch;
		await expect(fetchOwners?.(new AbortController().signal)).resolves.toEqual([
			{ owner: '', files: 4, bytes: 10 },
			{ owner: 'bob', files: 1, bytes: 2 }
		]);
		expect(owners).toHaveBeenNthCalledWith(1, null);
		expect(owners).toHaveBeenNthCalledWith(2, 'page-2');
	});

	it('rejects the users query with the API failure', async () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		vi.mocked(users).mockResolvedValueOnce({ ok: false, error: failure });
		const fetchUsers = queryOptionsFor('users')?.fetch;
		await expect(fetchUsers?.(new AbortController().signal)).rejects.toEqual(failure);
	});

	it('routes admin, removal and transfer actions to the user API', async () => {
		useQueries({ data: usersPage, status: 'success', updatedAt: Date.now() });
		render(AccessPlanet);
		vi.mocked(updateUser).mockResolvedValueOnce({ ok: true, value: adminUser });
		vi.mocked(deleteUser).mockResolvedValueOnce({ ok: true, value: undefined });
		vi.mocked(reassignOwner).mockResolvedValueOnce({ ok: true, value: { job_id: 'job-7' } });
		await runnerFor('access.action.admin')?.({ id: 'alice', isAdmin: false });
		await runnerFor('access.action.remove')?.('bob');
		await runnerFor('access.action.transfer')?.({ from: '', to: 'alice', prefix: '/' });
		expect(updateUser).toHaveBeenCalledWith('alice', { is_admin: false });
		expect(deleteUser).toHaveBeenCalledWith('bob');
		expect(reassignOwner).toHaveBeenCalledWith('', 'alice', '/');
	});
});

describe('PositionalMigration', () => {
	const rows: UserInfo[] = [
		userRecord({ user_id: 'api-user-0', source: 'api_keys', index: 0, key_hint: '…c3d4' }),
		userRecord({ user_id: 'api-user-1', source: 'api_keys', index: 1, key_hint: '…e5f6' })
	];
	const reservedIds = ['root', 'internal-proxy'];

	const renderMigration = (
		overrides: Partial<{ error: UiError | null; onsubmit: (mapping: PositionalMapping[]) => void }> = {}
	) => {
		const onsubmit = overrides.onsubmit ?? vi.fn();
		render(PositionalMigration, {
			props: {
				open: true,
				rows,
				reservedIds,
				busy: false,
				error: overrides.error ?? null,
				onsubmit
			}
		});
		return onsubmit;
	};

	beforeEach(() => {
		cleanup();
	});

	it('asks for one user per key and blocks submit until every key has one', () => {
		renderMigration();
		const dialog = screen.getByRole('dialog');
		expect(within(dialog).getByLabelText(t('access.positional.field.label', { index: 1 }))).toBeTruthy();
		expect(within(dialog).getByLabelText(t('access.positional.field.label', { index: 2 }))).toBeTruthy();
		expect(within(dialog).getByText(t('access.positional.reason.missing'))).toBeTruthy();
		const submit = within(dialog).getByRole('button', { name: t('access.positional.submit') });
		expect(submit.getAttribute('aria-disabled')).toBe('true');
	});

	it('submits the mapping in key order once every key has its own user', async () => {
		const onsubmit = renderMigration();
		const dialog = screen.getByRole('dialog');
		await fireEvent.input(within(dialog).getByLabelText(t('access.positional.field.label', { index: 1 })), {
			target: { value: 'alice' }
		});
		await fireEvent.input(within(dialog).getByLabelText(t('access.positional.field.label', { index: 2 })), {
			target: { value: 'bob' }
		});
		await tick();
		await fireEvent.click(within(dialog).getByRole('button', { name: t('access.positional.submit') }));
		expect(onsubmit).toHaveBeenCalledWith([
			{ index: 0, user_id: 'alice' },
			{ index: 1, user_id: 'bob' }
		]);
	});

	it('explains duplicate and reserved user IDs before submit', async () => {
		renderMigration();
		const dialog = screen.getByRole('dialog');
		const first = within(dialog).getByLabelText(t('access.positional.field.label', { index: 1 }));
		const second = within(dialog).getByLabelText(t('access.positional.field.label', { index: 2 }));
		await fireEvent.input(first, { target: { value: 'alice' } });
		await fireEvent.input(second, { target: { value: 'alice' } });
		await tick();
		expect(within(dialog).getByText(t('access.positional.reason.duplicate'))).toBeTruthy();
		await fireEvent.input(second, { target: { value: 'root' } });
		await tick();
		expect(within(dialog).getByText(t('access.positional.reason.reserved'))).toBeTruthy();
	});

	it('shows the failure inside the dialog', () => {
		renderMigration({ error: { title: 'We could not fix the shared keys.', body: 'Try again.', retryable: false } });
		const dialog = screen.getByRole('dialog');
		expect(within(dialog).getByText('We could not fix the shared keys.')).toBeTruthy();
	});
});
