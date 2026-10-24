export const HIDDEN_PAUSE_MS = 60000;

const MAX_RECONNECT_DELAY_MS = 30000;
const RECONNECT_DELAYS_MS: readonly number[] = [1000, 2000, 5000, 10000, MAX_RECONNECT_DELAY_MS];

export function reconnectDelayMs(attempt: number): number {
	const index = Math.min(Math.max(0, attempt), RECONNECT_DELAYS_MS.length - 1);
	return RECONNECT_DELAYS_MS.at(index) ?? MAX_RECONNECT_DELAY_MS;
}
