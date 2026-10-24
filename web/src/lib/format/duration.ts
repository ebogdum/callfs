import { DASH, defaultLocale } from './number';

export type DurationStyle = 'long' | 'short';
export type DurationUnit = 'day' | 'hour' | 'minute' | 'second' | 'millisecond';
export type ParseDurationResult = { ok: true; value: number } | { ok: false; error: 'invalid' };

const GO_UNIT_MS: Record<string, number> = {
	ns: 1e-6,
	'µs': 1e-3,
	'μs': 1e-3,
	us: 1e-3,
	ms: 1,
	s: 1000,
	m: 60 * 1000,
	h: 60 * 60 * 1000,
};
const GO_DURATION = /^([+-]?)((?:\d+(?:\.\d+)?(?:ns|µs|μs|us|ms|s|m|h))+)$/u;
const GO_PART = /(\d+(?:\.\d+)?)(ns|µs|μs|us|ms|s|m|h)/gu;

export const parseGoDuration = (text: string): ParseDurationResult => {
	if ('0' === text) {
		return { ok: true, value: 0 };
	}
	const match = GO_DURATION.exec(text);
	if (null === match) {
		return { ok: false, error: 'invalid' };
	}
	const sign = '-' === match[1] ? -1 : 1;
	let total = 0;
	for (const [, amount, unit] of match[2].matchAll(GO_PART)) {
		total += Number(amount) * GO_UNIT_MS[unit];
	}
	return { ok: true, value: sign * total };
};

export const toGoDuration = (ms: number): string | null => {
	if (!Number.isFinite(ms)) {
		return null;
	}
	const sign = ms < 0 ? '-' : '';
	const total = Math.round(Math.abs(ms));
	if (0 === total) {
		return '0s';
	}
	if (1000 > total) {
		return `${sign}${total}ms`;
	}
	const hours = Math.floor(total / (60 * 60 * 1000));
	const minutes = Math.floor((total % (60 * 60 * 1000)) / (60 * 1000));
	const seconds = Number(((total % (60 * 1000)) / 1000).toFixed(3));
	if (0 < hours) {
		return `${sign}${hours}h${minutes}m${seconds}s`;
	}
	if (0 < minutes) {
		return `${sign}${minutes}m${seconds}s`;
	}
	return `${sign}${seconds}s`;
};

const DURATION_PARTS: ReadonlyArray<readonly [DurationUnit, number]> = [
	['day', 24 * 60 * 60 * 1000],
	['hour', 60 * 60 * 1000],
	['minute', 60 * 1000],
	['second', 1000],
	['millisecond', 1],
];
const MAX_PARTS = 2;

export const formatDuration = (ms: number | null, style: DurationStyle = 'long', locale: string = defaultLocale()): string => {
	if (null === ms || !Number.isFinite(ms)) {
		return DASH;
	}
	const unitDisplay = 'short' === style ? 'narrow' : 'long';
	const formatterFor = (unit: DurationUnit) => new Intl.NumberFormat(locale, { style: 'unit', unit, unitDisplay });
	let remaining = Math.round(Math.abs(ms));
	const parts: string[] = [];
	for (const [unit, size] of DURATION_PARTS) {
		if (MAX_PARTS <= parts.length) {
			break;
		}
		const count = Math.floor(remaining / size);
		if (0 === count) {
			continue;
		}
		remaining -= count * size;
		parts.push(formatterFor(unit).format(count));
	}
	if (0 === parts.length) {
		return formatterFor('second').format(0);
	}
	return parts.join(' ');
};
