import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { request, setCsrfToken } from '$lib/api/http';
import * as sessionApi from '$lib/api/session';
import type { Capabilities, Me } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';

vi.mock('$lib/api/session', () => ({
	signIn: vi.fn(),
	signOut: vi.fn(),
	me: vi.fn(),
	capabilities: vi.fn(),
}));

vi.mock('$lib/api/http', () => ({
	request: vi.fn(),
	filePath: (path: string) => path.replace(/^\//, ''),
	setCsrfToken: vi.fn(),
}));

vi.mock('$lib/api/errors', () => ({
	toUiError: vi.fn((_failure: unknown, context: { action: string }) => ({
		title: context.action,
		body: 'The request failed.',
		retryable: false,
	})),
}));

import { CapabilitiesStore } from './capabilities.svelte';
import { NOTIFICATION_LIMIT, NotificationStore } from './notifications.svelte';
import {
	PREFERENCES_MIRROR_KEY,
	PREFERENCES_SAVE_DELAY_MS,
	PreferencesStore,
	diffValues,
	migrate,
	mergeValues,
} from './preferences.svelte';
import { EXPIRY_WARNING_MS, IDLE_TIMEOUT_MS, SessionStore, warningFor } from './session.svelte';
import { TOAST_VISIBLE_LIMIT, ToastStore } from './toasts.svelte';

const UI_ERROR: UiError = { title: 'Something failed.', body: 'Try again.', retryable: false };

const ME: Me = {
	user_id: 'alice',
	is_admin: true,
	auth_method: 'session',
	session_expires_at: null,
	csrf_token: 'csrf-1',
	permissions_model: 'owner',
};

const CAPABILITIES: Capabilities = {
	version: '1.0.0',
	commit: 'abc123',
	build_date: '2026-01-01',
	go_version: 'go1.24',
	instance_id: 'node-a',
	node_id: null,
	features: {
		erasure: false,
		raft: false,
		cross_server: true,
		websocket: true,
		replication: false,
		quic: false,
		metrics_listener: false,
		trash: true,
		resumable_uploads: true,
		search: true,
		ui: true,
	},
	storages: ['localfs'],
	default_storage: 'localfs',
	file_index: 'sqlite',
	lock_type: 'local',
	external_url: '',
	limits: {
		max_upload_bytes: 10737418240,
		max_split_upload_bytes: 1073741824,
		max_ws_upload_bytes: 100000000,
		ws_frame_bytes: 1048576,
		max_patch_body_bytes: 1048576,
		link_expiry_min_s: 1,
		link_expiry_max_s: 86400,
		list_page_max: 1000,
		upload_part_bytes: 8388608,
	},
	erasure_defaults: { data_shards: 4, parity_shards: 2, max_total_shards: 64, min_file_size: 0 },
	trash_retention_s: 2592000,
};

const PREFS_PATH = '/v1/files/.callfs/ui/users/alice/preferences.json';

function httpFailure(status: number) {
	return { kind: 'http' as const, status, code: null, message: null, details: null, requestId: null, retryAfterS: null };
}

function okResponse(data: unknown, etag: string | null) {
	return { ok: true as const, value: { data, status: 200, headers: new Headers(), requestId: null, etag } };
}

async function flushPromises(): Promise<void> {
	for (let turn = 0; 20 > turn; turn += 1) {
		await Promise.resolve();
	}
}

describe('warningFor', () => {
	it('returns no warning while both clocks have time left', () => {
		expect(warningFor(Number.POSITIVE_INFINITY, IDLE_TIMEOUT_MS, false)).toBeNull();
	});

	it('returns expired once any clock is out or reauth is required', () => {
		expect(warningFor(-1, IDLE_TIMEOUT_MS, false)).toEqual({ reason: 'expired', secondsLeft: 0 });
		expect(warningFor(Number.POSITIVE_INFINITY, 0, false)).toEqual({ reason: 'expired', secondsLeft: 0 });
		expect(warningFor(Number.POSITIVE_INFINITY, IDLE_TIMEOUT_MS, true)).toEqual({ reason: 'expired', secondsLeft: 0 });
	});
});

describe('SessionStore', () => {
	const BASE = 1_000_000;
	let now = BASE;
	const clock = (): number => now;

	beforeEach(() => {
		vi.useFakeTimers();
		now = BASE;
		vi.clearAllMocks();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('refresh adopts the current user and csrf token', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: true, value: ME });
		const result = await store.refresh();
		expect(result).toEqual({ ok: true, value: ME });
		expect(store.me).toEqual(ME);
		expect(store.isAdmin).toBe(true);
		expect(store.csrf).toBe('csrf-1');
		expect(setCsrfToken).toHaveBeenCalledWith('csrf-1');
	});

	it('refresh returns the error and leaves the user empty', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: false, error: UI_ERROR });
		const result = await store.refresh();
		expect(result).toEqual({ ok: false, error: UI_ERROR });
		expect(store.me).toBeNull();
		expect(store.isAdmin).toBe(false);
	});

	it('signIn builds the session from the grant', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.signIn).mockResolvedValue({
			ok: true,
			value: { user_id: 'bob', is_admin: false, expires_at: new Date(BASE + 3_600_000).toISOString(), csrf_token: 'csrf-2' },
		});
		const result = await store.signIn('key-1');
		expect(sessionApi.signIn).toHaveBeenCalledWith('key-1');
		expect(result.ok).toBe(true);
		expect(store.me?.user_id).toBe('bob');
		expect(store.me?.auth_method).toBe('session');
		expect(store.csrf).toBe('csrf-2');
	});

	it('signIn failure leaves the store untouched', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.signIn).mockResolvedValue({ ok: false, error: UI_ERROR });
		const result = await store.signIn('wrong');
		expect(result).toEqual({ ok: false, error: UI_ERROR });
		expect(store.me).toBeNull();
		expect(setCsrfToken).not.toHaveBeenCalled();
	});

	it('signOut clears the user, csrf token and warning', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: true, value: ME });
		vi.mocked(sessionApi.signOut).mockResolvedValue({ ok: true, value: undefined });
		await store.refresh();
		const result = await store.signOut();
		expect(result).toEqual({ ok: true, value: undefined });
		expect(store.me).toBeNull();
		expect(store.csrf).toBeNull();
		expect(setCsrfToken).toHaveBeenLastCalledWith(null);
	});

	it('tick raises an idle warning two minutes before the idle timeout', async () => {
		const store = new SessionStore(clock, IDLE_TIMEOUT_MS);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: true, value: ME });
		await store.refresh();
		store.tick(BASE + IDLE_TIMEOUT_MS - EXPIRY_WARNING_MS);
		expect(store.warning).toEqual({ reason: 'idle', secondsLeft: 120 });
	});

	it('tick raises an expiry warning from the absolute session expiry', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({
			ok: true,
			value: { ...ME, session_expires_at: new Date(BASE + 600_000).toISOString() },
		});
		await store.refresh();
		store.tick(BASE + 500_000);
		expect(store.warning).toEqual({ reason: 'expiry', secondsLeft: 100 });
	});

	it('tick reports expired once the absolute expiry passes', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({
			ok: true,
			value: { ...ME, session_expires_at: new Date(BASE + 600_000).toISOString() },
		});
		await store.refresh();
		store.tick(BASE + 600_000);
		expect(store.warning).toEqual({ reason: 'expired', secondsLeft: 0 });
	});

	it('onUnauthorized opens the expired state', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: true, value: ME });
		await store.refresh();
		store.onUnauthorized();
		expect(store.warning).toEqual({ reason: 'expired', secondsLeft: 0 });
	});

	it('touch moves the idle clock so the idle warning does not appear', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: true, value: ME });
		await store.refresh();
		store.touch(BASE + 3_500_000);
		store.tick(BASE + IDLE_TIMEOUT_MS - 60_000);
		expect(store.warning).toBeNull();
	});

	it('touch inside the throttle window is ignored', async () => {
		const store = new SessionStore(clock);
		vi.mocked(sessionApi.me).mockResolvedValue({ ok: true, value: ME });
		await store.refresh();
		store.touch(BASE + 500);
		store.tick(BASE + IDLE_TIMEOUT_MS - 60_000);
		expect(store.warning).toEqual({ reason: 'idle', secondsLeft: 60 });
	});

	it('watchActivity forwards input events to touch until torn down', () => {
		const store = new SessionStore(clock);
		const touch = vi.spyOn(store, 'touch');
		const target = new EventTarget();
		const stop = store.watchActivity(target);
		target.dispatchEvent(new Event('keydown'));
		expect(touch).toHaveBeenCalledTimes(1);
		stop();
		target.dispatchEvent(new Event('keydown'));
		expect(touch).toHaveBeenCalledTimes(1);
	});
});

