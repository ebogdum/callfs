import { describe, expect, it } from 'vitest';
import { formatBytes } from './bytes';
import { formatCount, formatNumber } from './number';
import { formatAbsolute, formatRelative, formatTimestamp } from './time';
import { formatDuration, parseGoDuration, toGoDuration } from './duration';

const NOW = Date.UTC(2026, 2, 20, 12);
const HOUR = 60 * 60 * 1000;
const DAY = 24 * HOUR;

describe('formatBytes', () => {
	it('formats SI units with a non-breaking space', () => {
		expect(formatBytes(1500000, { locale: 'en-US' })).toEqual({ text: '1.5 MB', exact: '1,500,000 B' });
	});

	it('uses the requested precision', () => {
		expect(formatBytes(1234567, { precision: 2, locale: 'en-US' }).text).toBe('1.23 MB');
	});

	it('picks the next unit when rounding reaches 1000', () => {
		expect(formatBytes(999.6, { locale: 'en-US' }).text).toBe('1 kB');
	});

	it('selects gigabytes for two billion bytes', () => {
		expect(formatBytes(2000000000, { locale: 'en-US' }).text).toBe('2 GB');
	});

	it('renders a dash and no exact value for null', () => {
		expect(formatBytes(null)).toEqual({ text: '—', exact: null });
	});
});

describe('formatNumber', () => {
	it('groups thousands for the int style', () => {
		expect(formatNumber(1234567, 'int', 'en-US')).toBe('1,234,567');
	});

	it('abbreviates large values for the compact style', () => {
		expect(formatNumber(1234567, 'compact', 'en-US')).toBe('1.2M');
	});

	it('uses the true minus sign', () => {
		expect(formatNumber(-5, 'int', 'en-US')).toBe('−5');
	});

	it('renders a dash for null', () => {
		expect(formatNumber(null)).toBe('—');
	});
});

describe('formatCount', () => {
	it('returns whole counts and a rounded share', () => {
		expect(formatCount(312, 1204, 'en-US')).toEqual({ count: '312 of 1,204', rate: '26%' });
	});

	it('caps the share at 100 percent', () => {
		expect(formatCount(9, 4, 'en-US')).toEqual({ count: '9 of 4', rate: '100%' });
	});

	it('returns null when the denominator is zero', () => {
		expect(formatCount(5, 0, 'en-US')).toBeNull();
	});

	it('returns null when the numerator is null', () => {
		expect(formatCount(null, 10, 'en-US')).toBeNull();
	});
});

describe('formatRelative', () => {
	it('renders past times in hours', () => {
		expect(formatRelative(NOW - 3 * HOUR, NOW, 'en-US')).toBe('3 hours ago');
	});

	it('renders seconds for short gaps', () => {
		expect(formatRelative(NOW - 30000, NOW, 'en-US')).toBe('30 seconds ago');
	});

	it('renders future times with in', () => {
		expect(formatRelative(NOW + 2 * DAY, NOW, 'en-US')).toBe('in 2 days');
	});

	it('switches to the absolute date after seven days', () => {
		expect(formatRelative(NOW - 10 * DAY, NOW, 'en-US')).toBe('Mar 10, 2026');
	});

	it('renders a dash for an invalid date', () => {
		expect(formatRelative('not a date', NOW, 'en-US')).toBe('—');
	});
});

describe('formatAbsolute', () => {
	it('renders day, short month and year', () => {
		expect(formatAbsolute(new Date(Date.UTC(2026, 2, 12, 12)), 'en-US')).toBe('Mar 12, 2026');
	});

	it('renders a dash for null-like input', () => {
		expect(formatAbsolute(Number.NaN, 'en-US')).toBe('—');
	});
});

describe('formatTimestamp', () => {
	it('renders date, time and a timezone name', () => {
		const text = formatTimestamp(new Date(Date.UTC(2026, 2, 12, 12)), 'en-US');
		expect(text.startsWith('Mar 12, 2026, ')).toBe(true);
		expect(text.split(' ').at(-1)).toMatch(/^(UTC|GMT.*|[A-Z]{2,5})$/);
	});
});

describe('parseGoDuration', () => {
	it('parses a single unit', () => {
		expect(parseGoDuration('30s')).toEqual({ ok: true, value: 30000 });
	});

	it('parses compound units', () => {
		expect(parseGoDuration('1h30m')).toEqual({ ok: true, value: 5400000 });
	});

	it('parses fractions and milliseconds', () => {
		expect(parseGoDuration('1.5s')).toEqual({ ok: true, value: 1500 });
		expect(parseGoDuration('500ms')).toEqual({ ok: true, value: 500 });
	});

	it('parses micro seconds with the micro sign', () => {
		expect(parseGoDuration('1µs')).toEqual({ ok: true, value: 0.001 });
	});

	it('parses long retention values', () => {
		expect(parseGoDuration('720h')).toEqual({ ok: true, value: 2592000000 });
	});

	it('parses a negative sign', () => {
		expect(parseGoDuration('-1s')).toEqual({ ok: true, value: -1000 });
	});

	it('accepts a bare zero', () => {
		expect(parseGoDuration('0')).toEqual({ ok: true, value: 0 });
	});

	it('rejects text without units and empty input', () => {
		expect(parseGoDuration('abc')).toEqual({ ok: false, error: 'invalid' });
		expect(parseGoDuration('')).toEqual({ ok: false, error: 'invalid' });
		expect(parseGoDuration('30')).toEqual({ ok: false, error: 'invalid' });
	});
});

describe('toGoDuration', () => {
	it('writes seconds, minutes and hours the way Go does', () => {
		expect(toGoDuration(30000)).toBe('30s');
		expect(toGoDuration(90000)).toBe('1m30s');
		expect(toGoDuration(3600000)).toBe('1h0m0s');
		expect(toGoDuration(2592000000)).toBe('720h0m0s');
	});

	it('writes fractions and milliseconds', () => {
		expect(toGoDuration(1500)).toBe('1.5s');
		expect(toGoDuration(500)).toBe('500ms');
	});

	it('writes zero as 0s', () => {
		expect(toGoDuration(0)).toBe('0s');
	});

	it('returns null for non-finite input', () => {
		expect(toGoDuration(Number.NaN)).toBeNull();
	});

	it('round-trips through parseGoDuration', () => {
		const text = toGoDuration(5400000);
		expect(null === text ? null : parseGoDuration(text)).toEqual({ ok: true, value: 5400000 });
	});
});

describe('formatDuration', () => {
	it('renders long seconds', () => {
		expect(formatDuration(30000, 'long', 'en-US')).toBe('30 seconds');
	});

	it('renders at most two parts', () => {
		expect(formatDuration(125000, 'long', 'en-US')).toBe('2 minutes 5 seconds');
		expect(formatDuration(5400000, 'long', 'en-US')).toBe('1 hour 30 minutes');
	});

	it('renders whole days', () => {
		expect(formatDuration(30 * DAY, 'long', 'en-US')).toBe('30 days');
	});

	it('renders milliseconds below one second', () => {
		expect(formatDuration(500, 'long', 'en-US')).toBe('500 milliseconds');
	});

	it('renders zero as seconds', () => {
		expect(formatDuration(0, 'long', 'en-US')).toBe('0 seconds');
	});

	it('renders the short style with narrow units', () => {
		expect(formatDuration(30000, 'short', 'en-US')).toBe('30s');
		expect(formatDuration(125000, 'short', 'en-US')).toBe('2m 5s');
	});

	it('renders a dash for null', () => {
		expect(formatDuration(null)).toBe('—');
	});
});
