import type { Tone } from '$lib/ui/types';

export const NOTIFICATION_LIMIT = 100;

export type NotificationCategory = 'error' | 'job' | 'cluster' | 'event';
export type NotificationFilter = 'all' | 'errors' | 'jobs' | 'cluster';

export interface NotificationItem {
	id: number;
	at: number;
	tone: Tone;
	category: NotificationCategory;
	title: string;
	body: string;
	requestId: string | null;
	read: boolean;
}

export interface NotificationInput {
	tone: Tone;
	category: NotificationCategory;
	title: string;
	body: string;
	requestId?: string | null;
}

export function matchesFilter(item: NotificationItem, filter: NotificationFilter): boolean {
	if ('all' === filter) {
		return true;
	}
	if ('errors' === filter) {
		return 'error' === item.category;
	}
	if ('jobs' === filter) {
		return 'job' === item.category;
	}
	return 'cluster' === item.category;
}

export class NotificationStore {
	items = $state<NotificationItem[]>([]);
	unread = $derived(this.items.filter((item) => false === item.read).length);
	readonly #clock: () => number;
	#nextId = 1;

	constructor(clock: () => number = Date.now) {
		this.#clock = clock;
	}

	add(input: NotificationInput): NotificationItem {
		const item: NotificationItem = {
			id: this.#nextId,
			at: this.#clock(),
			tone: input.tone,
			category: input.category,
			title: input.title,
			body: input.body,
			requestId: input.requestId ?? null,
			read: false,
		};
		this.#nextId += 1;
		this.items = [item, ...this.items].slice(0, NOTIFICATION_LIMIT);
		return item;
	}

	filtered(filter: NotificationFilter): NotificationItem[] {
		return this.items.filter((item) => matchesFilter(item, filter));
	}

	markRead(id: number): void {
		this.items = this.items.map((item) => (id === item.id ? { ...item, read: true } : item));
	}

	markAllRead(): void {
		this.items = this.items.map((item) => ({ ...item, read: true }));
	}

	clear(id: number): void {
		this.items = this.items.filter((item) => id !== item.id);
	}

	clearAll(): void {
		this.items = [];
	}
}

export const notifications = new NotificationStore();
