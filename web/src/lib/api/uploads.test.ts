import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { request } from './http';
import { abortUpload, completeUpload, createUpload, uploadResumable } from './uploads';
import type { UploadSession } from './types';

vi.mock('./http', () => ({ request: vi.fn(), nodePath: (path: string) => path }));
vi.mock('./errors', () => ({
	httpFailure: (status: number, _body: string, _retryAfter: string | null, requestId: string | null) => ({
		kind: 'http',
		status,
		code: null,
		message: null,
		details: null,
		requestId,
		retryAfterS: null
	})
}));

const requestMock = vi.mocked(request);
const fetchMock = vi.fn<typeof fetch>();

const ok = <T>(data: T) => ({
	ok: true as const,
	value: { data, status: 200, headers: new Headers(), requestId: null, etag: null }
});

const session = (overrides: Partial<UploadSession>): UploadSession => ({
	upload_id: 'up-1',
	path: '/a.bin',
	offset: 0,
	size: 10,
	state: 'open',
	expires_at: '2026-10-24T00:00:00Z',
	...overrides
});

const created = (upload_id: string): { upload_id: string; offset: number; part_bytes: number; expires_at: string } => ({
	upload_id,
	offset: 0,
	part_bytes: 4,
	expires_at: '2026-10-24T00:00:00Z'
});

const completed = { path: '/a.bin', etag: 'etag-1', created: true };

const fileOf = (bytes: number): File => new File([new Uint8Array(bytes)], 'a.bin');

const ackWith = (offset: number): Response => new Response(JSON.stringify({ offset }), { status: 200 });

const rangesSent = (): string[] =>
	fetchMock.mock.calls.map(([, init]) => (init?.headers as Record<string, string>)['Content-Range']);

beforeEach(() => {
	vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
	vi.unstubAllGlobals();
	requestMock.mockReset();
	fetchMock.mockReset();
});

describe('upload session endpoints', () => {
	it('posts the session, completes it and aborts it on their endpoints', async () => {
		requestMock.mockResolvedValueOnce(ok(created('up-1')));
		requestMock.mockResolvedValueOnce(ok(completed));
		requestMock.mockResolvedValueOnce(ok(undefined));

		const create = await createUpload('/a.bin', 10, false);
		const complete = await completeUpload('up-1');
		const abort = await abortUpload('up-1');

		expect(create).toEqual({ ok: true, value: created('up-1') });
		expect(complete).toEqual({ ok: true, value: completed });
		expect(abort).toEqual({ ok: true, value: undefined });
		expect(requestMock.mock.calls[0]).toEqual([
			'/v1/uploads',
			{ method: 'POST', body: { path: '/a.bin', size: 10, overwrite: false, if_match: null, mode: '0644', storage: null, erasure: null } }
		]);
		expect(requestMock.mock.calls[1]).toEqual(['/v1/uploads/up-1/complete', { method: 'POST' }]);
		expect(requestMock.mock.calls[2]).toEqual(['/v1/uploads/up-1', { method: 'DELETE' }]);
	});

	it('returns the failure as a value when the create call fails', async () => {
		requestMock.mockResolvedValueOnce({ ok: false, error: { kind: 'network' } });

		const create = await createUpload('/a.bin', 10, false);

		expect(create).toEqual({ ok: false, error: { kind: 'network' } });
	});
});

