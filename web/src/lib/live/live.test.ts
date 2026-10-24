import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Event as LiveEvent } from '$lib/api/types';
import { applyStatusSignals, keysForEvent, LiveHub, TOPIC_KEYS } from './hub.svelte';

const { invalidate } = vi.hoisted(() => ({ invalidate: vi.fn<(key: readonly unknown[]) => void>() }));

vi.mock('$lib/data/cache', () => ({ invalidate }));

type Listener = (message: MessageEvent<string>) => void;

class MockEventSource {
	static instances: MockEventSource[] = [];
	readonly url: string;
	closed = false;
	readonly #listeners = new Map<string, Listener[]>();

	constructor(url: string) {
		this.url = url;
		MockEventSource.instances.push(this);
	}

	addEventListener(type: string, listener: Listener): void {
		this.#listeners.set(type, [...(this.#listeners.get(type) ?? []), listener]);
	}

	close(): void {
		this.closed = true;
	}

	emit(type: string, data = '', lastEventId = ''): void {
		const message = new MessageEvent<string>(type, { data, lastEventId });
		for (const listener of this.#listeners.get(type) ?? []) {
			listener(message);
		}
	}
}

function eventOf(type: string, entities: { kind: string; id: string }[] = []): LiveEvent {
	return {
		id: 'e1',
		at: '2026-10-23T00:00:00Z',
		type,
		level: 'info',
		actor: null,
		instance_id: 'node-1',
		message: '',
		entities: entities as LiveEvent['entities'],
		fields: {},
	};
}

function latest(): MockEventSource {
	return MockEventSource.instances.at(-1) as MockEventSource;
}

function setVisibility(state: 'hidden' | 'visible'): void {
	Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state });
	document.dispatchEvent(new Event('visibilitychange'));
}

function iconHref(): string | null | undefined {
	return document.querySelector('link[rel="icon"]')?.getAttribute('href');
}

let hub: LiveHub;

beforeEach(() => {
	vi.useFakeTimers();
	MockEventSource.instances = [];
	vi.stubGlobal('EventSource', MockEventSource);
	invalidate.mockClear();
	setVisibility('visible');
	hub = new LiveHub();
});

afterEach(() => {
	hub.stop();
	vi.useRealTimers();
	vi.unstubAllGlobals();
	Object.defineProperty(navigator, 'onLine', { configurable: true, get: () => true });
	document.head.innerHTML = '';
	document.title = '';
});

describe('stream lifecycle', () => {
	it('opens one stream for all subscribed topics', () => {
		hub.start();
		hub.subscribe(['link.*', 'file.*']);
		hub.subscribe(['file.*']);
		expect(MockEventSource.instances).toHaveLength(1);
		const url = new URL(latest().url, 'http://localhost');
		expect(url.pathname).toBe('/v1/admin/events');
		expect(url.searchParams.get('types')).toBe('file.*,link.*');
	});

	it('closes the stream when the last topic is released', () => {
		hub.start();
		const release = hub.subscribe(['file.*']);
		release();
		expect(latest().closed).toBe(true);
	});

	it('closes the stream on stop', () => {
		hub.start();
		hub.subscribe(['file.*']);
		hub.stop();
		expect(latest().closed).toBe(true);
	});
});

describe('reconnect backoff', () => {
	it('waits 1s, 2s, 5s, 10s, 30s and then stays at 30s', () => {
		hub.start();
		hub.subscribe(['file.*']);
		for (const delay of [1000, 2000, 5000, 10000, 30000, 30000]) {
			latest().emit('error');
			expect(hub.connection.state).toBe('reconnecting');
			const opened = MockEventSource.instances.length;
			vi.advanceTimersByTime(delay - 1);
			expect(MockEventSource.instances).toHaveLength(opened);
			vi.advanceTimersByTime(1);
			expect(MockEventSource.instances).toHaveLength(opened + 1);
		}
	});

	it('resets the backoff once the stream opens', () => {
		hub.start();
		hub.subscribe(['file.*']);
		latest().emit('error');
		vi.advanceTimersByTime(1000);
		latest().emit('open');
		expect(hub.connection.state).toBe('live');
		latest().emit('error');
		const opened = MockEventSource.instances.length;
		vi.advanceTimersByTime(1000);
		expect(MockEventSource.instances).toHaveLength(opened + 1);
	});

	it('resumes from the last event id after a reconnect', () => {
		hub.start();
		hub.subscribe(['file.*']);
		latest().emit('file.created', JSON.stringify(eventOf('file.created')), '41');
		latest().emit('error');
		vi.advanceTimersByTime(1000);
		const url = new URL(latest().url, 'http://localhost');
		expect(url.searchParams.get('last_event_id')).toBe('41');
	});
});

