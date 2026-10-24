import { httpFailure, type ApiFailure } from './errors';
import { filePath, fileUrl, nodePath, request } from './http';
import type { DeletePlan, DirPage, Result, Stat } from './types';

export type FileSort = 'name' | 'size' | 'mtime' | 'type';
export type SortOrder = 'asc' | 'desc';

export interface ListDirOptions {
	limit?: number;
	cursor?: string | null;
	sort?: FileSort;
	order?: SortOrder;
	type?: 'file' | 'directory';
	q?: string;
	includeHidden?: boolean;
}

export interface ErasureOptions {
	dataShards: number;
	parityShards: number;
	instances?: string[];
}

export interface UploadOptions {
	overwrite?: boolean;
	ifMatch?: string | null;
	erasure?: ErasureOptions | null;
	onProgress?: (loaded: number, total: number) => void;
	signal?: AbortSignal;
}

export interface UploadResult {
	etag: string | null;
	created: boolean;
	requestId: string | null;
}

export interface TrashReceipt {
	trash_id: string;
	expires_at: string;
	original_path: string;
}

export interface DeleteResult {
	deleted: number;
	deleted_files?: number;
	deleted_directories?: number;
}

export interface DownloadOptions {
	onProgress?: (received: number, total: number | null) => void;
	signal?: AbortSignal;
}

const CSRF_HEADER = 'X-CallFS-CSRF';
const CSRF_STORAGE_KEY = 'callfs.csrf';
const REQUEST_ID_HEADER = 'X-Request-ID';
const SIZE_FRAME_PREFIX = 'size:';
const PROGRESS_FRAME_PREFIX = 'progress:';

function withTrailingSlash(path: string): string {
	return path.endsWith('/') ? path : `${path}/`;
}

function csrfToken(): string | null {
	try {
		return globalThis.sessionStorage.getItem(CSRF_STORAGE_KEY);
	} catch {
		return null;
	}
}

function websocketUrl(pathWithQuery: string): string {
	const scheme = 'https:' === globalThis.location.protocol ? 'wss' : 'ws';
	return `${scheme}://${globalThis.location.host}${pathWithQuery}`;
}

export function stat(path: string) {
	return request<Stat>(`/v1/stat/${filePath(path)}`);
}

export function listDir(path: string, opts: ListDirOptions = {}) {
	return request<DirPage>(`/v1/directories/${filePath(withTrailingSlash(path))}`, {
		query: {
			limit: opts.limit,
			cursor: opts.cursor,
			sort: opts.sort,
			order: opts.order,
			type: opts.type,
			q: opts.q,
			include_hidden: opts.includeHidden
		}
	});
}

async function readRangeBody<T>(
	path: string,
	from: number,
	to: number,
	read: (response: Response) => Promise<T>
): Promise<Result<T, ApiFailure>> {
	const response = await fetch(fileUrl(path), {
		headers: { Range: `bytes=${from}-${to}` },
		credentials: 'same-origin'
	});
	if (response.ok) {
		return { ok: true, value: await read(response) };
	}
	const text = await response.text();
	return {
		ok: false,
		error: httpFailure(response.status, text, response.headers.get('Retry-After'), response.headers.get(REQUEST_ID_HEADER))
	};
}

export async function readRange(path: string, from: number, to: number): Promise<Result<ArrayBuffer, ApiFailure>> {
	try {
		return await readRangeBody(path, from, to, (response) => response.arrayBuffer());
	} catch {
		return { ok: false, error: { kind: 'network' } };
	}
}

export async function readRangeBlob(path: string, from: number, to: number): Promise<Result<Blob, ApiFailure>> {
	try {
		return await readRangeBody(path, from, to, (response) => response.blob());
	} catch {
		return { ok: false, error: { kind: 'network' } };
	}
}

function applyErasureHeaders(xhr: XMLHttpRequest, erasure: ErasureOptions): void {
	xhr.setRequestHeader('X-CallFS-Erasure', 'true');
	xhr.setRequestHeader('X-CallFS-Erasure-Data-Shards', String(erasure.dataShards));
	xhr.setRequestHeader('X-CallFS-Erasure-Parity-Shards', String(erasure.parityShards));
	if (undefined !== erasure.instances && 0 < erasure.instances.length) {
		xhr.setRequestHeader('X-CallFS-Erasure-Instances', erasure.instances.join(','));
	}
}