describe('uploadResumable', () => {
	it('sends parts in order with Content-Range, reports progress and completes', async () => {
		const onProgress = vi.fn();
		requestMock.mockResolvedValueOnce(ok(created('up-1')));
		requestMock.mockResolvedValueOnce(ok(completed));
		fetchMock.mockResolvedValueOnce(ackWith(4));
		fetchMock.mockResolvedValueOnce(ackWith(8));
		fetchMock.mockResolvedValueOnce(ackWith(10));

		const result = await uploadResumable('/a.bin', fileOf(10), { partBytes: 4, parallel: 2, onProgress });

		expect(result).toEqual({ ok: true, value: completed });
		expect(rangesSent()).toEqual(['bytes 0-3/10', 'bytes 4-7/10', 'bytes 8-9/10']);
		expect(onProgress.mock.calls).toEqual([
			[4, 10],
			[8, 10],
			[10, 10]
		]);
	});

	it('resumes from the offset reported by GET when the session is open', async () => {
		requestMock.mockResolvedValueOnce(ok(session({ offset: 4 })));
		requestMock.mockResolvedValueOnce(ok(completed));
		fetchMock.mockResolvedValueOnce(ackWith(10));

		const result = await uploadResumable('/a.bin', fileOf(10), { resumeId: 'up-1' });

		expect(result).toEqual({ ok: true, value: completed });
		expect(rangesSent()).toEqual(['bytes 4-9/10']);
	});

	it('starts a new session when the resumed session has expired', async () => {
		requestMock.mockResolvedValueOnce(ok(session({ state: 'expired' })));
		requestMock.mockResolvedValueOnce(ok(created('up-2')));
		requestMock.mockResolvedValueOnce(ok(completed));
		fetchMock.mockResolvedValueOnce(ackWith(10));

		const result = await uploadResumable('/a.bin', fileOf(10), { partBytes: 10, resumeId: 'up-1' });

		expect(result).toEqual({ ok: true, value: completed });
		expect(fetchMock.mock.calls[0]?.[0]).toBe('/v1/uploads/up-2');
	});

	it('retries a failed part with backoff, resyncs the offset and then succeeds', async () => {
		requestMock.mockResolvedValueOnce(ok(created('up-1')));
		requestMock.mockResolvedValueOnce(ok(session({ offset: 0 })));
		requestMock.mockResolvedValueOnce(ok(session({ offset: 0 })));
		requestMock.mockResolvedValueOnce(ok(completed));
		fetchMock.mockResolvedValueOnce(new Response('{}', { status: 503 }));
		fetchMock.mockRejectedValueOnce(new TypeError('network'));
		fetchMock.mockResolvedValueOnce(ackWith(10));

		const result = await uploadResumable('/a.bin', fileOf(10), { partBytes: 10 });

		expect(result).toEqual({ ok: true, value: completed });
		expect(fetchMock).toHaveBeenCalledTimes(3);
	}, 10_000);

	it('gives up after three retries of the same part', async () => {
		requestMock.mockResolvedValueOnce(ok(created('up-1')));
		requestMock.mockResolvedValueOnce(ok(session({ offset: 0 })));
		requestMock.mockResolvedValueOnce(ok(session({ offset: 0 })));
		requestMock.mockResolvedValueOnce(ok(session({ offset: 0 })));
		fetchMock.mockImplementation(async () => new Response('{}', { status: 503 }));

		const result = await uploadResumable('/a.bin', fileOf(10), { partBytes: 10 });

		expect(result).toEqual({ ok: false, error: { kind: 'http', status: 503, code: null, message: null, details: null, requestId: null, retryAfterS: null } });
		expect(fetchMock).toHaveBeenCalledTimes(4);
	}, 10_000);

	it('does not retry a rejected part and returns its failure', async () => {
		requestMock.mockResolvedValueOnce(ok(created('up-1')));
		fetchMock.mockResolvedValueOnce(new Response('{}', { status: 413 }));

		const result = await uploadResumable('/a.bin', fileOf(10), { partBytes: 10 });

		expect(result.ok).toBe(false);
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it('stops with an aborted failure when the signal is aborted during a part', async () => {
		const controller = new AbortController();
		requestMock.mockResolvedValueOnce(ok(created('up-1')));
		fetchMock.mockImplementation(
			(_url, init) =>
				new Promise<Response>((_resolve, reject) => {
					init?.signal?.addEventListener('abort', () => {
						reject(new DOMException('aborted', 'AbortError'));
					});
				})
		);

		const pending = uploadResumable('/a.bin', fileOf(10), { partBytes: 10, signal: controller.signal });
		await vi.waitFor(() => {
			expect(fetchMock).toHaveBeenCalled();
		});
		controller.abort();

		expect(await pending).toEqual({ ok: false, error: { kind: 'aborted' } });
	});
});
