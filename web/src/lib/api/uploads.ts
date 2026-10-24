import { httpFailure, type ApiFailure } from './errors';
import { nodePath, request, type ApiResponse } from './http';
import type { Result, UploadSession } from './types';

export interface UploadCreated {
	upload_id: string;
	offset: number;
	part_bytes: number;
	expires_at: string;
}

export interface UploadPartAck {
	offset: number;
}

export interface UploadCompleted {
	path: string;
	etag: string;
	created: boolean;
}

export interface ResumableUploadOptions {
	partBytes?: number;
	parallel?: number;
	overwrite?: boolean;
	resumeId?: string;
	signal?: AbortSignal;
	onProgress?: (sentBytes: number, totalBytes: number) => void;
}

interface OpenedUpload {
	id: string;
	offset: number;
}

const UPLOADS_PATH = '/v1/uploads';
const DEFAULT_PART_BYTES = 8 * 1024 * 1024;
const DEFAULT_PARALLEL = 3;
const MAX_PART_RETRIES = 3;
const BACKOFF_BASE_MS = 500;
const RETRYABLE_STATUSES = [409, 429, 500, 502, 503, 504];
const CSRF_HEADER = 'X-CallFS-CSRF';
const CSRF_STORAGE_KEY = 'callfs.csrf';
const REQUEST_ID_HEADER = 'X-Request-ID';

function uploadPath(uploadId: string): string {
	return `${UPLOADS_PATH}/${encodeURIComponent(uploadId)}`;
}

async function unwrap<T>(pending: Promise<Result<ApiResponse<T>, ApiFailure>>): Promise<Result<T, ApiFailure>> {
	const result = await pending;
	return result.ok ? { ok: true, value: result.value.data } : result;
}

export function createUpload(path: string, size: number, overwrite: boolean): Promise<Result<UploadCreated, ApiFailure>> {
	return unwrap(
		request<UploadCreated>(UPLOADS_PATH, {
			method: 'POST',
			body: { path, size, overwrite, if_match: null, mode: '0644', storage: null, erasure: null }
		})
	);
}

export function getUpload(uploadId: string): Promise<Result<UploadSession, ApiFailure>> {
	return unwrap(request<UploadSession>(uploadPath(uploadId)));
}

export function completeUpload(uploadId: string): Promise<Result<UploadCompleted, ApiFailure>> {
	return unwrap(request<UploadCompleted>(`${uploadPath(uploadId)}/complete`, { method: 'POST' }));
}

export function abortUpload(uploadId: string): Promise<Result<void, ApiFailure>> {
	return unwrap(request<void>(uploadPath(uploadId), { method: 'DELETE' }));
}

function storedCsrfToken(): string | null {
	try {
		return sessionStorage.getItem(CSRF_STORAGE_KEY);
	} catch {
		return null;
	}
}

function decodeAck(text: string): Result<UploadPartAck, ApiFailure> {
	try {
		const parsed: unknown = JSON.parse(text);
		if (null !== parsed && 'object' === typeof parsed && 'offset' in parsed && 'number' === typeof parsed.offset) {
			return { ok: true, value: { offset: parsed.offset } };
		}
	} catch {
		return { ok: false, error: { kind: 'malformed' } };
	}
	return { ok: false, error: { kind: 'malformed' } };
}

async function sendPart(
	uploadId: string,
	start: number,
	total: number,
	bytes: ArrayBuffer,
	signal: AbortSignal | undefined
): Promise<Result<UploadPartAck, ApiFailure>> {
	const headers: Record<string, string> = {
		Accept: 'application/json',
		'Content-Type': 'application/octet-stream',
		'Content-Range': `bytes ${start}-${start + bytes.byteLength - 1}/${total}`
	};
	const csrfToken = storedCsrfToken();
	if (null !== csrfToken) {
		headers[CSRF_HEADER] = csrfToken;
	}
	try {
		const response = await fetch(nodePath(uploadPath(uploadId)), {
			method: 'PUT',
			headers,
			body: bytes,
			credentials: 'same-origin',
			signal
		});
		const text = await response.text();
		if (!response.ok) {
			return {
				ok: false,
				error: httpFailure(response.status, text, response.headers.get('Retry-After'), response.headers.get(REQUEST_ID_HEADER))
			};
		}
		return decodeAck(text);
	} catch {
		const aborted = undefined !== signal && true === signal.aborted;
		return { ok: false, error: { kind: aborted ? 'aborted' : 'network' } };
	}
}

function isResumable(session: UploadSession, path: string, size: number): boolean {
	return 'open' === session.state && path === session.path && size === session.size;
}

