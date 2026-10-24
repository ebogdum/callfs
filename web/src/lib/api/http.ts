import { nodeScope } from '$lib/stores/node-scope.svelte';
import { session } from '$lib/stores/session.svelte';
import { httpFailure, type ApiFailure, type HttpFailure } from './errors';
import { scopedPath } from './scope';
import type { Me, Result } from './types';

export interface RequestOptions {
	method?: 'GET' | 'HEAD' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
	query?: Record<string, string | number | boolean | undefined | null>;
	body?: unknown;
	headers?: Record<string, string>;
	signal?: AbortSignal;
	timeoutMs?: number;
	raw?: boolean;
	idempotent?: boolean;
	node?: string | null;
}

export interface ApiResponse<T> {
	data: T;
	status: number;
	headers: Headers;
	requestId: string | null;
	etag: string | null;
}

export interface RequestRecord {
	url: string;
	status: number;
	requestId: string | null;
	at: number;
}

interface Exchange {
	status: number;
	headers: Headers;
	text: string;
}

const DEFAULT_TIMEOUT_MS = 30_000;
const MAX_RETRIES = 3;
const RETRYABLE_STATUSES = [429, 502, 503];
const CSRF_HEADER = 'X-CallFS-CSRF';
const CSRF_STORAGE_KEY = 'callfs.csrf';
const REQUEST_ID_HEADER = 'X-Request-ID';
const LOGIN_PATH = '/v1/session';
const ME_PATH = '/v1/me';
const RECENT_LIMIT = 20;

const recentRequests: RequestRecord[] = [];
let csrfToken: string | null = null;

function sessionStorageOrNull(): Storage | null {
	try {
		return globalThis.sessionStorage;
	} catch {
		return null;
	}
}

function getCsrfToken(): string | null {
	if (null === csrfToken) {
		const store = sessionStorageOrNull();
		csrfToken = null === store ? null : store.getItem(CSRF_STORAGE_KEY);
	}
	return csrfToken;
}

export function setCsrfToken(token: string | null): void {
	csrfToken = token;
	const store = sessionStorageOrNull();
	if (null === store) {
		return;
	}
	if (null === token) {
		store.removeItem(CSRF_STORAGE_KEY);
		return;
	}
	store.setItem(CSRF_STORAGE_KEY, token);
}

export function recentRequestRecords(): RequestRecord[] {
	return recentRequests.slice();
}

function recordRequest(url: string, status: number, requestId: string | null): void {
	recentRequests.push({ url, status, requestId, at: Date.now() });
	if (RECENT_LIMIT < recentRequests.length) {
		recentRequests.shift();
	}
}

function isReadMethod(method: string): boolean {
	return 'GET' === method || 'HEAD' === method;
}

function isHttp(failure: ApiFailure): failure is HttpFailure {
	return 'http' === failure.kind;
}

function isTransient(failure: ApiFailure): boolean {
	return 'network' === failure.kind || (isHttp(failure) && RETRYABLE_STATUSES.includes(failure.status));
}

function retryDelayMs(failure: ApiFailure, attempt: number): number {
	if (isHttp(failure) && null !== failure.retryAfterS) {
		return failure.retryAfterS * 1000;
	}
	return 1000 * 2 ** attempt;
}

function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

function buildUrl(path: string, query: RequestOptions['query']): string {
	if (undefined === query) {
		return path;
	}
	const params = new URLSearchParams();
	for (const [key, value] of Object.entries(query)) {
		if (undefined !== value && null !== value) {
			params.set(key, String(value));
		}
	}
	const search = params.toString();
	return '' === search ? path : `${path}?${search}`;
}

function buildInit(method: string, opts: RequestOptions, signal: AbortSignal): RequestInit {
	const headers: Record<string, string> = { Accept: 'application/json', ...opts.headers };
	let body: string | undefined;
	if (undefined !== opts.body) {
		headers['Content-Type'] = 'application/json';
		body = JSON.stringify(opts.body);
	}
	if (!isReadMethod(method)) {
		const token = getCsrfToken();
		if (null !== token) {
			headers[CSRF_HEADER] = token;
		}
	}
	return { method, headers, body, credentials: 'same-origin', signal };
}

function abortFailure(timedOut: boolean, signal: AbortSignal | undefined): ApiFailure {
	if (timedOut) {
		return { kind: 'timeout' };
	}
	if (undefined !== signal && true === signal.aborted) {
		return { kind: 'aborted' };
	}
	return { kind: 'network' };
}