export function upload(path: string, file: Blob, opts: UploadOptions = {}): Promise<Result<UploadResult, ApiFailure>> {
	return new Promise((resolve) => {
		if (true === opts.signal?.aborted) {
			resolve({ ok: false, error: { kind: 'aborted' } });
			return;
		}
		const xhr = new XMLHttpRequest();
		const abort = (): void => {
			xhr.abort();
		};
		const settle = (result: Result<UploadResult, ApiFailure>): void => {
			opts.signal?.removeEventListener('abort', abort);
			resolve(result);
		};
		xhr.open(true === opts.overwrite ? 'PUT' : 'POST', fileUrl(path));
		xhr.setRequestHeader('Content-Type', 'application/octet-stream');
		const token = csrfToken();
		if (null !== token) {
			xhr.setRequestHeader(CSRF_HEADER, token);
		}
		if ('string' === typeof opts.ifMatch) {
			xhr.setRequestHeader('If-Match', opts.ifMatch);
		}
		const erasure = opts.erasure ?? null;
		if (null !== erasure) {
			applyErasureHeaders(xhr, erasure);
		}
		xhr.upload.onprogress = (event: ProgressEvent) => {
			opts.onProgress?.(event.loaded, event.total);
		};
		xhr.onload = () => {
			if (200 <= xhr.status && 300 > xhr.status) {
				settle({
					ok: true,
					value: {
						etag: xhr.getResponseHeader('ETag'),
						created: 201 === xhr.status,
						requestId: xhr.getResponseHeader(REQUEST_ID_HEADER)
					}
				});
				return;
			}
			settle({
				ok: false,
				error: httpFailure(
					xhr.status,
					xhr.responseText,
					xhr.getResponseHeader('Retry-After'),
					xhr.getResponseHeader(REQUEST_ID_HEADER)
				)
			});
		};
		xhr.onerror = () => {
			settle({ ok: false, error: { kind: 'network' } });
		};
		xhr.onabort = () => {
			settle({ ok: false, error: { kind: 'aborted' } });
		};
		opts.signal?.addEventListener('abort', abort);
		xhr.send(file);
	});
}

function folderUrl(path: string): string {
	return fileUrl(withTrailingSlash(path));
}

export function createFolder(path: string) {
	return request<void>(folderUrl(path), { method: 'POST' });
}

export function rename(path: string, name: string) {
	return request<unknown>(fileUrl(path), { method: 'PATCH', body: { name } });
}

export function move(path: string, destination: string, opts: { overwrite?: boolean } = {}) {
	return request<unknown>(fileUrl(path), { method: 'PATCH', body: { destination, overwrite: opts.overwrite } });
}

export function setMode(path: string, mode: string) {
	return request<unknown>(fileUrl(path), { method: 'PATCH', body: { mode } });
}

export function setOwner(path: string, owner: string) {
	return request<unknown>(fileUrl(path), { method: 'PATCH', body: { owner } });
}

export function trash(path: string) {
	return request<TrashReceipt>(fileUrl(path), { method: 'DELETE', query: { trash: true } });
}

export function deletePermanent(path: string, opts: { recursive: boolean }) {
	return request<DeleteResult>(fileUrl(path), { method: 'DELETE', query: { recursive: opts.recursive } });
}

export function planDelete(path: string) {
	return request<DeletePlan>(fileUrl(path), { method: 'DELETE', query: { recursive: true, dry_run: true } });
}

export function wsDownload(path: string, opts: DownloadOptions = {}): Promise<Result<Blob, ApiFailure>> {
	return new Promise((resolve) => {
		if (true === opts.signal?.aborted) {
			resolve({ ok: false, error: { kind: 'aborted' } });
			return;
		}
		let socket: WebSocket;
		try {
			socket = new WebSocket(websocketUrl(nodePath(`/v1/files/ws/${filePath(path)}`)));
		} catch {
			resolve({ ok: false, error: { kind: 'network' } });
			return;
		}
		const chunks: ArrayBuffer[] = [];
		let total: number | null = null;
		let downloaded = 0;
		let settled = false;
		const abort = (): void => {
			settle({ ok: false, error: { kind: 'aborted' } });
			socket.close();
		};
		const settle = (result: Result<Blob, ApiFailure>): void => {
			if (settled) {
				return;
			}
			settled = true;
			opts.signal?.removeEventListener('abort', abort);
			resolve(result);
		};
		opts.signal?.addEventListener('abort', abort);
		socket.binaryType = 'arraybuffer';
		socket.onmessage = (event: MessageEvent) => {
			if ('string' === typeof event.data) {
				if (event.data.startsWith(SIZE_FRAME_PREFIX)) {
					total = Number(event.data.slice(SIZE_FRAME_PREFIX.length));
					return;
				}
				if (event.data.startsWith(PROGRESS_FRAME_PREFIX)) {
					const received = Number(event.data.slice(PROGRESS_FRAME_PREFIX.length));
					opts.onProgress?.(received, total);
				}
				return;
			}
			if (event.data instanceof ArrayBuffer) {
				chunks.push(event.data);
				downloaded += event.data.byteLength;
			}
		};
		socket.onerror = () => {
			settle({ ok: false, error: { kind: 'network' } });
		};
		socket.onclose = () => {
			if (null !== total && total === downloaded) {
				settle({ ok: true, value: new Blob(chunks) });
				return;
			}
			settle({ ok: false, error: { kind: 'network' } });
		};
	});
}
