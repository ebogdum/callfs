import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { upload } from '$lib/api/files';
import { abortUpload, createUpload, getUpload, uploadResumable } from '$lib/api/uploads';
import {
	createTransfer,
	etaSeconds,
	nextThroughput,
	parseRecords,
	planConflict,
	summarize,
	TransferStore,
	type Transfer
} from './transfers.svelte';

vi.mock('$lib/api/files', () => ({ upload: vi.fn(), wsDownload: vi.fn() }));
vi.mock('$lib/api/uploads', () => ({
	abortUpload: vi.fn(),
	createUpload: vi.fn(),
	getUpload: vi.fn(),
	uploadResumable: vi.fn()
}));
vi.mock('$lib/api/errors', () => ({
	toUiError: () => ({ title: 'Failed', body: 'Failed', retryable: true })
}));

const MIB = 1024 * 1024;
const RECORDS_KEY = 'callfs.transfers.resumable';

function pending(): Promise<never> {
	return new Promise<never>(() => undefined);
}

function bigFile(name: string, size: number): File {
	const file = new File(['x'], name);
	Object.defineProperty(file, 'size', { value: size });
	return file;
}

beforeEach(() => {
	vi.clearAllMocks();
	sessionStorage.clear();
	vi.mocked(upload).mockReturnValue(pending());
	vi.mocked(uploadResumable).mockReturnValue(pending());
	vi.mocked(createUpload).mockReturnValue(pending());
	vi.mocked(getUpload).mockReturnValue(pending());
	vi.mocked(abortUpload).mockResolvedValue({ ok: true, value: undefined });
});

afterEach(() => {
	vi.useRealTimers();
});

describe('planConflict', () => {
	it('uploads without asking when the name is free', () => {
		expect(planConflict(null, 'a.txt', 3, new Map())).toEqual({
			action: 'upload',
			name: 'a.txt',
			overwrite: false,
			ifMatch: null
		});
	});

	it('asks for a choice when the name is taken and no policy is set', () => {
		const listing = new Map([['a.txt', { size: 3, etag: 'e1' }]]);
		expect(planConflict(null, 'a.txt', 3, listing)).toEqual({ action: 'choose' });
	});

	it('keeps both under the next free name', () => {
		const listing = new Map([
			['a.txt', { size: 3, etag: 'e1' }],
			['a (2).txt', { size: 3, etag: 'e2' }]
		]);
		expect(planConflict('keep-both', 'a.txt', 3, listing)).toEqual({
			action: 'upload',
			name: 'a (3).txt',
			overwrite: false,
			ifMatch: null
		});
	});

	it('skips an unchanged file under replace-if-changed', () => {
		const listing = new Map([['a.txt', { size: 3, etag: 'e1' }]]);
		expect(planConflict('replace-if-changed', 'a.txt', 3, listing)).toEqual({ action: 'skip' });
	});

	it('replaces a changed file only against the existing version tag', () => {
		const listing = new Map([['a.txt', { size: 9, etag: 'e1' }]]);
		expect(planConflict('replace-if-changed', 'a.txt', 3, listing)).toEqual({
			action: 'upload',
			name: 'a.txt',
			overwrite: true,
			ifMatch: 'e1'
		});
	});
});

describe('throughput, ETA and aggregate', () => {
	it('starts the throughput at the first sample and blends later samples', () => {
		expect(nextThroughput(null, 200, 2)).toBe(100);
		expect(nextThroughput(100, 200, 1)).toBeCloseTo(130);
	});

	it('derives ETA from remaining bytes and throughput', () => {
		expect(etaSeconds(1000, 100)).toBe(10);
		expect(etaSeconds(1000, 0)).toBeNull();
		expect(etaSeconds(null, 100)).toBeNull();
	});

	it('sums done, total, throughput and ETA across unfinished transfers', () => {
		const first: Transfer = {
			...createTransfer({ id: 'a', direction: 'upload', name: 'a.txt', destination: '/team/a.txt', total: 100, existing: null }),
			state: 'uploading',
			done: 10,
			bytesPerSecond: 5
		};
		const second: Transfer = {
			...createTransfer({ id: 'b', direction: 'upload', name: 'b.txt', destination: '/team/b.txt', total: 100, existing: null }),
			state: 'uploading',
			done: 30,
			bytesPerSecond: 5
		};
		expect(summarize([first, second])).toEqual({ done: 40, total: 200, bytesPerSecond: 10, eta: 16 });
	});
});

describe('parseRecords', () => {
	it('returns no records for missing or corrupt storage', () => {
		expect(parseRecords(null)).toEqual([]);
		expect(parseRecords('{broken')).toEqual([]);
	});

	it('keeps well-formed records and drops malformed ones', () => {
		const valid = { uploadId: 'upload-1', directory: '/team', name: 'big.bin', size: 65 * MIB };
		expect(parseRecords(JSON.stringify([valid, { uploadId: 7 }]))).toEqual([valid]);
	});
});

