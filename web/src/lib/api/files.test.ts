import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('$lib/stores/session.svelte', () => ({
	session: { onUnauthorized: async () => false }
}));

import { batch } from './batch';
import { createFolder, deletePermanent, listDir, readRange, stat, trash, upload, wsDownload } from './files';
import { countObjects } from './objects';
import { search } from './search';

interface FakeProgressEvent {
	loaded: number;
	total: number;
}

class FakeXhr {
	static instances: FakeXhr[] = [];
	method = '';
	url = '';
	headers: Record<string, string> = {};
	status = 0;
	responseText = '';
	responseHeaders: Record<string, string> = {};
	aborted = false;
	upload: { onprogress: ((event: FakeProgressEvent) => void) | null } = { onprogress: null };
	onload: (() => void) | null = null;
	onerror: (() => void) | null = null;
	onabort: (() => void) | null = null;

	constructor() {
		FakeXhr.instances.push(this);
	}

	open(method: string, url: string): void {
		this.method = method;
		this.url = url;
	}

	setRequestHeader(name: string, value: string): void {
		this.headers[name] = value;
	}

	getResponseHeader(name: string): string | null {
		return this.responseHeaders[name] ?? null;
	}

	send(): void {}

	abort(): void {
		this.aborted = true;
		this.onabort?.();
	}
}

class FakeSocket {
	static last: FakeSocket | null = null;
	url: string;
	binaryType = '';
	onmessage: ((event: { data: unknown }) => void) | null = null;
	onerror: (() => void) | null = null;
	onclose: (() => void) | null = null;

	constructor(url: string) {
		this.url = url;
		FakeSocket.last = this;
	}
}

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json', ...headers }
	});
}

