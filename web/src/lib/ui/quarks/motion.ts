import type { TransitionConfig } from 'svelte/transition';

const MINIMUM_VISIBLE_MS = 400;

export const holdRemainingMs = (shownAt: number, now: number): number =>
	Math.max(0, shownAt + MINIMUM_VISIBLE_MS - now);

export const holdMinimum = (_node: Element, params: { shownAt: number }): TransitionConfig => ({
	duration: holdRemainingMs(params.shownAt, Date.now())
});