describe('TransferStore', () => {
	it('holds a name clash until a policy is chosen, then applies it', async () => {
		const store = new TransferStore();
		const listing = new Map([['a.txt', { size: 9, etag: 'e1' }]]);
		store.enqueueUploads([new File(['x'], 'a.txt')], '/team', listing, null);
		const id = store.items[0]?.id ?? '';
		expect(store.items[0]?.state).toBe('conflict');
		expect(store.items[0]?.existing).toEqual({ size: 9, etag: 'e1' });
		store.resolveConflict(id, 'skip');
		expect(store.items[0]?.state).toBe('cancelled');
		expect(vi.mocked(upload)).not.toHaveBeenCalled();
	});

	it('moves a server 409 into the conflict state instead of failing', async () => {
		vi.mocked(upload).mockResolvedValue({
			ok: false,
			error: { kind: 'http', status: 409, code: 'FILE_ALREADY_EXISTS', message: null, details: null, requestId: null, retryAfterS: null }
		});
		const store = new TransferStore();
		store.enqueueUploads([new File(['x'], 'b.txt')], '/team', new Map(), null);
		await vi.waitFor(() => {
			expect(store.items[0]?.state).toBe('conflict');
		});
		expect(store.items[0]?.error).toBeNull();
	});

	it('runs no more uploads at once than the parallel limit', () => {
		const store = new TransferStore();
		store.setParallel(1);
		store.enqueueUploads([new File(['a'], 'a.txt'), new File(['b'], 'b.txt')], '/team', new Map(), null);
		expect(store.items.map((item) => item.state)).toEqual(['uploading', 'queued']);
	});

	it('clamps the parallel limit between 1 and 6', () => {
		const store = new TransferStore();
		store.setParallel(9);
		expect(store.settings.parallel).toBe(6);
		store.setParallel(0);
		expect(store.settings.parallel).toBe(1);
	});

	it('pauses an upload by aborting its request and resumes it', () => {
		const store = new TransferStore();
		store.enqueueUploads([new File(['a'], 'a.txt')], '/team', new Map(), null);
		const id = store.items[0]?.id ?? '';
		store.pause(id);
		expect(store.items[0]?.state).toBe('paused');
		expect(vi.mocked(upload).mock.calls[0]?.[2]?.signal?.aborted).toBe(true);
		store.resume(id);
		expect(store.items[0]?.state).toBe('uploading');
	});

	it('marks a transfer stalled after 15 seconds without bytes', () => {
		vi.useFakeTimers();
		const store = new TransferStore();
		store.enqueueUploads([new File(['a'], 'a.txt')], '/team', new Map(), null);
		vi.advanceTimersByTime(16_000);
		expect(store.items[0]?.state).toBe('stalled');
	});

	it('rejects files over 10 GB before any upload starts', () => {
		const store = new TransferStore();
		const result = store.enqueueUploads([bigFile('huge.bin', 11 * 1024 * MIB)], '/team', new Map(), null);
		expect(result.rejected).toHaveLength(1);
		expect(store.items).toHaveLength(0);
		expect(vi.mocked(upload)).not.toHaveBeenCalled();
	});

	it('records a resumable session in sessionStorage for files over 64 MB', async () => {
		vi.mocked(createUpload).mockResolvedValue({
			ok: true,
			value: { upload_id: 'upload-1', offset: 0, part_bytes: 8 * MIB, expires_at: '2026-10-24T00:00:00Z' }
		});
		const store = new TransferStore();
		store.enqueueUploads([bigFile('big.bin', 65 * MIB)], '/team', new Map(), null);
		await vi.waitFor(() => {
			expect(store.items[0]?.resumeId).toBe('upload-1');
		});
		expect(JSON.parse(sessionStorage.getItem(RECORDS_KEY) ?? '[]')).toEqual([
			{ uploadId: 'upload-1', directory: '/team', name: 'big.bin', size: 65 * MIB }
		]);
	});

	it('cancels a resumable session on the server and drops it from sessionStorage', async () => {
		vi.mocked(createUpload).mockResolvedValue({
			ok: true,
			value: { upload_id: 'upload-1', offset: 0, part_bytes: 8 * MIB, expires_at: '2026-10-24T00:00:00Z' }
		});
		const store = new TransferStore();
		store.enqueueUploads([bigFile('big.bin', 65 * MIB)], '/team', new Map(), null);
		await vi.waitFor(() => {
			expect(store.items[0]?.resumeId).toBe('upload-1');
		});
		await store.cancel(store.items[0]?.id ?? '');
		expect(store.items[0]?.state).toBe('cancelled');
		expect(vi.mocked(abortUpload)).toHaveBeenCalledWith('upload-1');
		expect(sessionStorage.getItem(RECORDS_KEY)).toBe('[]');
	});
});
