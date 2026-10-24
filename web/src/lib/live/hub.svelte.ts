import { scopedPath } from '$lib/api/scope';
import type { Event as LiveEvent, Result } from '$lib/api/types';
import { invalidate } from '$lib/data/cache';
import { parentPath } from '$lib/format/path';
import { nodeScope } from '$lib/stores/node-scope.svelte';
import { LiveConnection } from './connection.svelte';
import { reconnectDelayMs } from './poller';

export type InvalidationKey = readonly unknown[];
export type LiveEventListener = (event: LiveEvent) => void;

export interface StatusCounts {
	criticalFindings: number;
	failedJobs: number;
	warningFindings: number;
}

const EVENT_TYPES: readonly string[] = [
	'file.created',
	'file.updated',
	'file.deleted',
	'file.trashed',
	'file.restored',
	'file.moved',
	'file.renamed',
	'file.mode_changed',
	'file.owner_changed',
	'link.created',
	'link.opened',
	'link.failed',
	'link.revoked',
	'link.cleanup',
	'cluster.leader_changed',
	'cluster.member_joined',
	'cluster.member_removed',
	'cluster.snapshot_taken',
	'cluster.member_unreachable',
	'server.started',
	'server.stopping',
	'server.unreachable',
	'server.reachable',
	'config.changed',
	'config.validated',
	'config.restart_required',
	'job.started',
	'job.progress',
	'job.finished',
	'job.failed',
	'health.changed',
	'finding.opened',
	'finding.resolved',
	'finding.muted',
	'security.auth_failed',
	'security.csrf_failed',
	'security.rate_limited',
	'log.warn',
	'log.error',
];

function fileKeys(event: LiveEvent): InvalidationKey[] {
	const keys: InvalidationKey[] = [];
	for (const entity of event.entities) {
		if ('file' === entity.kind || 'folder' === entity.kind) {
			keys.push(['dir', parentPath(entity.id)], ['stat', entity.id]);
		}
	}
	keys.push(['search']);
	return keys;
}

function jobKeys(event: LiveEvent): InvalidationKey[] {
	if ('job.progress' === event.type) {
		return [];
	}
	const keys: InvalidationKey[] = [['jobs']];
	for (const entity of event.entities) {
		if ('job' === entity.kind) {
			keys.push(['job', entity.id]);
		}
	}
	return keys;
}

export const TOPIC_KEYS: Record<string, (event: LiveEvent) => InvalidationKey[]> = {
	'file.*': fileKeys,
	'link.*': () => [['links'], ['link-stats']],
	'cluster.*': () => [['cluster'], ['instances'], ['health']],
	'job.*': jobKeys,
	'config.*': () => [['config'], ['findings']],
	'health.changed': () => [['health'], ['findings']],
	'finding.*': () => [['findings']],
};

function topicMatches(topic: string, type: string): boolean {
	if (topic.endsWith('.*')) {
		return type.startsWith(topic.slice(0, -1));
	}
	return topic === type;
}

export function keysForEvent(event: LiveEvent): InvalidationKey[] {
	return Object.entries(TOPIC_KEYS).flatMap(([topic, keysOf]) =>
		topicMatches(topic, event.type) ? keysOf(event) : []
	);
}

function isEntity(value: unknown): value is LiveEvent['entities'][number] {
	return (
		'object' === typeof value &&
		null !== value &&
		'kind' in value &&
		'string' === typeof value.kind &&
		'id' in value &&
		'string' === typeof value.id
	);
}

function isLiveEvent(value: unknown): value is LiveEvent {
	return (
		'object' === typeof value &&
		null !== value &&
		'type' in value &&
		'string' === typeof value.type &&
		'entities' in value &&
		Array.isArray(value.entities) &&
		value.entities.every(isEntity)
	);
}

function parseEvent(data: string): Result<LiveEvent, 'malformed'> {
	let value: unknown;
	try {
		value = JSON.parse(data);
	} catch {
		return { ok: false, error: 'malformed' };
	}
	return isLiveEvent(value) ? { ok: true, value } : { ok: false, error: 'malformed' };
}

export function applyStatusSignals(counts: StatusCounts): void {
	const attention = counts.criticalFindings + counts.failedJobs;
	const iconFile = 0 < attention ? 'favicon-bad.svg' : 0 < counts.warningFindings ? 'favicon-warn.svg' : 'favicon.svg';
	const icon = document.querySelector('link[rel="icon"]');
	if (null !== icon) {
		icon.setAttribute('href', `/ui/${iconFile}`);
	}
	const baseTitle = document.title.replace(/^\(\d+\) /, '');
	document.title = 0 < attention ? `(${attention}) ${baseTitle}` : baseTitle;
}

