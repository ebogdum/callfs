import type { CopyKey } from '$lib/copy/t';

export type ModeResult<T> = { ok: true; value: T } | { ok: false; error: CopyKey };

const MODE_PATTERN = /^[0-7]{3,4}$/;
const SYMBOLIC_PATTERN = /^[r-][w-][x-][r-][w-][x-][r-][w-][x-]$/;
const INVALID: CopyKey = 'mode.error.invalid';

function triad(digit: number): string {
	return `${digit & 4 ? 'r' : '-'}${digit & 2 ? 'w' : '-'}${digit & 1 ? 'x' : '-'}`;
}

function triadValue(text: string): number {
	return (
		('r' === text.charAt(0) ? 4 : 0) + ('w' === text.charAt(1) ? 2 : 0) + ('x' === text.charAt(2) ? 1 : 0)
	);
}

export function isValidMode(mode: string): boolean {
	return MODE_PATTERN.test(mode);
}

export function modeToSymbolic(mode: string): ModeResult<string> {
	if (!isValidMode(mode)) return { ok: false, error: INVALID };
	const digits = mode.slice(-3);
	return { ok: true, value: [...digits].map((digit) => triad(Number(digit))).join('') };
}

export function symbolicToMode(symbolic: string): ModeResult<string> {
	if (!SYMBOLIC_PATTERN.test(symbolic)) return { ok: false, error: INVALID };
	const digits = [0, 3, 6].map((start) => triadValue(symbolic.slice(start, start + 3)));
	return { ok: true, value: `0${digits.join('')}` };
}