describe('events and invalidation', () => {
	it('file events invalidate the parent folder, the path and search', () => {
		hub.start();
		hub.subscribe(['file.*']);
		latest().emit(
			'file.created',
			JSON.stringify(eventOf('file.created', [{ kind: 'file', id: '/docs/a.txt' }])),
		);
		expect(invalidate.mock.calls).toEqual([[['dir', '/docs']], [['stat', '/docs/a.txt']], [['search']]]);
	});

	it('job progress reaches listeners without invalidation', () => {
		const listener = vi.fn<(event: LiveEvent) => void>();
		hub.addListener(listener);
		hub.start();
		hub.subscribe(['job.*']);
		latest().emit('job.progress', JSON.stringify(eventOf('job.progress', [{ kind: 'job', id: 'j1' }])));
		expect(invalidate).not.toHaveBeenCalled();
		expect(listener).toHaveBeenCalledTimes(1);
		expect(listener.mock.calls[0]?.[0]).toMatchObject({ type: 'job.progress' });
	});

	it('failed jobs invalidate the job list and the job', () => {
		hub.start();
		hub.subscribe(['job.*']);
		latest().emit('job.failed', JSON.stringify(eventOf('job.failed', [{ kind: 'job', id: 'j1' }])));
		expect(invalidate.mock.calls).toEqual([[['jobs']], [['job', 'j1']]]);
	});

	it('ignores malformed frames', () => {
		hub.start();
		hub.subscribe(['file.*']);
		latest().emit('file.created', 'not json');
		latest().emit('file.created', '{"type":1,"entities":[]}');
		expect(invalidate).not.toHaveBeenCalled();
	});

	it('maps cluster and link events to their query keys', () => {
		expect(keysForEvent(eventOf('cluster.leader_changed'))).toEqual([['cluster'], ['instances'], ['health']]);
		expect(keysForEvent(eventOf('link.created'))).toEqual([['links'], ['link-stats']]);
		expect(keysForEvent(eventOf('health.changed'))).toEqual([['health'], ['findings']]);
	});

	it('exposes every topic key', () => {
		expect(Object.keys(TOPIC_KEYS)).toEqual([
			'file.*',
			'link.*',
			'cluster.*',
			'job.*',
			'config.*',
			'health.changed',
			'finding.*',
		]);
	});
});

describe('visibility and network', () => {
	it('pauses after 60s hidden and reconnects when visible', () => {
		hub.start();
		hub.subscribe(['file.*']);
		setVisibility('hidden');
		vi.advanceTimersByTime(59999);
		expect(hub.connection.state).toBe('live');
		vi.advanceTimersByTime(1);
		expect(hub.connection.state).toBe('paused');
		expect(latest().closed).toBe(true);
		setVisibility('visible');
		expect(MockEventSource.instances).toHaveLength(2);
		expect(hub.connection.state).toBe('reconnecting');
	});

	it('goes offline and reconnects when the network returns', () => {
		hub.start();
		hub.subscribe(['file.*']);
		window.dispatchEvent(new Event('offline'));
		expect(hub.connection.state).toBe('offline');
		expect(latest().closed).toBe(true);
		window.dispatchEvent(new Event('online'));
		expect(MockEventSource.instances).toHaveLength(2);
		expect(hub.connection.state).toBe('reconnecting');
	});

	it('waits for the network instead of retrying when a stream fails offline', () => {
		hub.start();
		hub.subscribe(['file.*']);
		Object.defineProperty(navigator, 'onLine', { configurable: true, get: () => false });
		latest().emit('error');
		expect(hub.connection.state).toBe('offline');
		vi.advanceTimersByTime(30000);
		expect(MockEventSource.instances).toHaveLength(1);
	});
});

describe('applyStatusSignals', () => {
	beforeEach(() => {
		document.head.innerHTML = '<link rel="icon" href="/ui/favicon.svg">';
		document.title = 'Overview · callfs';
	});

	it('critical findings and failed jobs set the bad icon and the count prefix', () => {
		applyStatusSignals({ criticalFindings: 1, failedJobs: 1, warningFindings: 0 });
		expect(iconHref()).toBe('/ui/favicon-bad.svg');
		expect(document.title).toBe('(2) Overview · callfs');
	});

	it('warnings set the warn icon without a prefix', () => {
		applyStatusSignals({ criticalFindings: 0, failedJobs: 0, warningFindings: 3 });
		expect(iconHref()).toBe('/ui/favicon-warn.svg');
		expect(document.title).toBe('Overview · callfs');
	});

	it('clearing the status restores the base icon and title', () => {
		applyStatusSignals({ criticalFindings: 2, failedJobs: 0, warningFindings: 0 });
		applyStatusSignals({ criticalFindings: 0, failedJobs: 0, warningFindings: 0 });
		expect(iconHref()).toBe('/ui/favicon.svg');
		expect(document.title).toBe('Overview · callfs');
	});
});
