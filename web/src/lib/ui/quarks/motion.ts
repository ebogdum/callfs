import { cubicIn, cubicOut } from 'svelte/easing';
import type { TransitionConfig } from 'svelte/transition';

export type TransitionDirection = 'in' | 'out' | 'both';
export type SlideSide = 'left' | 'right' | 'top' | 'bottom';

interface TimingParams {
	delay?: number;
}

interface DirectionOptions {
	direction?: TransitionDirection;
}

interface SlideParams extends TimingParams {
	side: SlideSide;
}

type DurationName = 'dur-fade' | 'dur-medium' | 'dur-large' | 'dur-exit-medium' | 'dur-exit-large';

interface ResolvedTiming {
	delay: number;
	duration: number;
	easing: (progress: number) => number;
}

const FALLBACK_MS: Record<DurationName, number> = {
	'dur-fade': 120,
	'dur-medium': 220,
	'dur-large': 280,
	'dur-exit-medium': 150,
	'dur-exit-large': 190
};

const SCALE_START = 0.97;
const RISE_DISTANCE = 'var(--space-2)';
const MINIMUM_VISIBLE_MS = 400;

const durationCache = new Map<DurationName, number>();

export const prefersReducedMotion = (): boolean => {
	const preference = document.documentElement.dataset.motion;
	if ('reduced' === preference || 'none' === preference) {
		return true;
	}
	if ('full' === preference) {
		return false;
	}
	return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
};

const readDuration = (name: DurationName): number => {
	const cached = durationCache.get(name);
	if (undefined !== cached) {
		return cached;
	}
	const parsed = Number.parseFloat(
		getComputedStyle(document.documentElement).getPropertyValue(`--${name}`)
	);
	const value = Number.isFinite(parsed) ? parsed : FALLBACK_MS[name];
	durationCache.set(name, value);
	return value;
};

const resolveTiming = (
	params: TimingParams,
	options: DirectionOptions | undefined,
	enter: DurationName,
	exit: DurationName
): ResolvedTiming => {
	const exiting = 'out' === options?.direction;
	return {
		delay: params.delay ?? 0,
		duration: prefersReducedMotion() ? 0 : readDuration(exiting ? exit : enter),
		easing: exiting ? cubicIn : cubicOut
	};
};

export const fadeIn = (
	_node: Element,
	params: TimingParams = {},
	options?: DirectionOptions
): TransitionConfig => ({
	...resolveTiming(params, options, 'dur-fade', 'dur-fade'),
	css: (progress: number) => `opacity: ${progress}`
});

export const rise = (
	_node: Element,
	params: TimingParams = {},
	options?: DirectionOptions
): TransitionConfig => ({
	...resolveTiming(params, options, 'dur-medium', 'dur-exit-medium'),
	css: (progress: number) =>
		`opacity: ${progress}; transform: translateY(calc((1 - ${progress}) * ${RISE_DISTANCE}))`
});

export const scaleIn = (
	_node: Element,
	params: TimingParams = {},
	options?: DirectionOptions
): TransitionConfig => ({
	...resolveTiming(params, options, 'dur-medium', 'dur-exit-medium'),
	css: (progress: number) =>
		`opacity: ${progress}; transform: scale(${SCALE_START + (1 - SCALE_START) * progress})`
});

export const slideIn = (
	_node: Element,
	params: SlideParams,
	options?: DirectionOptions
): TransitionConfig => {
	const axis = 'left' === params.side || 'right' === params.side ? 'X' : 'Y';
	const sign = 'left' === params.side || 'top' === params.side ? -1 : 1;
	return {
		...resolveTiming(params, options, 'dur-large', 'dur-exit-large'),
		css: (progress: number) => `transform: translate${axis}(${sign * (1 - progress) * 100}%)`
	};
};

export const collapse = (
	_node: Element,
	params: TimingParams = {},
	options?: DirectionOptions
): TransitionConfig => ({
	...resolveTiming(params, options, 'dur-medium', 'dur-exit-medium'),
	css: (progress: number) => `opacity: ${progress}; grid-template-rows: ${progress}fr`
});

export const holdRemainingMs = (shownAt: number, now: number): number =>
	Math.max(0, shownAt + MINIMUM_VISIBLE_MS - now);

export const holdMinimum = (_node: Element, params: { shownAt: number }): TransitionConfig => ({
	duration: holdRemainingMs(params.shownAt, Date.now())
});