describe('files api', () => {
	let fetchMock: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		fetchMock = vi.fn();
		vi.stubGlobal('fetch', fetchMock);
		vi.stubGlobal('XMLHttpRequest', FakeXhr);
		vi.stubGlobal('WebSocket', FakeSocket);
		FakeXhr.instances = [];
		FakeSocket.last = null;
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	it('stat requests the stat route and returns the parsed body with its etag', async () => {
		fetchMock.mockResolvedValueOnce(jsonResponse({ name: 'b.txt', type: 'file' }, 200, { ETag: '"v1"' }));
		const result = await stat('/a/b.txt');
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/stat/a/b.txt');
		expect(result).toMatchObject({ ok: true, value: { data: { name: 'b.txt' }, etag: '"v1"' } });
	});

	it('listDir sends paging, sort and hidden flags as query parameters on a directory path', async () => {
		fetchMock.mockResolvedValueOnce(jsonResponse({ path: '/team', items: [] }));
		await listDir('/team', { limit: 200, sort: 'name', order: 'asc', includeHidden: false });
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/directories/team/?limit=200&sort=name&order=asc&include_hidden=false');
	});

	it('readRange sends a Range header and returns the body as an ArrayBuffer', async () => {
		fetchMock.mockResolvedValueOnce(new Response(new Uint8Array([1, 2, 3]), { status: 206 }));
		const result = await readRange('/a/b.bin', 0, 2);
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect((init.headers as Record<string, string>).Range).toBe('bytes=0-2');
		expect(result.ok ? result.value.byteLength : -1).toBe(3);
	});

	it('readRange maps a 416 response to an http failure', async () => {
		fetchMock.mockResolvedValueOnce(new Response('{"code":"RANGE_NOT_SATISFIABLE"}', { status: 416 }));
		const result = await readRange('/a/b.bin', 10, 20);
		expect(result).toMatchObject({ ok: false, error: { kind: 'http', status: 416 } });
	});

	it('upload posts with progress events, an octet-stream content type and resolves the etag', async () => {
		const progress: [number, number][] = [];
		const pending = upload('/a/new.txt', new Blob(['hello']), {
			onProgress: (loaded, total) => progress.push([loaded, total])
		});
		const xhr = FakeXhr.instances[0];
		xhr.upload.onprogress?.({ loaded: 5, total: 10 });
		xhr.responseHeaders = { ETag: '"e1"' };
		xhr.status = 201;
		xhr.onload?.();
		const result = await pending;
		expect(xhr.method).toBe('POST');
		expect(xhr.url).toBe('/v1/files/a/new.txt');
		expect(xhr.headers['Content-Type']).toBe('application/octet-stream');
		expect(progress).toEqual([[5, 10]]);
		expect(result).toEqual({ ok: true, value: { etag: '"e1"', created: true, requestId: null } });
	});

	it('upload with overwrite uses PUT, sends If-Match and erasure headers', async () => {
		const pending = upload('/a/x.bin', new Blob(['x']), {
			overwrite: true,
			ifMatch: '"v1"',
			erasure: { dataShards: 4, parityShards: 2, instances: ['node-a', 'node-b'] }
		});
		const xhr = FakeXhr.instances[0];
		xhr.status = 200;
		xhr.onload?.();
		await pending;
		expect(xhr.method).toBe('PUT');
		expect(xhr.headers['If-Match']).toBe('"v1"');
		expect(xhr.headers['X-CallFS-Erasure']).toBe('true');
		expect(xhr.headers['X-CallFS-Erasure-Data-Shards']).toBe('4');
		expect(xhr.headers['X-CallFS-Erasure-Parity-Shards']).toBe('2');
		expect(xhr.headers['X-CallFS-Erasure-Instances']).toBe('node-a,node-b');
	});

	it('upload maps a 412 response to a precondition failure', async () => {
		const pending = upload('/a/x.bin', new Blob(['x']), { overwrite: true, ifMatch: '"old"' });
		const xhr = FakeXhr.instances[0];
		xhr.status = 412;
		xhr.responseText = '{"code":"PRECONDITION_FAILED","message":"changed"}';
		xhr.onload?.();
		const result = await pending;
		expect(result).toMatchObject({ ok: false, error: { kind: 'http', status: 412 } });
	});

	it('upload aborts the request when the signal fires', async () => {
		const controller = new AbortController();
		const pending = upload('/a/big.bin', new Blob(['x']), { signal: controller.signal });
		controller.abort();
		const result = await pending;
		expect(FakeXhr.instances[0].aborted).toBe(true);
		expect(result).toEqual({ ok: false, error: { kind: 'aborted' } });
	});

	it('upload does not send when the signal is already aborted', async () => {
		const controller = new AbortController();
		controller.abort();
		const result = await upload('/a/big.bin', new Blob(['x']), { signal: controller.signal });
		expect(FakeXhr.instances).toHaveLength(0);
		expect(result).toEqual({ ok: false, error: { kind: 'aborted' } });
	});

	it('wsDownload parses size and progress frames and assembles binary chunks', async () => {
		const progress: [number, number | null][] = [];
		const pending = wsDownload('/a/b.bin', { onProgress: (received, total) => progress.push([received, total]) });
		const socket = FakeSocket.last as FakeSocket;
		socket.onmessage?.({ data: 'size:6' });
		socket.onmessage?.({ data: new Uint8Array([1, 2, 3]).buffer });
		socket.onmessage?.({ data: 'progress:3' });
		socket.onmessage?.({ data: new Uint8Array([4, 5, 6]).buffer });
		socket.onmessage?.({ data: 'progress:6' });
		socket.onclose?.();
		const result = await pending;
		expect(socket.url.endsWith('/v1/files/ws/a/b.bin')).toBe(true);
		expect(socket.binaryType).toBe('arraybuffer');
		expect(progress).toEqual([[3, 6], [6, 6]]);
		expect(result.ok ? result.value.size : -1).toBe(6);
	});

	it('wsDownload resolves a network failure when the socket closes before all bytes arrive', async () => {
		const pending = wsDownload('/a/b.bin');
		const socket = FakeSocket.last as FakeSocket;
		socket.onmessage?.({ data: 'size:6' });
		socket.onmessage?.({ data: new Uint8Array([1, 2, 3]).buffer });
		socket.onclose?.();
		expect(await pending).toEqual({ ok: false, error: { kind: 'network' } });
	});

	it('wsDownload resolves a network failure on socket error', async () => {
		const pending = wsDownload('/a/b.bin');
		const socket = FakeSocket.last as FakeSocket;
		socket.onerror?.();
		socket.onclose?.();
		expect(await pending).toEqual({ ok: false, error: { kind: 'network' } });
	});

	it('createFolder posts to the folder path with a trailing slash', async () => {
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 201 }));
		await createFolder('/team/new');
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/files/team/new/');
		expect(init.method).toBe('POST');
	});

	it('trash sends DELETE with trash=true and deletePermanent sends recursive as requested', async () => {
		fetchMock.mockResolvedValueOnce(jsonResponse({ trash_id: 't1', expires_at: 'x', original_path: '/a' }));
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
		await trash('/a');
		await deletePermanent('/dir', { recursive: true });
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/files/a?trash=true');
		expect(fetchMock.mock.calls[1][0]).toBe('/v1/files/dir?recursive=true');
		expect((fetchMock.mock.calls[1][1] as RequestInit).method).toBe('DELETE');
	});

	it('search sends query parameters and drops a null cursor', async () => {
		fetchMock.mockResolvedValueOnce(jsonResponse({ items: [] }));
		await search({ q: 'log', limit: 50 }, null);
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/search?q=log&limit=50');
	});

	it('countObjects requests a count-only listing for the prefix', async () => {
		fetchMock.mockResolvedValueOnce(jsonResponse({ count: 2, bytes: 10, truncated: false }));
		await countObjects('logs/');
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/objects?prefix=logs%2F&count_only=true');
	});

	it('batch posts the ops with the mode and stop_on_error false', async () => {
		fetchMock.mockResolvedValueOnce(jsonResponse({ results: [] }));
		await batch([{ op: 'trash', path: '/a' }], 'sync');
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect(fetchMock.mock.calls[0][0]).toBe('/v1/batch');
		expect(JSON.parse(String(init.body))).toEqual({
			ops: [{ op: 'trash', path: '/a' }],
			mode: 'sync',
			stop_on_error: false
		});
	});
});
