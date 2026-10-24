import { DASH, defaultLocale } from './number';

export type DateInput = Date | string | number;

const RELATIVE_LIMIT_MS = 7 * 24 * 60 * 60 * 1000;
const SECOND_UNIT = ['second', 1000] as const;
const RELATIVE_UNITS: ReadonlyArray<readonly [Intl.RelativeTimeFormatUnit, number]> = [
	['day', 24 * 60 * 60 * 1000],
	['hour', 60 * 60 * 1000],
	['minute', 60 * 1000],
	SECOND_UNIT,
];

const toDate = (input: DateInput): Date | null => {
	const date = input instanceof Date ? input : new Date(input);
	return Number.isNaN(date.getTime()) ? null : date;
};

export const formatAbsolute = (input: DateInput, locale: string = defaultLocale()): string => {
	const date = toDate(input);
	if (null === date) {
		return DASH;
	}
	return new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', year: 'numeric' }).format(date);
};

export const formatRelative = (input: DateInput, now: DateInput = Date.now(), locale: string = defaultLocale()): string => {
	const date = toDate(input);
	const reference = toDate(now);
	if (null === date || null === reference) {
		return DASH;
	}
	const delta = date.getTime() - reference.getTime();
	const magnitude = Math.abs(delta);
	if (RELATIVE_LIMIT_MS <= magnitude) {
		return formatAbsolute(date, locale);
	}
	const [unit, size] = RELATIVE_UNITS.find(([, unitMs]) => unitMs <= magnitude) ?? SECOND_UNIT;
	const value = Math.sign(delta) * Math.floor(magnitude / size);
	return new Intl.RelativeTimeFormat(locale, { numeric: 'always' }).format(value, unit);
};

export const formatTimestamp = (input: DateInput, locale: string = defaultLocale()): string => {
	const date = toDate(input);
	if (null === date) {
		return DASH;
	}
	return new Intl.DateTimeFormat(locale, {
		day: 'numeric',
		month: 'short',
		year: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		timeZoneName: 'short',
	}).format(date);
};