describe('CapabilitiesStore', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('loads once per session and answers has() from the feature map', async () => {
		const store = new CapabilitiesStore();
		vi.mocked(sessionApi.capabilities).mockResolvedValue({ ok: true, value: CAPABILITIES });
		expect(store.has('trash')).toBe(false);
		await store.load();
		await store.load();
		expect(sessionApi.capabilities).toHaveBeenCalledTimes(1);
		expect(store.status).toBe('success');
		expect(store.has('trash')).toBe(true);
		expect(store.has('raft')).toBe(false);
	});

	it('records the error when loading fails', async () => {
		const store = new CapabilitiesStore();
		vi.mocked(sessionApi.capabilities).mockResolvedValue({ ok: false, error: UI_ERROR });
		const result = await store.load();
		expect(result).toEqual({ ok: false, error: UI_ERROR });
		expect(store.status).toBe('error');
		expect(store.error).toEqual(UI_ERROR);
	});

	it('reset clears the cached capabilities', async () => {
		const store = new CapabilitiesStore();
		vi.mocked(sessionApi.capabilities).mockResolvedValue({ ok: true, value: CAPABILITIES });
		await store.load();
		store.reset();
		expect(store.status).toBe('loading');
		expect(store.has('search')).toBe(false);
	});
});

describe('preferences migrate and merge', () => {
	it('returns defaults for missing input', () => {
		const prefs = migrate(undefined);
		expect(prefs.theme).toBe('system');
		expect(prefs.fontScale).toBe(1);
		expect(prefs.files.sort).toEqual({ key: 'name', dir: 'asc' });
		expect(prefs.tables).toEqual({});
	});

	it('keeps valid fields and replaces mistyped ones with defaults', () => {
		const prefs = migrate({
			theme: 'dark',
			fontScale: 'big',
			files: { view: 'grid', columns: 'name' },
			tables: { files: { visible: ['name'], order: [], widths: {}, density: 'compact', pageSize: 50 } },
		});
		expect(prefs.theme).toBe('dark');
		expect(prefs.fontScale).toBe(1);
		expect(prefs.files.view).toBe('grid');
		expect(prefs.files.columns).toEqual([]);
		expect(prefs.tables.files?.pageSize).toBe(50);
	});

	it('ignores non-object input', () => {
		expect(migrate('nope').contrast).toBe('standard');
	});

	it('diffValues keeps only changed leaves and mergeValues applies them over a base', () => {
		const diff = diffValues({ a: 1, b: { c: 2, d: 3 } }, { a: 1, b: { c: 9, d: 3 } });
		expect(diff).toEqual({ b: { c: 9 } });
		expect(mergeValues({ a: 1, b: { c: 2, d: 3 } }, diff)).toEqual({ a: 1, b: { c: 9, d: 3 } });
	});
});

