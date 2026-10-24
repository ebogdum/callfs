import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { CacheStats, Locks, RateLimits } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';
import { register } from '$lib/commands/registry.svelte';
import { createMutation } from '$lib/data/mutation.svelte';
import { createQuery, type Query } from '$lib/data/query.svelte';
import { goto } from '$app/navigation';
import CachePlanet from './CachePlanet.svelte';
import LimitsPlanet from './LimitsPlanet.svelte';
import LocksPlanet from './LocksPlanet.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn(() => Promise.resolve()) }));
vi.mock('$app/state', () => ({
	page: { url: new URL('https://callfs.test/ui/cluster/locks?path=%2Fa') }
}));
vi.mock('$lib/api/admin/runtime', () => ({
	locks: vi.fn(),
	forceRelease: vi.fn(),
	cache: vi.fn(),
	invalidateCache: vi.fn(),
	rateLimits: vi.fn()
}));
vi.mock('$lib/commands/registry.svelte', () => ({ register: vi.fn(() => () => undefined) }));
vi.mock('$lib/data/query.svelte', () => ({ createQuery: vi.fn() }));
vi.mock('$lib/data/mutation.svelte', () => ({ createMutation: vi.fn() }));

const lockData: Locks = {
	type: 'redis',
	wait_timeout_ms: 30000,
	ttl_ms: 10000,
	held: [
		{
			key: 'callfs:lock:a',
			path: '/a.txt',
			owner_hint: '…a1b2',
			acquired_at: '2026-10-23T10:00:00Z',
			expires_at: '2026-10-23T10:00:10Z'
		}
	],
	stats: {
		acquired: 10,
		timeouts: 2,
		cancelled: 0,
		errors: 0,
		lost: 1,
		renew_errors: 0,
		wait_p50_ms: 5,
		wait_p95_ms: 120
	}
};

const cacheData: CacheStats = {
	enabled: true,
	ttl_ms: 2000,
	entries: 812,
	max_entries: 1000,
	hits: 9512,
	misses: 488,
	evictions: 24,
	expired_cleaned: 3,
	last_cleanup_at: '2026-10-23T10:00:00Z'
};

const cacheOutcome = (value: CacheStats) => ({ ok: true as const, value });

const limitsData: RateLimits = {
	limits: [
		{ endpoint: 'download', rps: 10, burst: 5 },
		{ endpoint: 'link_generate', rps: 100, burst: 1 },
		{ endpoint: 'session', rps: 1, burst: 5 }
	],
	tracked_clients: 812,
	max_entries: 100000,
	ttl_s: 600,
	rejections_total: { download: 3, link_generate: 0, session: 7 },
	evictions_total: 0,
	top_clients: [
		{ client: '203.0.113.9', endpoint: 'session', rejections: 7, last_at: '2026-10-23T10:00:00Z' }
	]
};

const networkError: UiError = {
	title: 'We cannot reach callfs.',
	body: 'Check your connection. We will try again.',
	retryable: true
};

interface QueryInput<T> {
	data?: T;
	status: Query<T>['status'];
	error?: UiError | null;
	updatedAt?: number | null;
}

function mockQuery<T>(input: QueryInput<T>): ReturnType<typeof vi.fn> {
	const refetch = vi.fn(() => Promise.resolve());
	const state = {
		data: input.data,
		status: input.status,
		error: input.error ?? null,
		updatedAt: input.updatedAt ?? null,
		fetching: false,
		refetch,
		setData: vi.fn()
	};
	vi.mocked(createQuery).mockReturnValue(state as unknown as Query<unknown>);
	return refetch;
}

const COMMANDS_CONTEXT = 'callfs.commands';

function mockRegistry() {
	return { register: vi.fn(() => () => undefined) };
}

function mockMutation(result: unknown = null) {
	const run = vi.fn(() => Promise.resolve(result));
	const mutation = { run, pending: false, error: null };
	vi.mocked(createMutation).mockReturnValue(mutation as unknown as ReturnType<typeof createMutation>);
	return run;
}

afterEach(() => {
	cleanup();
});

