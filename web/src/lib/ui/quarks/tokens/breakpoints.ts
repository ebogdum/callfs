export const BREAKPOINTS = {
	sm: 640,
	md: 768,
	split: 840,
	lg: 1024,
	xl: 1280,
	xxl: 1536
} as const;

export type BreakpointName = keyof typeof BREAKPOINTS;

export function matches(name: BreakpointName): boolean {
	if ('undefined' === typeof window || 'function' !== typeof window.matchMedia) {
		return false;
	}
	return window.matchMedia(`(min-width: ${BREAKPOINTS[name]}px)`).matches;
}
