import { DASH, defaultLocale, formatFixed } from './number';

const UNITS = ['B', 'kB', 'MB', 'GB', 'TB', 'PB', 'EB'] as const;
const LAST_UNIT_INDEX = UNITS.length - 1;
const BASE = 1000;

export interface BytesOptions {
	precision?: number;
	locale?: string;
}

export interface FormattedBytes {
	text: string;
	exact: string | null;
}

const decimalsFor = (index: number, precision: number): number => (0 === index ? 0 : precision);

const unitIndexFor = (magnitude: number, precision: number): number => {
	let index = 0;
	while (LAST_UNIT_INDEX > index && BASE ** (index + 1) <= magnitude) {
		index += 1;
	}
	while (LAST_UNIT_INDEX > index && BASE <= Number((magnitude / BASE ** index).toFixed(decimalsFor(index, precision)))) {
		index += 1;
	}
	return index;
};

export const formatBytes = (n: number | null, { precision = 1, locale = defaultLocale() }: BytesOptions = {}): FormattedBytes => {
	if (null === n || !Number.isFinite(n)) {
		return { text: DASH, exact: null };
	}
	const index = unitIndexFor(Math.abs(n), precision);
	const scaled = n / BASE ** index;
	return {
		text: `${formatFixed(scaled, decimalsFor(index, precision), locale)} ${UNITS[index]}`,
		exact: `${formatFixed(Math.round(n), 0, locale)} B`,
	};
};