beforeEach(() => {
	vi.clearAllMocks();
	mockMutation();
});

describe('LocksPlanet', () => {
	it('renders the title heading and no primary button in the success state', async () => {
		mockQuery({ data: lockData, status: 'success', updatedAt: Date.parse('2026-10-23T10:00:05Z') });
		render(LocksPlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Edit locks' })).toBeTruthy();
		expect(document.querySelectorAll('button[data-variant="primary"]')).toHaveLength(0);
	});

	it('shows held locks with the path, the KPI strip and the force release action', async () => {
		mockQuery({ data: lockData, status: 'success', updatedAt: Date.parse('2026-10-23T10:00:05Z') });
		render(LocksPlanet);
		await tick();
		expect(screen.getByText('Held now')).toBeTruthy();
		expect(screen.getByText('/a.txt')).toBeTruthy();
	});

	it('renders the empty copy when no lock is held', async () => {
		mockQuery({ data: { ...lockData, held: [] }, status: 'success' });
		render(LocksPlanet);
		await tick();
		expect(screen.getByText('No edit locks are held')).toBeTruthy();
	});

	it('renders the error state with the stale data kept when there is no data', async () => {
		mockQuery({ status: 'error', error: networkError });
		render(LocksPlanet);
		await tick();
		expect(screen.getByText('We cannot reach callfs.')).toBeTruthy();
		expect(screen.queryByText('Held now')).toBeNull();
	});

	it('retries the query from the error state', async () => {
		const refetch = mockQuery({ status: 'error', error: networkError });
		render(LocksPlanet);
		await tick();
		await fireEvent.click(screen.getByRole('button', { name: /try again|retry/i }));
		expect(refetch).toHaveBeenCalled();
	});

	it('renders the loading state without KPI content', async () => {
		mockQuery({ status: 'loading' });
		render(LocksPlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Edit locks' })).toBeTruthy();
		expect(screen.queryByText('Held now')).toBeNull();
	});

	it('registers the force release command on mount', async () => {
		mockQuery({ data: lockData, status: 'success' });
		const registry = mockRegistry();
		render(LocksPlanet, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		const [commands] = registry.register.mock.calls[0] as unknown as [{ id: string }[]];
		expect(commands.map((command) => command.id)).toContain('cluster.locks.force_release');
	});

	it('force releases the lock with a reason and the typed word', async () => {
		mockQuery({ data: lockData, status: 'success' });
		const run = mockMutation({ released: true });
		const registry = mockRegistry();
		render(LocksPlanet, { context: new Map([[COMMANDS_CONTEXT, registry]]) });
		await tick();
		const commands = (registry.register.mock.calls[0] as unknown[])[0] as {
			id: string;
			run: () => void;
		}[];
		commands.find((command) => 'cluster.locks.force_release' === command.id)?.run();
		await tick();
		expect(screen.getByText('Force release the lock on /a.txt?')).toBeTruthy();
		await fireEvent.input(screen.getByLabelText('Reason'), { target: { value: 'stuck writer' } });
		await fireEvent.input(screen.getByLabelText(/Type RELEASE to confirm/), { target: { value: 'RELEASE' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Force release' }));
		await tick();
		expect(run).toHaveBeenCalledWith({ key: 'callfs:lock:a', path: '/a.txt', reason: 'stuck writer' });
	});
});

describe('CachePlanet', () => {
	it('renders the title heading and exactly one primary button in the success state', async () => {
		mockQuery({ data: cacheOutcome(cacheData), status: 'success' });
		render(CachePlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Index cache' })).toBeTruthy();
		expect(document.querySelectorAll('button[data-variant="primary"]')).toHaveLength(1);
		expect(screen.getByRole('button', { name: 'Clear cache' })).toBeTruthy();
	});

	it('shows the hit and entry KPIs', async () => {
		mockQuery({ data: cacheOutcome(cacheData), status: 'success' });
		render(CachePlanet);
		await tick();
		expect(screen.getByRole('region', { name: 'Hits' })).toBeTruthy();
		expect(screen.getByRole('region', { name: 'Entries' })).toBeTruthy();
	});

	it('renders the off state when the cache is disabled', async () => {
		mockQuery({ data: cacheOutcome({ ...cacheData, enabled: false }), status: 'success' });
		render(CachePlanet);
		await tick();
		expect(screen.getByText('The index cache is off on this server.')).toBeTruthy();
	});

	it('renders the error state when there is no data', async () => {
		mockQuery({ status: 'error', error: networkError });
		render(CachePlanet);
		await tick();
		expect(screen.getByText('We cannot reach callfs.')).toBeTruthy();
	});

	it('renders the loading state with only the title', async () => {
		mockQuery({ status: 'loading' });
		render(CachePlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Index cache' })).toBeTruthy();
		expect(screen.queryByText('Hits')).toBeNull();
	});

	it('clears the whole cache only after the inline confirmation', async () => {
		mockQuery({ data: cacheOutcome(cacheData), status: 'success' });
		const run = mockMutation({ invalidated: 812 });
		render(CachePlanet);
		await tick();
		await fireEvent.click(screen.getByRole('button', { name: 'Clear cache' }));
		await tick();
		expect(run).not.toHaveBeenCalled();
		await fireEvent.click(screen.getByRole('button', { name: 'Clear all' }));
		await tick();
		expect(run).toHaveBeenCalledWith({ all: true });
	});

	it('registers the cache commands on mount', async () => {
		mockQuery({ data: cacheOutcome(cacheData), status: 'success' });
		render(CachePlanet);
		await tick();
		const [commands] = vi.mocked(register).mock.calls[0] as unknown as [{ id: string }[]];
		expect(commands.map((command) => command.id)).toEqual(
			expect.arrayContaining(['cluster.cache.clear', 'cluster.cache.clear_path'])
		);
	});
});

describe('LimitsPlanet', () => {
	it('renders the title heading and exactly one primary button in the success state', async () => {
		mockQuery({ data: limitsData, status: 'success' });
		render(LimitsPlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Request limits' })).toBeTruthy();
		expect(document.querySelectorAll('button[data-variant="primary"]')).toHaveLength(1);
		expect(screen.getByRole('button', { name: 'Change limits' })).toBeTruthy();
	});

	it('shows one card per endpoint with its rate', async () => {
		mockQuery({ data: limitsData, status: 'success' });
		render(LimitsPlanet);
		await tick();
		expect(screen.getByText('Download links')).toBeTruthy();
		expect(screen.getByText('Link creation')).toBeTruthy();
		expect(screen.getByRole('heading', { level: 2, name: 'Sign-in' })).toBeTruthy();
		expect(screen.getByText('10 per second, burst 5')).toBeTruthy();
	});

	it('navigates to the limits settings from the primary button', async () => {
		mockQuery({ data: limitsData, status: 'success' });
		render(LimitsPlanet);
		await tick();
		await fireEvent.click(screen.getByRole('button', { name: 'Change limits' }));
		expect(goto).toHaveBeenCalledWith('/ui/settings/limits');
	});

	it('renders the empty state when no limit is set', async () => {
		mockQuery({ data: { ...limitsData, limits: [] }, status: 'empty' });
		render(LimitsPlanet);
		await tick();
		expect(screen.getByText('No request limits are set')).toBeTruthy();
	});

	it('renders the partial alert above the success content', async () => {
		mockQuery({ data: limitsData, status: 'partial' });
		render(LimitsPlanet);
		await tick();
		expect(screen.getByText(/Only the busiest clients are listed\./)).toBeTruthy();
		expect(screen.getByText('Download links')).toBeTruthy();
	});

	it('renders the error state when there is no data', async () => {
		mockQuery({ status: 'error', error: networkError });
		render(LimitsPlanet);
		await tick();
		expect(screen.getByText('We cannot reach callfs.')).toBeTruthy();
	});

	it('renders the loading state with only the title', async () => {
		mockQuery({ status: 'loading' });
		render(LimitsPlanet);
		await tick();
		expect(screen.getByRole('heading', { level: 1, name: 'Request limits' })).toBeTruthy();
		expect(screen.queryByText('Download links')).toBeNull();
	});
});
