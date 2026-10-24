export const DASH = '—';

export type NumberStyle = 'int' | 'compact' | 'decimal';

export const defaultLocale = (): string => ('undefined' === typeof navigator ? 'en-US' : navigator.language);

export const formatFixed = (n: number, maximumFractionDigits: number, locale: string): string =>
	new Intl.NumberFormat(locale, { maximumFractionDigits }).format(n).replace('-', '−');

const NUMBER_OPTIONS: Record<NumberStyle, Intl.NumberFormatOptions> = {
	int: { maximumFractionDigits: 0 },
	compact: { notation: 'compact', maximumFractionDigits: 1 },
	decimal: { maximumFractionDigits: 2 },
};

export const formatNumber = (n: number | null, style: NumberStyle = 'int', locale: string = defaultLocale()): string => {
	if (null === n || !Number.isFinite(n)) {
		return DASH;
	}
	return new Intl.NumberFormat(locale, NUMBER_OPTIONS[style]).format(n).replace('-', '−');
};

export const formatCount = (
	num: number | null,
	den: number,
	locale: string = defaultLocale(),
): { count: string; rate: string } | null => {
	if (null === num || 0 === den) {
		return null;
	}
	const whole = new Intl.NumberFormat(locale, { maximumFractionDigits: 0 });
	const percent = new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 0 });
	return {
		count: `${whole.format(num)} of ${whole.format(den)}`,
		rate: percent.format(Math.min(num / den, 1)),
	};
};
