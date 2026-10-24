import { HIDDEN_PAUSE_MS } from './poller';

export type ConnectionState = 'live' | 'reconnecting' | 'offline' | 'paused';

export interface ConnectionHandlers {
	onPause: () => void;
	onResume: () => void;
}

export type LiveIndicator = 'live' | 'reconnecting' | 'offline' | 'paused';

export class LiveConnection {
	state = $state<ConnectionState>('live');
	apiReachable = $state(true);
	indicator = $derived<LiveIndicator>(false === this.apiReachable ? 'offline' : this.state);
	pageState = $derived<'live' | 'stale' | 'paused' | 'offline'>(
		'reconnecting' === this.indicator ? 'stale' : this.indicator
	);
	readonly #handlers: ConnectionHandlers;
	#hiddenTimer: ReturnType<typeof setTimeout> | null = null;

	constructor(handlers: ConnectionHandlers) {
		this.#handlers = handlers;
	}

	watch(): () => void {
		const onVisibilityChange = (): void => {
			if ('hidden' === document.visibilityState) {
				this.#startHiddenTimer();
				return;
			}
			this.#clearHiddenTimer();
			if ('paused' === this.state) {
				this.#resume();
			}
		};
		const onOffline = (): void => {
			if ('paused' === this.state) {
				return;
			}
			this.state = 'offline';
			this.#handlers.onPause();
		};
		const onOnline = (): void => {
			if ('offline' === this.state) {
				this.#resume();
			}
		};
		document.addEventListener('visibilitychange', onVisibilityChange);
		window.addEventListener('offline', onOffline);
		window.addEventListener('online', onOnline);
		return (): void => {
			document.removeEventListener('visibilitychange', onVisibilityChange);
			window.removeEventListener('offline', onOffline);
			window.removeEventListener('online', onOnline);
			this.#clearHiddenTimer();
		};
	}

	#startHiddenTimer(): void {
		this.#clearHiddenTimer();
		this.#hiddenTimer = setTimeout(() => {
			this.#hiddenTimer = null;
			this.#pause();
		}, HIDDEN_PAUSE_MS);
	}

	#clearHiddenTimer(): void {
		if (null !== this.#hiddenTimer) {
			clearTimeout(this.#hiddenTimer);
			this.#hiddenTimer = null;
		}
	}

	#pause(): void {
		if ('paused' === this.state) {
			return;
		}
		this.state = 'paused';
		this.#handlers.onPause();
	}

	#resume(): void {
		this.state = 'reconnecting';
		this.#handlers.onResume();
	}
}
