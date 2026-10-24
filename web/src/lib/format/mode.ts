import type { CopyKey } from '$lib/copy/t';

export type ModeResult<T> = { ok: true; value: T } | { ok: false; error: CopyKey };

const MODE_PATTERN = /^[0-7]{3,4}$/;
const SYMBOLIC_PATTERN = /^[r-][w-][x-][r-][w-][x-][r-][w-][x-]$/;
const CLAUSE_PATTERN = /^([ugoa]*)([=+-])([rwx]*)$/;
const WHO_INDEX: Record<string, number> = { u: 0, g: 1, o: 2 };
const PERMISSION_BITS: Record<string, number> = { r: 4, w: 2, x: 1 };
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

export function symbolicToMode(symbolic: string, base = '000'): ModeResult<string> {
	if (SYMBOLIC_PATTERN.test(symbolic)) {
		const digits = [0, 3, 6].map((start) => triadValue(symbolic.slice(start, start + 3)));
		return { ok: true, value: `0${digits.join('')}` };
	}
	if (!isValidMode(base)) return { ok: false, error: INVALID };
	const digits = [...base.slice(-3)].map(Number);
	for (const clause of symbolic.split(',')) {
		const match = CLAUSE_PATTERN.exec(clause.trim());
		if (null === match) return { ok: false, error: INVALID };
		const who = '' === (match[1] ?? '') || (match[1] ?? '').includes('a') ? 'ugo' : (match[1] ?? '');
		const bits = [...(match[3] ?? '')].reduce((sum, letter) => sum | (PERMISSION_BITS[letter] ?? 0), 0);
		for (const letter of who) {
			const index = WHO_INDEX[letter] ?? 0;
			const current = digits[index] ?? 0;
			digits[index] = '=' === match[2] ? bits : '+' === match[2] ? current | bits : current & ~bits;
		}
	}
	return { ok: true, value: `0${digits.join('')}` };
}
