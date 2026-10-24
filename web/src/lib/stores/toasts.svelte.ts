import type { Tone } from '$lib/ui/types';

export const TOAST_VISIBLE_LIMIT = 3;
export const TOAST_DURATION_MS = 6000;
export const TOAST_ACTION_DURATION_MS = 10000;

export interface ToastAction {
	label: string;
	run: () => void;
}

export interface ToastInput {
	tone: Tone;
	title: string;
	body?: string;
	action?: ToastAction;
	duration?: number;
}

export interface ToastItem {
	id: number;
	tone: Tone;
	title: string;
	body?: string;
	action?: ToastAction;
	duration: number;
}

interface TimerEntry {
	handle: ReturnType<typeof setTimeout> | null;
	remainingMs: number;
	startedAt: number;
}

export class ToastStore {
	items = $state<ToastItem[]>([]);
	queue = $state<ToastItem[]>([]);
	readonly #clock: () => number;
	readonly #timers = new Map<number, TimerEntry>();
	#nextId = 1;

	constructor(clock: () => number = Date.now) {
		this.#clock = clock;
	}

	push(input: ToastInput): number {
		const item: ToastItem = {
			id: this.#nextId,
			tone: input.tone,
			title: input.title,
			body: input.body,
			action: input.action,
			duration: input.duration ?? (undefined === input.action ? TOAST_DURATION_MS : TOAST_ACTION_DURATION_MS),
		};
		this.#nextId += 1;
		if (TOAST_VISIBLE_LIMIT > this.items.length) {
			this.#show(item);
		} else {
			this.queue = [...this.queue, item];
		}
		return item.id;
	}

	dismiss(id: number): void {
		this.#clearTimer(id);
		this.items = this.items.filter((item) => id !== item.id);
		this.queue = this.queue.filter((item) => id !== item.id);
		this.#promote();
	}

	pause(id: number): void {
		const entry = this.#timers.get(id);
		if (undefined === entry || null === entry.handle) {
			return;
		}
		clearTimeout(entry.handle);
		entry.remainingMs = Math.max(0, entry.remainingMs - (this.#clock() - entry.startedAt));
		entry.handle = null;
	}

	resume(id: number): void {
		const entry = this.#timers.get(id);
		if (undefined === entry || null !== entry.handle) {
			return;
		}
		this.#arm(id, entry, entry.remainingMs);
	}

	#show(item: ToastItem): void {
		this.items = [...this.items, item];
		this.#arm(item.id, { handle: null, remainingMs: item.duration, startedAt: 0 }, item.duration);
	}

	#arm(id: number, entry: TimerEntry, ms: number): void {
		entry.startedAt = this.#clock();
		entry.handle = setTimeout(() => this.dismiss(id), ms);
		this.#timers.set(id, entry);
	}

	#clearTimer(id: number): void {
		const entry = this.#timers.get(id);
		if (undefined !== entry && null !== entry.handle) {
			clearTimeout(entry.handle);
		}
		this.#timers.delete(id);
	}

	#promote(): void {
		let next = this.queue[0];
		while (undefined !== next && TOAST_VISIBLE_LIMIT > this.items.length) {
			this.queue = this.queue.slice(1);
			this.#show(next);
			next = this.queue[0];
		}
	}
}

export const toasts = new ToastStore();
