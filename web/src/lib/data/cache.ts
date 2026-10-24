import { toUiError, type ApiFailure } from '$lib/api/errors';
import type { UiError } from '$lib/ui/types';

export interface CacheListener {
	pull: () => void;
	revalidate: () => void;
}

export interface CacheEntry {
	readonly parts: readonly unknown[];
	data: unknown;
	error: UiError | null;
	updatedAt: number | null;
	fetching: boolean;
	inFlight: Promise<void> | null;
	readonly listeners: Set<CacheListener>;
}

const LOAD_ACTION = 'data.load';
const FAILURE_KINDS: readonly unknown[] = ['network', 'aborted', 'timeout', 'malformed', 'http'];
const entries = new Map<string, CacheEntry>();
const liveTopicSeenAt = new Map<string, number>();

export function keyText(key: readonly unknown[]): string {
	return JSON.stringify(key);
}

export function entryFor(key: readonly unknown[]): CacheEntry {
	const text = keyText(key);
	const existing = entries.get(text);
	if (undefined !== existing) {
		return existing;
	}
	const entry: CacheEntry = {
		parts: [...key],
		data: undefined,
		error: null,
		updatedAt: null,
		fetching: false,
		inFlight: null,
		listeners: new Set()
	};
	entries.set(text, entry);
	return entry;
}

export function subscribe(entry: CacheEntry, listener: CacheListener): () => void {
	entry.listeners.add(listener);
	return () => {
		entry.listeners.delete(listener);
	};
}

function notify(entry: CacheEntry): void {
	for (const listener of entry.listeners) {
		listener.pull();
	}
}

export function writeEntry(entry: CacheEntry, data: unknown): void {
	entry.data = data;
	entry.error = null;
	entry.updatedAt = Date.now();
	notify(entry);
}

function startsWith(parts: readonly unknown[], prefix: readonly unknown[]): boolean {
	return (
		prefix.length <= parts.length &&
		prefix.every((part, index) => JSON.stringify(part) === JSON.stringify(parts[index]))
	);
}

export function invalidate(prefix: readonly unknown[]): void {
	for (const entry of entries.values()) {
		if (startsWith(entry.parts, prefix)) {
			entry.updatedAt = null;
			for (const listener of entry.listeners) {
				listener.revalidate();
			}
		}
	}
}

export function invalidateAll(): void {
	invalidate([]);
}

export function clearCache(): void {
	entries.clear();
}

export function failureOf(error: unknown): ApiFailure {
	if ('object' === typeof error && null !== error && 'kind' in error && FAILURE_KINDS.includes(error.kind)) {
		return error as ApiFailure;
	}
	return { kind: 'network' };
}

export function fetchEntry(entry: CacheEntry, fetcher: (signal: AbortSignal) => Promise<unknown>): Promise<void> {
	if (null !== entry.inFlight) {
		return entry.inFlight;
	}
	const load = async (): Promise<void> => {
		try {
			entry.data = await fetcher(new AbortController().signal);
			entry.error = null;
			entry.updatedAt = Date.now();
		} catch (error) {
			entry.error = toUiError(failureOf(error), { action: LOAD_ACTION });
		}
	};
	entry.fetching = true;
	notify(entry);
	entry.inFlight = load().finally(() => {
		entry.inFlight = null;
		entry.fetching = false;
		notify(entry);
	});
	return entry.inFlight;
}

export function noteLiveTopic(topic: string): void {
	liveTopicSeenAt.set(topic, Date.now());
}

export function isLiveTopicRecent(topic: string, withinMs: number): boolean {
	const seenAt = liveTopicSeenAt.get(topic);
	return undefined !== seenAt && withinMs > Date.now() - seenAt;
}