export class LiveHub {
	readonly connection: LiveConnection;
	readonly #openSource: (url: string) => EventSource;
	readonly #topics = new Map<string, number>();
	readonly #listeners = new Set<LiveEventListener>();
	#source: EventSource | null = null;
	#currentUrl: string | null = null;
	#attempt = 0;
	#retryTimer: ReturnType<typeof setTimeout> | null = null;
	#lastEventId: string | null = null;
	#started = false;
	#detach: (() => void) | null = null;

	constructor(openSource: (url: string) => EventSource = (url) => new EventSource(url)) {
		this.#openSource = openSource;
		this.connection = new LiveConnection({
			onPause: () => {
				this.#disconnect();
			},
			onResume: () => {
				this.#sync();
			},
		});
	}

	start(): void {
		if (true === this.#started) {
			return;
		}
		this.#started = true;
		this.#detach = this.connection.watch();
		this.#sync();
	}

	stop(): void {
		this.#started = false;
		this.#detach?.();
		this.#detach = null;
		this.#disconnect();
	}

	subscribe(topics: readonly string[]): () => void {
		this.#adjust(topics, 1);
		this.#sync();
		return () => {
			this.#adjust(topics, -1);
			this.#sync();
		};
	}

	rescope(): void {
		this.#lastEventId = null;
		this.#attempt = 0;
		this.#disconnect();
		this.#sync();
	}

	addListener(listener: LiveEventListener): () => void {
		this.#listeners.add(listener);
		return () => {
			this.#listeners.delete(listener);
		};
	}

	#adjust(topics: readonly string[], delta: number): void {
		for (const topic of topics) {
			const count = (this.#topics.get(topic) ?? 0) + delta;
			if (0 < count) {
				this.#topics.set(topic, count);
			} else {
				this.#topics.delete(topic);
			}
		}
	}

	#sync(): void {
		const blocked = 'paused' === this.connection.state || 'offline' === this.connection.state;
		if (false === this.#started || 0 === this.#topics.size || blocked) {
			this.#disconnect();
			return;
		}
		const url = this.#url();
		if (null !== this.#source && url === this.#currentUrl) {
			return;
		}
		this.#disconnect();
		this.#open(url);
	}

	#url(): string {
		const query = new URLSearchParams({ types: [...this.#topics.keys()].sort().join(',') });
		if (null !== this.#lastEventId) {
			query.set('last_event_id', this.#lastEventId);
		}
		return scopedPath(`/v1/admin/events?${query.toString()}`, nodeScope.selected);
	}

	#open(url: string): void {
		const source = this.#openSource(url);
		this.#source = source;
		this.#currentUrl = url;
		source.addEventListener('open', () => {
			if (source === this.#source) {
				this.#attempt = 0;
				this.connection.state = 'live';
			}
		});
		for (const type of EVENT_TYPES) {
			source.addEventListener(type, (message: MessageEvent<string>) => {
				if (source === this.#source) {
					this.#receive(message);
				}
			});
		}
		source.addEventListener('error', () => {
			this.#fail(source);
		});
	}

	#receive(message: MessageEvent<string>): void {
		if ('' !== message.lastEventId) {
			this.#lastEventId = message.lastEventId;
		}
		const parsed = parseEvent(message.data);
		if (false === parsed.ok) {
			return;
		}
		for (const key of keysForEvent(parsed.value)) {
			invalidate(key);
		}
		for (const listener of this.#listeners) {
			listener(parsed.value);
		}
	}

	#fail(source: EventSource): void {
		if (source !== this.#source) {
			return;
		}
		source.close();
		this.#source = null;
		if (false === navigator.onLine) {
			this.connection.state = 'offline';
			return;
		}
		this.connection.state = 'reconnecting';
		this.#retryTimer = setTimeout(() => {
			this.#retryTimer = null;
			this.#sync();
		}, reconnectDelayMs(this.#attempt));
		this.#attempt += 1;
	}

	#disconnect(): void {
		if (null !== this.#retryTimer) {
			clearTimeout(this.#retryTimer);
			this.#retryTimer = null;
		}
		this.#source?.close();
		this.#source = null;
	}
}

export const liveHub = new LiveHub();
