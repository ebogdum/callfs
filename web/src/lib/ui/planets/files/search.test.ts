import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { countObjects, deletePrefix, listObjects, planPrefixDelete } from '$lib/api/objects';
import { search } from '$lib/api/search';
import type { FileEntry, SearchResult } from '$lib/api/types';
import { preferences } from '$lib/stores/preferences.svelte';
import FlatListPlanet from './FlatListPlanet.svelte';
import SearchPlanet from './SearchPlanet.svelte';

const route = vi.hoisted(() => ({ url: new URL('http://127.0.0.1/ui/files/search') }));

vi.mock('$app/state', () => ({ page: route }));
vi.mock('$app/navigation', () => ({ goto: vi.fn(() => Promise.resolve()) }));
vi.mock('$lib/api/search', () => ({ search: vi.fn() }));
vi.mock('$lib/api/objects', () => ({
	listObjects: vi.fn(),
	countObjects: vi.fn(),
	planPrefixDelete: vi.fn(),
	deletePrefix: vi.fn()
}));

const response = <T,>(data: T) => ({
	ok: true as const,
	value: { data, status: 200, headers: new Headers(), requestId: null, etag: null }
});

const fileEntry = (path: string): FileEntry => ({
	name: path.split('/').pop() ?? '',
	path,
	type: 'file',
	size: 2048,
	mode: '0644',
	owner: 'alice',
	etag: 'abc123',
	mtime: '2026-10-01T00:00:00Z',
	storage: 'localfs',
	instance_id: 'callfs-1',
	erasure_coded: false,
	locked: false
});

const searchPage = (overrides: Partial<SearchResult> = {}): SearchResult => ({
	items: [fileEntry('/logs/app.log')],
	next_cursor: null,
	took_ms: 400,
	scan_truncated: false,
	scanned: 1,
	facets: {
		type: { file: 1, directory: 0 },
		storage: { localfs: 1 },
		owner: { alice: 1 },
		instance: { 'callfs-1': 1 }
	},
	...overrides
});

const useRoute = (path: string, query: string) => {
	route.url = new URL(`http://127.0.0.1/ui/${path}${query}`);
};

describe('SearchPlanet', () => {
	beforeEach(() => {
		vi.mocked(search).mockReset();
	});

	it('shows the search tips before any query is typed', () => {
		useRoute('files/search', '');
		render(SearchPlanet);
		expect(screen.getByText('Start with a name, size or date.')).toBeInTheDocument();
	});

	it('shows the result count and time for a query', async () => {
		useRoute('files/search', '?q=name:*.log');
		vi.mocked(search).mockResolvedValue(response(searchPage()));
		render(SearchPlanet);
		expect(await screen.findByText('1 file in 0.4 seconds')).toBeInTheDocument();
	});

	it('replaces the count with the scan limit copy and an alert when the scan is truncated', async () => {
		useRoute('files/search', '?q=name:*.trunc');
		vi.mocked(search).mockResolvedValue(response(searchPage({ scan_truncated: true })));
		render(SearchPlanet);
		expect(await screen.findByText('More than 5,000 files. Narrow your search.')).toBeInTheDocument();
		expect(screen.getByText('Results are incomplete.')).toBeInTheDocument();
	});

	it('names the query in the no-results state', async () => {
		useRoute('files/search', '?q=name:*.nomatch');
		vi.mocked(search).mockResolvedValue(response(searchPage({ items: [], took_ms: 0 })));
		render(SearchPlanet);
		expect(await screen.findByText("No files match 'name:*.nomatch'.")).toBeInTheDocument();
		expect(screen.getByRole('button', { name: 'Clear filters' })).toBeInTheDocument();
	});

	it('shows the network error title when the search fails', async () => {
		useRoute('files/search', '?q=name:*.broken');
		vi.mocked(search).mockRejectedValue({ kind: 'network' });
		render(SearchPlanet);
		expect(await screen.findByText('We cannot reach callfs.')).toBeInTheDocument();
	});

	it('saves the query once and marks the save action as already saved', async () => {
		useRoute('files/search', '?q=name:*.keep');
		vi.mocked(search).mockResolvedValue(response(searchPage()));
		render(SearchPlanet);
		await screen.findByText('1 file in 0.4 seconds');
		await fireEvent.click(screen.getByRole('button', { name: 'Save search' }));
		expect(preferences.value.files.savedSearches).toContainEqual({ name: 'name:*.keep', query: 'name:*.keep' });
		expect(screen.getByRole('button', { name: 'Save search' })).toHaveAttribute('aria-disabled', 'true');
	});
});

describe('FlatListPlanet', () => {
	beforeEach(() => {
		vi.mocked(listObjects).mockReset();
		vi.mocked(countObjects).mockReset();
		vi.mocked(planPrefixDelete).mockReset();
		vi.mocked(deletePrefix).mockReset();
	});

	it('explains how the path start matches and lists the objects under it', async () => {
		useRoute('files/flat', '?prefix=/logs');
		vi.mocked(listObjects).mockResolvedValue(
			response({
				objects: [{ path: '/logs/app.log', size: 2048, etag: 'abc123', mtime: '2026-10-01T00:00:00Z' }],
				truncated: false
			})
		);
		vi.mocked(countObjects).mockResolvedValue(response({ count: 1, bytes: 2048, truncated: false }));
		render(FlatListPlanet);
		expect(
			screen.getByText('Matches every path that starts with these letters. /a also matches /ab.')
		).toBeInTheDocument();
		expect(await screen.findByText(/^1 total · /)).toBeInTheDocument();
	});

	it('shows the all-or-nothing copy when the listing is forbidden', async () => {
		useRoute('files/flat', '?prefix=/secret');
		vi.mocked(listObjects).mockRejectedValue({
			kind: 'http',
			status: 403,
			code: 'PERMISSION_DENIED',
			message: null,
			details: null,
			requestId: null,
			retryAfterS: null
		});
		vi.mocked(countObjects).mockResolvedValue(response({ count: 0, bytes: 0, truncated: false }));
		render(FlatListPlanet);
		expect(await screen.findByText('You cannot list one or more files here.')).toBeInTheDocument();
	});

	it('counts the delete plan before opening the typed confirmation', async () => {
		useRoute('files/flat', '?prefix=/tmp');
		vi.mocked(listObjects).mockResolvedValue(response({ objects: [], truncated: false }));
		vi.mocked(countObjects).mockResolvedValue(response({ count: 0, bytes: 0, truncated: false }));
		vi.mocked(planPrefixDelete).mockResolvedValue(response({ would_delete: 3, bytes: 4096, forbidden: 0 }));
		render(FlatListPlanet);
		await fireEvent.click(screen.getByRole('button', { name: 'Delete all under this path…' }));
		expect(await screen.findByText('3 objects and 4.1 kB will be deleted. This cannot be undone.')).toBeInTheDocument();
		expect(vi.mocked(deletePrefix)).not.toHaveBeenCalled();
	});
});