async function exchange(url: string, method: string, opts: RequestOptions): Promise<Result<Exchange, ApiFailure>> {
	const controller = new AbortController();
	if (true === opts.signal?.aborted) {
		controller.abort();
	}
	const timeoutMs = opts.timeoutMs ?? DEFAULT_TIMEOUT_MS;
	let timedOut = false;
	const timer = 0 < timeoutMs
		? setTimeout(() => {
				timedOut = true;
				controller.abort();
			}, timeoutMs)
		: null;
	const forwardAbort = () => {
		controller.abort();
	};
	opts.signal?.addEventListener('abort', forwardAbort);
	try {
		const response = await fetch(url, buildInit(method, opts, controller.signal));
		const text = await response.text();
		const requestId = response.headers.get(REQUEST_ID_HEADER);
		recordRequest(url, response.status, requestId);
		if (response.ok) {
			return { ok: true, value: { status: response.status, headers: response.headers, text } };
		}
		return {
			ok: false,
			error: httpFailure(response.status, text, response.headers.get('Retry-After'), requestId)
		};
	} catch {
		recordRequest(url, 0, null);
		return { ok: false, error: abortFailure(timedOut, opts.signal) };
	} finally {
		if (null !== timer) {
			clearTimeout(timer);
		}
		opts.signal?.removeEventListener('abort', forwardAbort);
	}
}

async function exchangeWithRetries(url: string, method: string, opts: RequestOptions): Promise<Result<Exchange, ApiFailure>> {
	const mayRetry = isReadMethod(method) || true === opts.idempotent;
	for (let attempt = 0; ; attempt += 1) {
		const result = await exchange(url, method, opts);
		if (result.ok || !mayRetry || MAX_RETRIES <= attempt || !isTransient(result.error)) {
			return result;
		}
		await delay(retryDelayMs(result.error, attempt));
	}
}

async function recover(
	failure: ApiFailure,
	path: string,
	url: string,
	method: string,
	opts: RequestOptions
): Promise<Result<Exchange, ApiFailure>> {
	if (!isHttp(failure)) {
		return { ok: false, error: failure };
	}
	if (401 === failure.status && LOGIN_PATH !== path && (await session.onUnauthorized())) {
		return exchangeWithRetries(url, method, opts);
	}
	if (403 === failure.status && 'CSRF_FAILED' === failure.code) {
		const me = await request<Me>(ME_PATH);
		if (me.ok && null !== me.value.data.csrf_token) {
			setCsrfToken(me.value.data.csrf_token);
			return exchangeWithRetries(url, method, opts);
		}
	}
	return { ok: false, error: failure };
}

function decodeJson(text: string): Result<unknown, ApiFailure> {
	try {
		return { ok: true, value: JSON.parse(text) };
	} catch {
		return { ok: false, error: { kind: 'malformed' } };
	}
}

function toResponse<T>(exchanged: Exchange, raw: boolean): Result<ApiResponse<T>, ApiFailure> {
	const { status, headers, text } = exchanged;
	let decoded: Result<unknown, ApiFailure>;
	if (raw) {
		decoded = { ok: true, value: text };
	} else if (204 === status || '' === text) {
		decoded = { ok: true, value: undefined };
	} else {
		decoded = decodeJson(text);
	}
	if (!decoded.ok) {
		return decoded;
	}
	return {
		ok: true,
		value: {
			data: decoded.value as T,
			status,
			headers,
			requestId: headers.get(REQUEST_ID_HEADER),
			etag: headers.get('ETag')
		}
	};
}

export function nodePath(path: string, node?: string | null): string {
	return scopedPath(path, undefined === node ? nodeScope.selected : node);
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<Result<ApiResponse<T>, ApiFailure>> {
	const url = buildUrl(nodePath(path, opts.node), opts.query);
	const method = opts.method ?? 'GET';
	const first = await exchangeWithRetries(url, method, opts);
	const exchanged = first.ok ? first : await recover(first.error, path, url, method, opts);
	if (!exchanged.ok) {
		return exchanged;
	}
	return toResponse<T>(exchanged.value, true === opts.raw);
}

export function filePath(path: string): string {
	const segments = path
		.split('/')
		.filter((segment) => '' !== segment)
		.map((segment) => encodeURIComponent(segment));
	const trailing = path.endsWith('/') && 0 < segments.length ? '/' : '';
	return segments.join('/') + trailing;
}

export function fileUrl(path: string, opts: { download?: boolean } = {}): string {
	const query = true === opts.download ? '?download=true' : '';
	return nodePath(`/v1/files/${filePath(path)}${query}`);
}