describe('PreferencesStore', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		localStorage.clear();
		vi.clearAllMocks();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('falls back to defaults when the preferences file is missing', async () => {
		vi.mocked(request).mockResolvedValue({ ok: false, error: httpFailure(404) });
		const store = new PreferencesStore();
		const result = await store.load('alice');
		expect(result.ok).toBe(true);
		expect(store.status).toBe('ready');
		expect(store.value.theme).toBe('system');
		expect(request).toHaveBeenCalledWith(PREFS_PATH, { node: null });
	});

	it('applies the stored file and mirrors it to localStorage', async () => {
		vi.mocked(request).mockResolvedValue(okResponse({ theme: 'dark' }, '"e1"'));
		const store = new PreferencesStore();
		await store.load('alice');
		expect(store.value.theme).toBe('dark');
		expect(JSON.parse(localStorage.getItem(PREFERENCES_MIRROR_KEY) ?? '{}').theme).toBe('dark');
	});

	it('records a load failure with the mapped error', async () => {
		vi.mocked(request).mockResolvedValue({ ok: false, error: httpFailure(500) });
		const store = new PreferencesStore();
		const result = await store.load('alice');
		expect(result.ok).toBe(false);
		expect(store.status).toBe('error');
		expect(store.error?.title).toBe('preferences.load');
	});

	it('update saves once after the debounce with If-Match', async () => {
		vi.mocked(request).mockResolvedValueOnce(okResponse({}, '"e1"'));
		const store = new PreferencesStore();
		await store.load('alice');
		vi.mocked(request).mockResolvedValueOnce(okResponse({}, '"e2"'));
		store.update({ theme: 'light' });
		expect(request).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(PREFERENCES_SAVE_DELAY_MS);
		expect(request).toHaveBeenLastCalledWith(PREFS_PATH, {
			method: 'PUT',
			body: expect.objectContaining({ theme: 'light' }),
			headers: { 'If-Match': '"e1"' },
			node: null,
		});
	});

	it('a 412 reloads the file and keeps local changes over remote ones', async () => {
		vi.mocked(request)
			.mockResolvedValueOnce(okResponse({ accentHue: 200 }, '"e1"'))
			.mockResolvedValueOnce({ ok: false, error: httpFailure(412) })
			.mockResolvedValueOnce(okResponse({ accentHue: 250, contrast: 'more' }, '"e2"'))
			.mockResolvedValueOnce(okResponse({}, '"e3"'));
		const store = new PreferencesStore();
		await store.load('alice');
		store.update({ theme: 'dark' });
		await vi.advanceTimersByTimeAsync(PREFERENCES_SAVE_DELAY_MS);
		await flushPromises();
		expect(request).toHaveBeenCalledTimes(4);
		expect(request).toHaveBeenNthCalledWith(3, PREFS_PATH, { node: null });
		expect(request).toHaveBeenLastCalledWith(PREFS_PATH, {
			method: 'PUT',
			body: expect.objectContaining({ theme: 'dark', accentHue: 250, contrast: 'more' }),
			headers: { 'If-Match': '"e2"' },
			node: null,
		});
		expect(store.value.theme).toBe('dark');
	});

	it('save returns the mapped error when the write fails', async () => {
		vi.mocked(request).mockResolvedValueOnce(okResponse({}, '"e1"'));
		const store = new PreferencesStore();
		await store.load('alice');
		vi.mocked(request).mockResolvedValueOnce({ ok: false, error: httpFailure(500) });
		const result = await store.save();
		expect(result.ok).toBe(false);
		expect(store.error?.title).toBe('preferences.save');
		expect(localStorage.getItem(PREFERENCES_MIRROR_KEY)).not.toBeNull();
	});
});