function isRetryable(failure: ApiFailure): boolean {
	if ('http' !== failure.kind) {
		return 'network' === failure.kind || 'timeout' === failure.kind;
	}
	return RETRYABLE_STATUSES.includes(failure.status);
}

function pause(ms: number, signal: AbortSignal | undefined): Promise<boolean> {
	return new Promise((resolve) => {
		if (undefined !== signal && true === signal.aborted) {
			resolve(false);
			return;
		}
		const onAbort = (): void => {
			clearTimeout(timer);
			resolve(false);
		};
		const timer = setTimeout(() => {
			signal?.removeEventListener('abort', onAbort);
			resolve(true);
		}, ms);
		signal?.addEventListener('abort', onAbort, { once: true });
	});
}

async function readPart(file: File, start: number, partBytes: number): Promise<Result<ArrayBuffer, ApiFailure>> {
	try {
		return { ok: true, value: await file.slice(start, start + partBytes).arrayBuffer() };
	} catch {
		return { ok: false, error: { kind: 'network' } };
	}
}

function prefetchParts(
	file: File,
	reads: Map<number, Promise<Result<ArrayBuffer, ApiFailure>>>,
	from: number,
	partBytes: number,
	parallel: number
): void {
	for (const key of reads.keys()) {
		if (from > key) {
			reads.delete(key);
		}
	}
	for (let index = 0; index < parallel; index += 1) {
		const start = from + index * partBytes;
		if (file.size <= start) {
			break;
		}
		if (!reads.has(start)) {
			reads.set(start, readPart(file, start, partBytes));
		}
	}
}

async function openUpload(path: string, file: File, overwrite: boolean, resumeId: string | undefined): Promise<Result<OpenedUpload, ApiFailure>> {
	if (undefined !== resumeId) {
		const resumed = await getUpload(resumeId);
		if (resumed.ok && isResumable(resumed.value, path, file.size)) {
			return { ok: true, value: { id: resumeId, offset: resumed.value.offset } };
		}
		if (!resumed.ok && 'http' !== resumed.error.kind) {
			return { ok: false, error: resumed.error };
		}
	}
	const created = await createUpload(path, file.size, overwrite);
	if (!created.ok) {
		return created;
	}
	return { ok: true, value: { id: created.value.upload_id, offset: created.value.offset } };
}

async function sendRemaining(
	uploadId: string,
	file: File,
	startOffset: number,
	partBytes: number,
	parallel: number,
	options: ResumableUploadOptions
): Promise<Result<void, ApiFailure>> {
	const reads = new Map<number, Promise<Result<ArrayBuffer, ApiFailure>>>();
	let offset = startOffset;
	let retries = 0;
	while (offset < file.size) {
		const current = reads.get(offset) ?? readPart(file, offset, partBytes);
		reads.delete(offset);
		prefetchParts(file, reads, offset + partBytes, partBytes, parallel);
		const chunk = await current;
		if (!chunk.ok) {
			return chunk;
		}
		const sent = await sendPart(uploadId, offset, file.size, chunk.value, options.signal);
		if (sent.ok) {
			offset = sent.value.offset;
			retries = 0;
			options.onProgress?.(offset, file.size);
			continue;
		}
		if (!isRetryable(sent.error) || MAX_PART_RETRIES <= retries) {
			return sent;
		}
		retries += 1;
		if (!(await pause(BACKOFF_BASE_MS * 2 ** (retries - 1), options.signal))) {
			return { ok: false, error: { kind: 'aborted' } };
		}
		const resumed = await getUpload(uploadId);
		if (!resumed.ok) {
			return resumed;
		}
		offset = resumed.value.offset;
	}
	return { ok: true, value: undefined };
}

function positiveOr(value: number | undefined, fallback: number): number {
	return undefined !== value && 0 < value ? value : fallback;
}

export async function uploadResumable(
	path: string,
	file: File,
	options: ResumableUploadOptions = {}
): Promise<Result<UploadCompleted, ApiFailure>> {
	const partBytes = positiveOr(options.partBytes, DEFAULT_PART_BYTES);
	const parallel = positiveOr(options.parallel, DEFAULT_PARALLEL);
	const opened = await openUpload(path, file, true === options.overwrite, options.resumeId);
	if (!opened.ok) {
		return opened;
	}
	const sent = await sendRemaining(opened.value.id, file, opened.value.offset, partBytes, parallel, options);
	if (!sent.ok) {
		return sent;
	}
	return completeUpload(opened.value.id);
}
