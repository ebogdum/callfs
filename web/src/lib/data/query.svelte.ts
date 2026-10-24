import { untrack } from 'svelte';
import type { QueryStatus, UiError } from '$lib/ui/types';
import {
	entryFor,
	fetchEntry,
	isLiveTopicRecent,
	keyText,
	subscribe,
	writeEntry,
	type CacheEntry,
	type CacheListener
} from './cache';

export interface QueryOptions<T> {
	key: readonly unknown[];
	fetch: (signal: AbortSignal) => Promise<T>;
	staleMs?: number;
	refetchMs?: number | ((data: T | undefined) => number | false);
	enabled?: () => boolean;
	isEmpty?: (data: T) => boolean;
	isPartial?: (data: T) => boolean;
	keepPrevious?: boolean;
	revalidateOnFocus?: boolean;
	liveTopics?: string[];
}

const DEFAULT_STALE_MS = 15_000;

export class Query<T> {
	data = $state<T | undefined>(undefined);
	error = $state<UiError | null>(null);
	updatedAt = $state<number | null>(null);
	fetching = $state(false);
	status = $derived(this.#statusOf());

	readonly #source: QueryOptions<T> | (() => QueryOptions<T>);
	#options = $state.raw<QueryOptions<T> | null>(null);
	#text: string | null = null;
	#entry: CacheEntry | null = null;
	#enabled = false;
	#unsubscribe: (() => void) | null = null;
	#unlisten: (() => void) | null = null;
	#timer: ReturnType<typeof setTimeout> | null = null;
	readonly #listener: CacheListener = {
		pull: () => this.#pull(),
		revalidate: () => this.#revalidate()
	};
	readonly #resume = (): void => this.#onResume();

	constructor(source: QueryOptions<T> | (() => QueryOptions<T>)) {
		this.#source = source;
	}

	attach(): () => void {
		const options = this.#resolve();
		const enabled = undefined === options.enabled || options.enabled();
		untrack(() => {
			this.#start(options, enabled);
		});
		return () => {
			this.#stop();
		};
	}

	async refetch(): Promise<void> {
		const entry = this.#entry;
		const options = this.#options;
		if (null === entry || null === options) {
			return;
		}
		await fetchEntry(entry, options.fetch);
	}

	setData(update: (current: T | undefined) => T | undefined): void {
		if (null === this.#entry) {
			return;
		}
		writeEntry(this.#entry, update(this.data));
	}

	#statusOf(): QueryStatus {
		if (null !== this.error) {
			return 'error';
		}
		const data = this.data;
		const options = this.#options;
		if (undefined === data) {
			return null !== options && undefined !== options.enabled && false === options.enabled() ? 'empty' : 'loading';
		}
		if (undefined !== options?.isEmpty && options.isEmpty(data)) {
			return 'empty';
		}
		if (undefined !== options?.isPartial && options.isPartial(data)) {
			return 'partial';
		}
		return 'success';
	}

	#resolve(): QueryOptions<T> {
		return 'function' === typeof this.#source ? this.#source() : this.#source;
	}

	#start(options: QueryOptions<T>, enabled: boolean): void {
		this.#options = options;
		this.#enabled = enabled;
		const entry = this.#bind(options);
		if (null === this.#unsubscribe) {
			this.#unsubscribe = subscribe(entry, this.#listener);
			document.addEventListener('visibilitychange', this.#resume);
			window.addEventListener('online', this.#resume);
			this.#unlisten = () => {
				document.removeEventListener('visibilitychange', this.#resume);
				window.removeEventListener('online', this.#resume);
			};
		}
		if (enabled) {
			if (this.#isStale(entry, options)) {
				void this.refetch();
			}
			this.#schedule();
		}
	}

	#stop(): void {
		this.#clearTimer();
		this.#unlisten?.();
		this.#unlisten = null;
		this.#unsubscribe?.();
		this.#unsubscribe = null;
		this.#enabled = false;
	}

	#bind(options: QueryOptions<T>): CacheEntry {
		const text = keyText(options.key);
		const entry = text === this.#text && null !== this.#entry ? this.#entry : entryFor(options.key);
		this.#text = text;
		this.#entry = entry;
		this.#pull();
		return entry;
	}

	#pull(): void {
		const entry = this.#entry;
		if (null === entry) {
			return;
		}
		const keepPrevious = true === this.#options?.keepPrevious;
		this.data = undefined === entry.data && keepPrevious ? this.data : (entry.data as T | undefined);
		this.error = entry.error;
		this.updatedAt = entry.updatedAt;
		this.fetching = entry.fetching;
	}

	#isStale(entry: CacheEntry, options: QueryOptions<T>): boolean {
		const staleMs = undefined === options.staleMs ? DEFAULT_STALE_MS : options.staleMs;
		return null === entry.updatedAt || staleMs <= Date.now() - entry.updatedAt;
	}

	#revalidate(): void {
		if (this.#enabled) {
			void this.refetch();
		}
	}

	#paused(): boolean {
		return true === document.hidden || false === navigator.onLine;
	}

	#interval(options: QueryOptions<T>): number | false {
		const { refetchMs } = options;
		if (undefined === refetchMs) {
			return false;
		}
		return 'function' === typeof refetchMs ? refetchMs(this.data) : refetchMs;
	}

	#schedule(): void {
		this.#clearTimer();
		const options = this.#options;
		if (null === options) {
			return;
		}
		const interval = this.#interval(options);
		if (false === interval) {
			return;
		}
		this.#timer = setTimeout(() => {
			this.#tick();
		}, interval);
	}

	#tick(): void {
		this.#timer = null;
		const options = this.#options;
		const interval = null === options ? false : this.#interval(options);
		if (null === options || false === interval || this.#paused()) {
			return;
		}
		const liveTopics = options.liveTopics ?? [];
		if (liveTopics.some((topic) => isLiveTopicRecent(topic, interval))) {
			this.#schedule();
			return;
		}
		void this.refetch().then(() => {
			if (null !== this.#unsubscribe) {
				this.#schedule();
			}
		});
	}

	#onResume(): void {
		const options = this.#options;
		const entry = this.#entry;
		if (null === options || null === entry || false === this.#enabled || this.#paused()) {
			return;
		}
		if (true === options.revalidateOnFocus || this.#isStale(entry, options)) {
			void this.refetch();
		}
		this.#schedule();
	}

	#clearTimer(): void {
		if (null !== this.#timer) {
			clearTimeout(this.#timer);
		}
		this.#timer = null;
	}
}

export function createQuery<T>(opts: QueryOptions<T> | (() => QueryOptions<T>)): Query<T> {
	const query = new Query(opts);
	$effect(() => query.attach());
	return query;
}