describe('NotificationStore', () => {
	it('prepends items and counts unread ones', () => {
		const store = new NotificationStore(() => 5);
		const first = store.add({ tone: 'danger', category: 'error', title: 'Failed', body: 'Retry.', requestId: 'r-1' });
		store.add({ tone: 'ok', category: 'job', title: 'Done', body: 'Finished.' });
		expect(store.items[0]?.category).toBe('job');
		expect(store.items[1]?.requestId).toBe('r-1');
		expect(store.unread).toBe(2);
		store.markRead(first.id);
		expect(store.unread).toBe(1);
		store.markAllRead();
		expect(store.unread).toBe(0);
	});

	it('filters by category', () => {
		const store = new NotificationStore();
		store.add({ tone: 'danger', category: 'error', title: 'Failed', body: 'Retry.' });
		store.add({ tone: 'info', category: 'cluster', title: 'Member', body: 'Joined.' });
		expect(store.filtered('all')).toHaveLength(2);
		expect(store.filtered('errors')).toHaveLength(1);
		expect(store.filtered('cluster')).toHaveLength(1);
		expect(store.filtered('jobs')).toHaveLength(0);
	});

	it('keeps only the newest items up to the limit and clears by id', () => {
		const store = new NotificationStore();
		for (let index = 0; NOTIFICATION_LIMIT >= index; index += 1) {
			store.add({ tone: 'neutral', category: 'event', title: `Event ${index}`, body: '' });
		}
		expect(store.items).toHaveLength(NOTIFICATION_LIMIT);
		const newest = store.items[0];
		store.clear(newest?.id ?? 0);
		expect(store.items).toHaveLength(NOTIFICATION_LIMIT - 1);
		store.clearAll();
		expect(store.items).toHaveLength(0);
	});
});

describe('ToastStore', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('shows at most three toasts and promotes queued ones on dismiss', () => {
		const store = new ToastStore();
		const ids = [1, 2, 3, 4].map(() => store.push({ tone: 'info', title: 'Saved' }));
		expect(store.items).toHaveLength(TOAST_VISIBLE_LIMIT);
		expect(store.queue).toHaveLength(1);
		store.dismiss(ids[0] ?? 0);
		expect(store.items.map((item) => item.id)).toContain(ids[3]);
		expect(store.queue).toHaveLength(0);
	});

	it('gives action toasts ten seconds and others six', () => {
		const store = new ToastStore();
		store.push({ tone: 'info', title: 'Moved', action: { label: 'Undo', run: () => undefined } });
		store.push({ tone: 'info', title: 'Saved' });
		expect(store.items[0]?.duration).toBe(10000);
		expect(store.items[1]?.duration).toBe(6000);
	});

	it('dismisses itself after its duration', () => {
		const store = new ToastStore();
		store.push({ tone: 'ok', title: 'Saved' });
		vi.advanceTimersByTime(5999);
		expect(store.items).toHaveLength(1);
		vi.advanceTimersByTime(1);
		expect(store.items).toHaveLength(0);
	});

	it('pause keeps the remaining time and resume continues from it', () => {
		const store = new ToastStore();
		const id = store.push({ tone: 'ok', title: 'Saved' });
		vi.advanceTimersByTime(2000);
		store.pause(id);
		vi.advanceTimersByTime(10000);
		expect(store.items).toHaveLength(1);
		store.resume(id);
		vi.advanceTimersByTime(3999);
		expect(store.items).toHaveLength(1);
		vi.advanceTimersByTime(1);
		expect(store.items).toHaveLength(0);
	});
});
