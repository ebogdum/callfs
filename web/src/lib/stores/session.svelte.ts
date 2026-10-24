import { setCsrfToken } from '$lib/api/http';
import * as sessionApi from '$lib/api/session';
import type { Me, Result } from '$lib/api/types';
import type { UiError } from '$lib/ui/types';

export const IDLE_TIMEOUT_MS = 60 * 60 * 1000;
export const EXPIRY_WARNING_MS = 2 * 60 * 1000;
const ACTIVITY_THROTTLE_MS = 1000;
const RESTORE_RETRY_DELAYS_MS = [500, 1000, 2000, 4000, 8000] as const;
const TICK_MS = 1000;
const ACTIVITY_EVENTS = ['pointerdown', 'keydown', 'wheel', 'touchstart'] as const;

function idleMsOf(source: object): number | null {
	const seconds = (source as { idle_timeout_seconds?: unknown }).idle_timeout_seconds;
	return 'number' === typeof seconds && 0 < seconds ? seconds * 1000 : null;
}

export type ExpiryReason = 'idle' | 'expiry' | 'expired';

export interface ExpiryWarning {
	reason: ExpiryReason;
	secondsLeft: number;
}

export function warningFor(expiryLeftMs: number, idleLeftMs: number, reauthRequired: boolean): ExpiryWarning | null {
	if (reauthRequired || 0 >= expiryLeftMs || 0 >= idleLeftMs) {
		return { reason: 'expired', secondsLeft: 0 };
	}
	if (EXPIRY_WARNING_MS >= expiryLeftMs) {
		return { reason: 'expiry', secondsLeft: Math.ceil(expiryLeftMs / 1000) };
	}
	if (EXPIRY_WARNING_MS >= idleLeftMs) {
		return { reason: 'idle', secondsLeft: Math.ceil(idleLeftMs / 1000) };
	}
	return null;
}

export class SessionStore {
	me = $state<Me | null>(null);
	csrf = $state<string | null>(null);
	warning = $state<ExpiryWarning | null>(null);
	restored = $state(false);
	expired = $state(false);
	isAdmin = $derived(null !== this.me && true === this.me.is_admin);
	readonly #clock: () => number;
	#idleTimeoutMs: number;
	#expiresAt: number | null = null;
	#lastActivity: number;
	#reauthRequired = false;
	#reauthWaiters: ((replay: boolean) => void)[] = [];
	#timer: ReturnType<typeof setInterval> | null = null;

	constructor(clock: () => number = Date.now, idleTimeoutMs: number = IDLE_TIMEOUT_MS) {
		this.#clock = clock;
		this.#idleTimeoutMs = idleTimeoutMs;
		this.#lastActivity = clock();
	}

	async refresh(): Promise<Result<Me, UiError>> {
		const result = await sessionApi.me();
		if (false === result.ok) {
			return result;
		}
		this.#adopt(result.value);
		return result;
	}

	async restore(): Promise<void> {
		if (true === this.restored) {
			return;
		}
		let outcome = await this.refresh();
		for (const delay of RESTORE_RETRY_DELAYS_MS) {
			if (outcome.ok || 429 !== outcome.error.status) {
				break;
			}
			await new Promise<void>((resolve) => setTimeout(resolve, delay));
			outcome = await this.refresh();
		}
		this.restored = true;
	}

	async signIn(key: string): Promise<Result<Me, UiError>> {
		const result = await sessionApi.signIn(key);
		if (false === result.ok) {
			return result;
		}
		const me: Me = {
			user_id: result.value.user_id,
			is_admin: result.value.is_admin,
			auth_method: 'session',
			session_expires_at: result.value.expires_at,
			csrf_token: result.value.csrf_token,
			permissions_model: 'owner',
			idle_timeout_seconds: result.value.idle_timeout_seconds,
		};
		this.#adopt(me, idleMsOf(result.value));
		return { ok: true, value: me };
	}

	async signOut(): Promise<Result<void, UiError>> {
		const wasExpired = 'expired' === this.warning?.reason;
		const result = await sessionApi.signOut();
		this.#clear();
		this.expired = wasExpired;
		return result;
	}

	onUnauthorized(): Promise<boolean> {
		if (null === this.me) {
			return Promise.resolve(false);
		}
		this.#reauthRequired = true;
		this.tick();
		return new Promise<boolean>((resolve) => {
			this.#reauthWaiters.push(resolve);
		});
	}

	#settleReauth(replay: boolean): void {
		const waiters = this.#reauthWaiters;
		this.#reauthWaiters = [];
		for (const resolve of waiters) {
			resolve(replay);
		}
	}

	touch(now: number = this.#clock()): void {
		if (ACTIVITY_THROTTLE_MS > now - this.#lastActivity) {
			return;
		}
		this.#lastActivity = now;
	}

	watchActivity(target: EventTarget): () => void {
		const onActivity = (): void => this.touch();
		for (const name of ACTIVITY_EVENTS) {
			target.addEventListener(name, onActivity, { passive: true });
		}
		return (): void => {
			for (const name of ACTIVITY_EVENTS) {
				target.removeEventListener(name, onActivity);
			}
		};
	}

	tick(now: number = this.#clock()): void {
		if (null === this.me && false === this.#reauthRequired) {
			this.warning = null;
			return;
		}
		const expiryLeftMs = null === this.#expiresAt ? Number.POSITIVE_INFINITY : this.#expiresAt - now;
		const idleLeftMs = this.#idleTimeoutMs - (now - this.#lastActivity);
		this.warning = warningFor(expiryLeftMs, idleLeftMs, this.#reauthRequired);
	}

	#adopt(me: Me, idleMs: number | null = idleMsOf(me)): void {
		if (null !== idleMs) {
			this.#idleTimeoutMs = idleMs;
		}
		this.me = me;
		this.csrf = me.csrf_token;
		setCsrfToken(me.csrf_token);
		this.#expiresAt = this.#parseExpiry(me.session_expires_at);
		this.#reauthRequired = false;
		this.expired = false;
		this.#lastActivity = this.#clock();
		this.#startTimer();
		this.tick();
		this.#settleReauth(true);
	}

	#clear(): void {
		this.me = null;
		this.csrf = null;
		setCsrfToken(null);
		this.#expiresAt = null;
		this.#reauthRequired = false;
		this.warning = null;
		this.#stopTimer();
		this.#settleReauth(false);
	}

	#parseExpiry(value: string | null): number | null {
		if (null === value) {
			return null;
		}
		const parsed = Date.parse(value);
		return Number.isNaN(parsed) ? null : parsed;
	}

	#startTimer(): void {
		if (null !== this.#timer) {
			return;
		}
		this.#timer = setInterval(() => this.tick(), TICK_MS);
	}

	#stopTimer(): void {
		if (null === this.#timer) {
			return;
		}
		clearInterval(this.#timer);
		this.#timer = null;
	}
}

export const session = new SessionStore();
