import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { invalidate } from './cache';
import { createMutation, type UndoAction } from './mutation.svelte';
import { Query } from './query.svelte';

describe('queries', () => {
	let hidden = false;

	beforeEach(() => {
		Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden });
	});

	afterEach(() => {
		vi.useRealTimers();
		hidden = false;
		Reflect.deleteProperty(document, 'hidden');
	});

	it('pauses polling while the document is hidden and resumes when it is visible', async () => {
		vi.useFakeTimers();
		const fetch = vi.fn(async () => 'rows');
		const query = new Query({ key: ['poll-test'], fetch, refetchMs: 1000, staleMs: 0 });
		const detach = query.attach();
		await vi.advanceTimersByTimeAsync(0);
		expect(fetch).toHaveBeenCalledTimes(1);

		hidden = true;
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.advanceTimersByTimeAsync(5000);
		expect(fetch).toHaveBeenCalledTimes(1);

		hidden = false;
		document.dispatchEvent(new Event('visibilitychange'));
		await vi.advanceTimersByTimeAsync(0);
		expect(fetch).toHaveBeenCalledTimes(2);
		detach();
	});

	it('shares one in-flight request between queries with the same key', async () => {
		let finish: (value: string) => void = () => undefined;
		const fetch = vi.fn(() => new Promise<string>((done) => {
			finish = done;
		}));
		const first = new Query({ key: ['dedupe-test', 1], fetch, staleMs: 0 });
		const second = new Query({ key: ['dedupe-test', 1], fetch, staleMs: 0 });
		const detachFirst = first.attach();
		const detachSecond = second.attach();
		finish('done');
		await first.refetch();
		expect(fetch).toHaveBeenCalledTimes(1);
		expect(second.data).toBe('done');
		detachFirst();
		detachSecond();
	});

	it('keeps the last good data and reports an error when a refetch fails', async () => {
		const fetch = vi
			.fn<(signal: AbortSignal) => Promise<string>>()
			.mockResolvedValueOnce('good')
			.mockRejectedValueOnce(new TypeError('offline'));
		const query = new Query<string>({ key: ['stale-test'], fetch, staleMs: 0 });
		const detach = query.attach();
		await query.refetch();
		expect(query.status).toBe('success');
		await query.refetch();
		expect(query.data).toBe('good');
		expect(query.status).toBe('error');
		expect(query.error?.retryable).toBe(true);
		detach();
	});

	it('refetches an attached query when its key is invalidated', async () => {
		const fetch = vi.fn(async () => 'listing');
		const query = new Query({ key: ['invalidate-test', 'dir'], fetch, staleMs: 60_000 });
		const detach = query.attach();
		await query.refetch();
		expect(fetch).toHaveBeenCalledTimes(1);
		invalidate(['invalidate-test']);
		expect(fetch).toHaveBeenCalledTimes(2);
		detach();
	});
});

describe('mutations', () => {
	it('rolls back the optimistic change and reports the error when the request fails', async () => {
		const rollback = vi.fn();
		const optimistic = vi.fn(() => rollback);
		const mutation = createMutation<string, string>({
			run: () => Promise.reject({ kind: 'network' }),
			optimistic,
			context: { action: 'files.rename' }
		});
		const result = await mutation.run('report-v2.pdf');
		expect(result).toBeNull();
		expect(optimistic).toHaveBeenCalledWith('report-v2.pdf');
		expect(rollback).toHaveBeenCalledTimes(1);
		expect(mutation.error?.retryable).toBe(true);
		expect(mutation.pending).toBe(false);
	});

	it('registers undo, invalidates the affected keys and announces success', async () => {
		const registerUndo = vi.fn<(undo: UndoAction) => void>();
		const announce = vi.fn<(message: string) => void>();
		const reverse = vi.fn(async () => 'reverted');
		const fetch = vi.fn(async () => 'listing');
		const listing = new Query({ key: ['undo-test', 'dir'], fetch, staleMs: 60_000 });
		const detach = listing.attach();
		await listing.refetch();
		expect(fetch).toHaveBeenCalledTimes(1);

		const mutation = createMutation<string, string>(
			{
				run: async (name) => `renamed:${name}`,
				invalidate: () => [['undo-test', 'dir']],
				undo: (vars) => ({ label: `Renamed to ${vars}. Undo`, run: reverse }),
				success: () => 'Saved.',
				context: { action: 'files.rename' }
			},
			{ registerUndo, announce }
		);
		const value = await mutation.run('report.pdf');
		expect(value).toBe('renamed:report.pdf');
		expect(fetch).toHaveBeenCalledTimes(2);
		expect(announce).toHaveBeenCalledWith('Saved.');
		expect(registerUndo).toHaveBeenCalledTimes(1);
		const [undo] = registerUndo.mock.calls[0];
		expect(undo.label).toBe('Renamed to report.pdf. Undo');
		expect(await undo.run()).toEqual({ ok: true, value: 'reverted' });
		expect(reverse).toHaveBeenCalledTimes(1);
		detach();
	});
});
