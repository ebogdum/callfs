import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ApiFailure } from '$lib/api/errors';
import type { ApiResponse } from '$lib/api/http';
import { rename } from '$lib/api/files';
import type { BatchResult, DeletePlan, FileEntry, Result } from '$lib/api/types';
import type { BrowseState } from './browse-state.svelte';
import {
	batchFailures,
	createFileActions,
	isCrossStorage,
	requiresTypedDelete,
	sumPlans,
	uniqueFolderName
} from './file-actions.svelte';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

vi.mock('$lib/api/files', () => ({
	createFolder: vi.fn(),
	deletePermanent: vi.fn(),
	move: vi.fn(),
	planDelete: vi.fn(),
	readRangeBlob: vi.fn(),
	rename: vi.fn(),
	setMode: vi.fn(),
	setOwner: vi.fn(),
	trash: vi.fn()
}));

vi.mock('$lib/api/batch', () => ({ batch: vi.fn() }));
vi.mock('$lib/api/erasure', () => ({ repair: vi.fn(), verify: vi.fn() }));
vi.mock('$lib/api/links', () => ({ generateLink: vi.fn() }));
vi.mock('$lib/api/search', () => ({ permissionCheck: vi.fn(), usage: vi.fn() }));
vi.mock('$lib/api/trash', () => ({ restore: vi.fn() }));
vi.mock('$lib/api/admin/users', () => ({ users: vi.fn() }));
vi.mock('$lib/data/cache', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/data/cache')>()),
	invalidate: vi.fn()
}));
vi.mock('$lib/stores/session.svelte', () => ({ session: { isAdmin: false, me: null } }));
vi.mock('$lib/stores/transfers.svelte', () => ({
	transfers: { download: vi.fn(), enqueueUploads: vi.fn(() => ({ queued: [], rejected: [] })) }
}));

const response = <T>(data: T): ApiResponse<T> => ({
	data,
	status: 200,
	headers: new Headers(),
	requestId: null,
	etag: null
});

const fileEntry = (overrides: Partial<FileEntry> = {}): FileEntry => ({
	name: 'a.txt',
	path: '/team/a.txt',
	type: 'file',
	size: 10,
	mode: '644',
	owner: 'alice',
	mtime: '2026-10-01T00:00:00Z',
	etag: 'etag-a',
	storage: 'localfs',
	instance_id: 'self',
	erasure_coded: false,
	locked: false,
	...overrides
});

const plan = (overrides: Partial<DeletePlan> = {}): DeletePlan => ({
	files: 1,
	directories: 0,
	bytes: 1,
	erasure_files: 0,
	remote_instances: [],
	...overrides
});

const browseState = (entries: FileEntry[]): BrowseState =>
	({
		path: '/team',
		entries,
		selection: { mode: 'multi', ids: new Set(entries.map((entry) => entry.path)), allMatching: false },
		q: '',
		view: 'list',
		showHidden: false,
		navigate: vi.fn()
	}) as unknown as BrowseState;

describe('typed delete threshold', () => {
	it('does not require the typed word at exactly 100 files and 1 GB', () => {
		expect(requiresTypedDelete(plan({ files: 100, bytes: 1024 ** 3 }))).toBe(false);
	});

	it('requires the typed word above 100 files', () => {
		expect(requiresTypedDelete(plan({ files: 101 }))).toBe(true);
	});

	it('requires the typed word above 1 GB', () => {
		expect(requiresTypedDelete(plan({ bytes: 1024 ** 3 + 1 }))).toBe(true);
	});
});

describe('pure helpers', () => {
	it('sums delete plans and keeps each remote instance once', () => {
		const total = sumPlans([
			plan({ files: 2, directories: 1, bytes: 5, remote_instances: ['b'] }),
			plan({ files: 3, bytes: 7, remote_instances: ['b', 'c'] })
		]);
		expect(total).toEqual({ files: 5, directories: 1, bytes: 12, erasure_files: 0, remote_instances: ['b', 'c'] });
	});

	it('counts only batch results at status 400 or above as failures', () => {
		const result: BatchResult = {
			results: [
				{ index: 0, status: 200, code: null, message: null, path: '/a' },
				{ index: 1, status: 409, code: 'CONFLICT', message: null, path: '/b' }
			]
		};
		expect(batchFailures(result)).toBe(1);
	});

	it('picks the first free name for a new folder', () => {
		expect(uniqueFolderName('New folder', ['a'])).toBe('New folder');
		expect(uniqueFolderName('New folder', ['New folder', 'New folder 2'])).toBe('New folder 3');
	});

	it('treats an empty storage or server as the current one', () => {
		expect(isCrossStorage({ overwrite: false, createParents: false })).toBe(false);
		expect(isCrossStorage({ overwrite: false, createParents: false, storage: '', server: '' })).toBe(false);
		expect(isCrossStorage({ overwrite: false, createParents: false, server: 'callfs-2' })).toBe(true);
	});
});

describe('createFileActions', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('starts with no drawer open', () => {
		const actions = createFileActions(browseState([fileEntry()]));
		expect(actions.drawer).toBeNull();
	});

	it('offers verify and repair only for split files', () => {
		const actions = createFileActions(browseState([fileEntry()]));
		const plain = actions.rowItems(fileEntry()).map((item) => item.id);
		const split = actions.rowItems(fileEntry({ erasure_coded: true })).map((item) => item.id);
		expect(plain).not.toContain('verify');
		expect(split).toEqual(expect.arrayContaining(['verify', 'repair']));
	});

	it('marks delete for good as a danger item and disables download on folders', () => {
		const folder = fileEntry({ name: 'docs', path: '/team/docs', type: 'directory' });
		const actions = createFileActions(browseState([folder]));
		const items = actions.rowItems(folder);
		expect(items.find((item) => 'delete' === item.id)?.tone).toBe('danger');
		expect(items.find((item) => 'download' === item.id)?.disabledReason).toBeTruthy();
	});

	it('labels bulk actions with the selected count', () => {
		const entries = [fileEntry(), fileEntry({ name: 'b.txt', path: '/team/b.txt' })];
		const actions = createFileActions(browseState(entries));
		expect(actions.selected).toHaveLength(2);
		expect(actions.bulkItems().find((item) => 'bulk-trash' === item.id)?.label).toBe('Move 2 to trash');
	});

	it('renames through the API and reports success', async () => {
		vi.mocked(rename).mockResolvedValue({ ok: true, value: response(undefined) } as Result<ApiResponse<void>, ApiFailure>);
		const entry = fileEntry();
		const actions = createFileActions(browseState([entry]));
		const outcome = await actions.rename(entry, 'b.txt');
		expect(rename).toHaveBeenCalledWith('/team/a.txt', 'b.txt');
		expect(outcome.ok).toBe(true);
	});

	it('returns an error result when the rename is refused', async () => {
		vi.mocked(rename).mockResolvedValue({
			ok: false,
			error: { kind: 'http', status: 409, code: 'FILE_ALREADY_EXISTS', message: null, details: null, requestId: null, retryAfterS: null }
		});
		const entry = fileEntry();
		const actions = createFileActions(browseState([entry]));
		const outcome = await actions.rename(entry, 'b.txt');
		expect(outcome.ok).toBe(false);
	});
});
